package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lejianwen/rustdesk-api/v2/global"
	request "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Audit struct {
}

// auditRateLimit 每 IP 每分钟允许的审计记录写入次数
// （客户端在连接建立/关闭、文件传输时上报，正常频率有限）
const auditRateLimit = 120

// verifyAuditIdentity 校验匿名审计上报的设备身份与速率。
// 官方 RustDesk 客户端匿名 POST /api/audit/conn|file 时携带 id 与 uuid，
// (id, uuid) 是同一台机器上稳定成对出现的标识，只有已知设备才能写入审计记录，
// 防止匿名来源伪造审计日志或批量写入造成存储型 DoS。
//
// 本方法不写任何响应：审计端点对客户端保持“静默 200、不落库”的契约，
// 无论拒绝原因是限流、身份不符还是报文异常，响应形态都相同，
// 不向攻击方泄露拒绝原因。响应统一由调用方（AuditConn/AuditFile）写一次。
func (a *Audit) verifyAuditIdentity(c *gin.Context, peerId, uuid string) bool {
	clientIp := c.ClientIP()
	if !global.RateLimiter.Allow("audit:"+clientIp, auditRateLimit, time.Minute) {
		global.Logger.Warnf("audit rejected: rate limited id=%s ip=%s", peerId, clientIp)
		return false
	}
	// id 长度规范化：超限直接拒绝（而不是截断后查询，避免身份校验失配）
	if verr := request.ValidatePeerIdentity(peerId, uuid); verr != nil {
		global.Logger.Warnf("audit rejected: malformed identity id=%s ip=%s (%v)", peerId, clientIp, verr)
		return false
	}
	if !service.AllService.PeerService.VerifyDeviceIdentity(peerId, uuid) {
		global.Logger.Warnf("audit rejected: unknown device identity id=%s ip=%s", peerId, clientIp)
		return false
	}
	return true
}

// silentSuccess 审计端点唯一的拒绝响应形态：200 + 空 data，不落库、不泄露原因
func (a *Audit) silentSuccess(c *gin.Context) {
	response.Success(c, "")
}

// AuditConn
// @Tags 审计
// @Summary 审计连接
// @Description 审计连接
// @Accept  json
// @Produce  json
// @Param body body request.AuditConnForm true "审计连接"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/conn [post]
func (a *Audit) AuditConn(c *gin.Context) {
	af := &request.AuditConnForm{}
	err := c.ShouldBindBodyWith(af, binding.JSON)
	if err != nil {
		// 报文异常同样按静默 200 处理（不落库、不泄露原因），保持响应形态单一
		global.Logger.Warnf("audit conn rejected: malformed body: %v", err)
		a.silentSuccess(c)
		return
	}
	if !a.verifyAuditIdentity(c, af.Id, af.Uuid) {
		a.silentSuccess(c)
		return
	}
	/*ttt := &gin.H{}
	c.ShouldBindBodyWith(ttt, binding.JSON)
	fmt.Println(ttt)*/
	ac := af.ToAuditConn()
	if af.Action == model.AuditActionNew {
		service.AllService.AuditService.CreateAuditConn(ac)
	} else if af.Action == model.AuditActionClose {
		ex := service.AllService.AuditService.InfoByPeerIdAndConnId(af.Id, af.ConnId)
		if ex.Id != 0 {
			ex.CloseTime = time.Now().Unix()
			service.AllService.AuditService.UpdateAuditConn(ex)
		}
	} else if af.Action == "" {
		ex := service.AllService.AuditService.InfoByPeerIdAndConnId(af.Id, af.ConnId)
		if ex.Id != 0 {
			up := &model.AuditConn{
				IdModel:   model.IdModel{Id: ex.Id},
				FromPeer:  ac.FromPeer,
				FromName:  ac.FromName,
				SessionId: ac.SessionId,
				Type:      ac.Type,
			}
			service.AllService.AuditService.UpdateAuditConn(up)
		}
	}
	response.Success(c, "")
}

// AuditFile
// @Tags 审计
// @Summary 审计文件
// @Description 审计文件
// @Accept  json
// @Produce  json
// @Param body body request.AuditFileForm true "审计文件"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/file [post]
func (a *Audit) AuditFile(c *gin.Context) {
	aff := &request.AuditFileForm{}
	err := c.ShouldBindBodyWith(aff, binding.JSON)
	if err != nil {
		// 报文异常同样按静默 200 处理（不落库、不泄露原因），保持响应形态单一
		global.Logger.Warnf("audit file rejected: malformed body: %v", err)
		a.silentSuccess(c)
		return
	}
	if !a.verifyAuditIdentity(c, aff.Id, aff.Uuid) {
		a.silentSuccess(c)
		return
	}
	//ttt := &gin.H{}
	//c.ShouldBindBodyWith(ttt, binding.JSON)
	//fmt.Println(ttt)
	af := aff.ToAuditFile()
	service.AllService.AuditService.CreateAuditFile(af)
	response.Success(c, "")
}
