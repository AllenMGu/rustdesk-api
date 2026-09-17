package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lejianwen/rustdesk-api/v2/global"
	requstform "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
)

type Peer struct {
}

const (
	// sysInfoRateLimit 每 IP 每分钟允许的 sysinfo 提交次数
	// （客户端仅在系统信息变化或版本变化时上报，正常频率极低）
	sysInfoRateLimit = 30
	// sysInfoNewRateLimit 每 IP 每分钟允许的新设备注册次数，
	// 限制匿名来源批量伪造 peer 记录（存储型 DoS）
	sysInfoNewRateLimit = 10
)

// SysInfo
// @Tags System
// @Summary 提交系统信息
// @Description 提交系统信息
// @Accept  json
// @Produce  json
// @Param body body requstform.PeerForm true "系统信息表单"
// @Success 200 {string} string "SYSINFO_UPDATED,ID_NOT_FOUND"
// @Failure 500 {object} response.ErrorResponse
// @Router /sysinfo [post]
func (p *Peer) SysInfo(c *gin.Context) {
	f := &requstform.PeerForm{}
	err := c.ShouldBindBodyWith(f, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	// 先规范化再查询：id 必填、长度超限直接拒绝（而不是截断后查询/入库，
	// 截断会改变查询键，导致同一设备重复建 peer 或身份校验失配）
	f.Id = strings.TrimSpace(f.Id)
	f.Uuid = strings.TrimSpace(f.Uuid)
	if verr := requstform.ValidatePeerIdentity(f.Id, f.Uuid); verr != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+" "+verr.Error())
		return
	}

	clientIp := c.ClientIP()
	// 匿名端点限流：限制单 IP 的提交频率，防止伪造记录造成存储膨胀
	if !global.RateLimiter.Allow("sysinfo:"+clientIp, sysInfoRateLimit, time.Minute) {
		c.Header("Retry-After", "60")
		response.Error(c, response.TranslateMsg(c, "TooManyRequests"))
		return
	}

	pe := service.AllService.PeerService.FindById(f.Id)
	if pe.RowId == 0 {
		// 新设备注册：必须携带非空 uuid（客户端 encode64(get_uuid()) 的机器标识），
		// 否则无法建立后续的身份校验，拒绝注册
		if f.Uuid == "" {
			global.Logger.Warnf("sysinfo rejected: new device without uuid id=%s ip=%s", f.Id, clientIp)
			c.String(http.StatusForbidden, "UNAUTHORIZED")
			return
		}
		// 新设备注册：单独限流，防止匿名来源批量伪造 peer
		if !global.RateLimiter.Allow("sysinfo-new:"+clientIp, sysInfoNewRateLimit, time.Minute) {
			c.Header("Retry-After", "60")
			response.Error(c, response.TranslateMsg(c, "TooManyRequests"))
			return
		}
		pe = f.ToPeer()
		pe.UserId = service.AllService.UserService.FindLatestUserIdFromLoginLogByUuid(pe.Uuid, pe.Id)
		err = service.AllService.PeerService.Create(pe)
		if err != nil {
			response.Error(c, response.TranslateMsg(c, "OperationFailed")+err.Error())
			return
		}
	} else if pe.Uuid == "" {
		// 尚未绑定 uuid 的旧 peer：仅允许在本端点（设备自报）做一次性绑定，
		// 校验路径（VerifyDeviceIdentity）绝不写库
		if f.Uuid == "" {
			global.Logger.Warnf("sysinfo rejected: legacy peer requires uuid id=%s ip=%s", f.Id, clientIp)
			c.String(http.StatusForbidden, "UNAUTHORIZED")
			return
		}
		// 按 FindById 得到的具体行（主键 row_id）绑定，而非业务 id：
		// 旧库同 id 多行时按 id UPDATE 会批量改动多行且"失败"仍有副作用
		if !service.AllService.PeerService.BindLegacyDeviceIdentity(pe.RowId, f.Uuid) {
			global.Logger.Warnf("sysinfo rejected: bind legacy identity failed id=%s ip=%s", f.Id, clientIp)
			c.String(http.StatusForbidden, "UNAUTHORIZED")
			return
		}
		pe = service.AllService.PeerService.FindById(f.Id)
	} else if !service.AllService.PeerService.VerifyDeviceIdentity(f.Id, f.Uuid) {
		// 已有设备：uuid 必须与已绑定的设备身份精确一致（Base64 大小写敏感）。
		// 不一致即视为伪造 id 或设备身份不符，拒绝写入；
		// 返回 403 "UNAUTHORIZED"：客户端不会把它当作 SYSINFO_UPDATED/ID_NOT_FOUND，
		// 会在退避（约 120 秒）后重试，而不是停止上报。
		global.Logger.Warnf("sysinfo rejected: device identity mismatch id=%s uuid=%s ip=%s", f.Id, f.Uuid, clientIp)
		c.String(http.StatusForbidden, "UNAUTHORIZED")
		return
	}

	if pe.UserId == 0 {
		pe.UserId = service.AllService.UserService.FindLatestUserIdFromLoginLogByUuid(pe.Uuid, pe.Id)
	}
	fpe := f.ToPeer()
	fpe.RowId = pe.RowId
	fpe.UserId = pe.UserId
	// 已绑定 uuid 的 peer 不允许被上报的其它 uuid 覆盖
	if pe.Uuid != "" {
		fpe.Uuid = pe.Uuid
	}
	err = service.AllService.PeerService.Update(fpe)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	//SYSINFO_UPDATED 上传成功
	//ID_NOT_FOUND 下次心跳会上传
	//直接响应文本
	c.String(http.StatusOK, "SYSINFO_UPDATED")
}

// SysInfoVer
// @Tags System
// @Summary 获取系统版本信息
// @Description 获取系统版本信息
// @Accept  json
// @Produce  json
// @Success 200 {string} string ""
// @Failure 500 {object} response.ErrorResponse
// @Router /sysinfo_ver [post]
func (p *Peer) SysInfoVer(c *gin.Context) {
	//读取resources/version文件
	v := service.AllService.AppService.GetAppVersion()
	// 加上启动时间，方便client上传信息
	v = fmt.Sprintf("%s\n%s", v, service.AllService.AppService.GetStartTime())
	c.String(http.StatusOK, v)
}
