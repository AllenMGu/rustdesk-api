package api

import (
	"sync/atomic"
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

// auditRateLimit 每 IP 每分钟允许的审计请求次数
// （客户端在连接建立/关闭、文件传输时上报，正常频率有限）
const auditRateLimit = 120

// auditRejectLogSample 审计端点拒绝/畸形报文日志采样分母：
// 攻击者高频发送畸形报文时按 1/50 记录 WARN，防止攻击本身造成日志洪泛
const auditRejectLogSample = 50

var auditRejectCounter atomic.Int64

// auditSampledWarn 按 1/auditRejectLogSample 采样输出 WARN
func auditSampledWarn(format string, args ...interface{}) {
	if auditRejectCounter.Add(1)%auditRejectLogSample == 1 {
		global.Logger.Warnf(format, args...)
	}
}

// auditIpAllowed IP 级前置限流：必须在任何 JSON 解码之前执行。
// 此前限流在 ShouldBindBodyWith 之后——攻击者持续发送 <128KB 的非法
// JSON 即可完全绕过 120/min 配额（每次还产生一条 WARN 日志）；
// 前置后畸形报文同样消耗配额。拒绝时静默 200（不泄露原因）。
func (a *Audit) auditIpAllowed(c *gin.Context) bool {
	if global.RateLimiter.Allow("audit:"+c.ClientIP(), auditRateLimit, time.Minute) {
		return true
	}
	auditSampledWarn("audit rejected: rate limited ip=%s", c.ClientIP())
	return false
}

// verifyAuditIdentity 校验匿名审计上报的设备身份。
// 官方 RustDesk 客户端匿名 POST /api/audit/conn|file 时携带 id 与 uuid，
// (id, uuid) 是同一台机器上稳定成对出现的标识，只有已知设备才能写入审计记录，
// 防止匿名来源伪造审计日志或批量写入造成存储型 DoS。
//
// IP 限流由调用方在 JSON 解码前执行（auditIpAllowed），本方法只做身份校验。
//
// 本方法不写任何响应：审计端点对客户端保持“静默 200、不落库”的契约，
// 无论拒绝原因是限流、身份不符还是报文异常，响应形态都相同，
// 不向攻击方泄露拒绝原因。响应统一由调用方（AuditConn/AuditFile）写一次。
func (a *Audit) verifyAuditIdentity(c *gin.Context, peerId, uuid string) bool {
	clientIp := c.ClientIP()
	// id 长度规范化：超限直接拒绝（而不是截断后查询，避免身份校验失配）
	if verr := request.ValidatePeerIdentity(peerId, uuid); verr != nil {
		auditSampledWarn("audit rejected: malformed identity id=%s ip=%s (%v)", peerId, clientIp, verr)
		return false
	}
	if !service.AllService.PeerService.VerifyDeviceIdentity(peerId, uuid) {
		auditSampledWarn("audit rejected: unknown device identity id=%s ip=%s", peerId, clientIp)
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
	// IP 级前置限流：先于 JSON 解码，畸形报文同样消耗配额
	if !a.auditIpAllowed(c) {
		a.silentSuccess(c)
		return
	}
	af := &request.AuditConnForm{}
	err := c.ShouldBindBodyWith(af, binding.JSON)
	if err != nil {
		// 报文异常同样按静默 200 处理（不落库、不泄露原因），保持响应形态单一；
		// 日志 1/50 采样，防止畸形报文洪水造成日志洪泛
		auditSampledWarn("audit conn rejected: malformed body: %v", err)
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
	// IP 级前置限流：先于 JSON 解码，畸形报文同样消耗配额
	if !a.auditIpAllowed(c) {
		a.silentSuccess(c)
		return
	}
	aff := &request.AuditFileForm{}
	err := c.ShouldBindBodyWith(aff, binding.JSON)
	if err != nil {
		// 报文异常同样按静默 200 处理（不落库、不泄露原因），保持响应形态单一；
		// 日志 1/50 采样，防止畸形报文洪水造成日志洪泛
		auditSampledWarn("audit file rejected: malformed body: %v", err)
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
