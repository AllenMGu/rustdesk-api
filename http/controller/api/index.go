package api

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	requstform "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// heartbeatRateLimitPerDevice 每设备 ID 每分钟允许的心跳次数。
// 合法客户端约 15s 一次心跳（4 次/分），10 次/分留有余量；
// 按设备 ID 而非 IP 限流，避免同一 NAT 后的多个设备互相影响。
const heartbeatRateLimitPerDevice = 10

// heartbeatRateLimitPerIP 每来源 IP 每分钟允许的心跳请求数（前置保护）。
// 身份校验（一次索引点查）在限流之后执行，此限流必须在 DB 查询之前，
// 否则匿名伪造者可以无限次触发 DB 查询 + WARN 日志（日志型 DoS）。
// 阈值按 NAT 场景取：单 IP 后最多数千台设备 × 4 次/分，5000 留 5 倍余量；
// 合法设备再叠加每设备 10 次/分的精细限流，两层互不冲突。
const heartbeatRateLimitPerIP = 5000

// heartbeatRejectLogSample 身份拒绝日志采样分母：大规模伪造时按 1/50
// 采样记录 WARN，避免攻击本身造成日志洪泛。
const heartbeatRejectLogSample = 50

var heartbeatRejectCounter atomic.Int64

type Index struct {
}

// Index 首页
// @Tags 首页
// @Summary 首页
// @Description 首页
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router / [get]
func (i *Index) Index(c *gin.Context) {
	response.Success(
		c,
		"Hello Gwen",
	)
}

// Heartbeat 心跳
// @Tags 首页
// @Summary 心跳
// @Description 心跳
// @Accept  json
// @Produce  json
// @Success 200 {object} nil
// @Failure 500 {object} response.Response
// @Router /heartbeat [post]
func (i *Index) Heartbeat(c *gin.Context) {
	info := &requstform.PeerInfoInHeartbeat{}
	err := c.ShouldBindJSON(info)
	if err != nil {
		// 与既有客户端契约一致：心跳端点所有路径均静默 200，不泄露解析细节
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	// IP 级前置限流（纯内存，先于任何 DB 查询与日志）：
	// 身份校验是 DB 查询、拒绝路径会写 WARN 日志，若限流在其之后，
	// 匿名伪造者可用随机 (id, uuid) 无限次触发 DB 查询 + 日志洪泛
	if !global.RateLimiter.Allow("heartbeat-ip:"+c.ClientIP(), heartbeatRateLimitPerIP, time.Minute) {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	// 规范化后校验身份字段：长度超限直接拒绝（与 sysinfo/audit 一致，不截断）
	id := strings.TrimSpace(info.Id)
	uuid := strings.TrimSpace(info.Uuid)
	if verr := requstform.ValidatePeerIdentity(id, uuid); verr != nil {
		// 采样日志：拒绝可能来自批量伪造，1/50 采样避免攻击造成日志洪泛
		if heartbeatRejectCounter.Add(1)%heartbeatRejectLogSample == 1 {
			global.Logger.Warnf("heartbeat rejected (sampled): invalid identity id=%q ip=%s", info.Id, c.ClientIP())
		}
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	// 设备身份校验（只读，绝不写库）：(id, uuid) 必须与已注册设备精确一致。
	// 修复：此前只判断 uuid 非空、按 id 查库即更新，攻击者知道设备 ID 后
	// 用任意非空 uuid 即可伪造心跳、污染 LastOnlineTime/LastOnlineIp
	//（WebClient 的在线状态判断依赖 LastOnlineTime）。
	if !service.AllService.PeerService.VerifyDeviceIdentity(id, uuid) {
		if heartbeatRejectCounter.Add(1)%heartbeatRejectLogSample == 1 {
			global.Logger.Warnf("heartbeat rejected (sampled): device identity mismatch id=%s ip=%s", id, c.ClientIP())
		}
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	// 按设备 ID 限流：合法心跳约 15s 一次，10 次/分足够且不影响 NAT 共享 IP
	if !global.RateLimiter.Allow("heartbeat:"+id, heartbeatRateLimitPerDevice, time.Minute) {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	peer := service.AllService.PeerService.FindById(id)
	if peer == nil || peer.RowId == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	//如果在40s以内则不更新
	if time.Now().Unix()-peer.LastOnlineTime >= 30 {
		upp := &model.Peer{RowId: peer.RowId, LastOnlineTime: time.Now().Unix(), LastOnlineIp: c.ClientIP()}
		service.AllService.PeerService.Update(upp)
	}
	c.JSON(http.StatusOK, gin.H{})
}

// Version 版本
// @Tags 首页
// @Summary 版本
// @Description 版本
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /version [get]
func (i *Index) Version(c *gin.Context) {
	//读取resources/version文件
	v := service.AllService.AppService.GetAppVersion()
	response.Success(
		c,
		v,
	)
}
