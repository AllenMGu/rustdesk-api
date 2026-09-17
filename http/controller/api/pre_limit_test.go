package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// 本文件覆盖第四轮复审 P1（卡合并）：匿名端点的 IP 限流必须位于 JSON 解码
// 之前——限流在 bind 之后时，攻击者持续发送 <128KB 的非法 JSON 即可完全
// 绕过配额（audit 端点每次还多一条 WARN 日志）。以下测试以"耗尽配额后
// 合法请求必须被拒绝且不落库/不写状态"作为可观察断言。

// TestAuditMalformedBodyConsumesRateLimit 畸形报文必须消耗 audit IP 配额：
// 连续 auditRateLimit 次畸形 body（每次仍静默 200）后，合法审计也必须
// 被限流拒绝且不得写入 audit_conns。修复前该测试失败（畸形不消耗配额，
// 合法请求照常落库）。
func TestAuditMalformedBodyConsumesRateLimit(t *testing.T) {
	setupSecurityTestEnv(t, "file:audit_pre_rl?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev1", Uuid: "real-uuid"})
	engine := newAuditEngine()

	for i := 0; i < auditRateLimit; i++ {
		w := postJSON(t, engine, "/api/audit/conn", `{malformed`)
		if w.Code != http.StatusOK {
			t.Fatalf("malformed body #%d must be silent 200, got %d %q", i, w.Code, w.Body.String())
		}
	}
	// 配额已耗尽：合法审计同样被拒，且不得落库
	w := postJSON(t, engine, "/api/audit/conn",
		`{"action":"new","conn_id":9991,"id":"dev1","uuid":"real-uuid","peer":["x"],"type":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("rate-limited audit must stay silent 200, got %d %q", w.Code, w.Body.String())
	}
	var count int64
	service.DB.Model(&model.AuditConn{}).Where("conn_id = 9991").Count(&count)
	if count != 0 {
		t.Fatal("rate-limited audit must NOT be written to DB")
	}
}

// TestHeartbeatMalformedBodyConsumesRateLimit heartbeat 的 IP 前置限流
// 必须先于 ShouldBindJSON：畸形 body 耗尽配额后，合法心跳也必须被拒，
// 且不得刷新 LastOnlineTime/LastOnlineIp。
func TestHeartbeatMalformedBodyConsumesRateLimit(t *testing.T) {
	setupSecurityTestEnv(t, "file:hb_pre_rl?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "hbdev", Uuid: "real-uuid", LastOnlineTime: 0})
	engine := newHeartbeatEngine()

	for i := 0; i < heartbeatRateLimitPerIP; i++ {
		w := postJSON(t, engine, "/api/heartbeat", `{broken`)
		if w.Code != http.StatusOK {
			t.Fatalf("malformed heartbeat #%d must be silent 200, got %d", i, w.Code)
		}
	}
	w := postJSON(t, engine, "/api/heartbeat", `{"id":"hbdev","uuid":"real-uuid"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("rate-limited heartbeat must be silent 200, got %d", w.Code)
	}
	pe := service.AllService.PeerService.FindById("hbdev")
	if pe.LastOnlineTime != 0 {
		t.Errorf("rate-limited heartbeat must not update LastOnlineTime, got %d", pe.LastOnlineTime)
	}
	if pe.LastOnlineIp != "" {
		t.Errorf("rate-limited heartbeat must not update LastOnlineIp, got %q", pe.LastOnlineIp)
	}
}

// TestSysInfoMalformedBodyConsumesRateLimit sysinfo 的 30/min 前置限流
// 必须先于 ShouldBindBodyWith：畸形 body 耗尽配额后，合法请求也必须先
// 收到 400 TooManyRequests（而不是进入身份/落库逻辑）。
func TestSysInfoMalformedBodyConsumesRateLimit(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_pre_rl?mode=memory&cache=shared")
	engine := newSysInfoEngine()

	for i := 0; i < sysInfoRateLimit; i++ {
		w := postJSON(t, engine, "/api/sysinfo", `{bad`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("malformed sysinfo #%d must be 400, got %d", i, w.Code)
		}
	}
	w := postJSON(t, engine, "/api/sysinfo", `{"id":"newdev","uuid":"u-123"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("rate-limited sysinfo must be 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "TooManyRequests") {
		t.Fatalf("expected TooManyRequests message, got %q", w.Body.String())
	}
}

// TestSharedPeerRejectsOversizedToken 第四轮复审 P2：share_token 长度上限。
// 实际 token 为 UUID（36 字符），超限必须直接拒绝（code 101），不得进入 DB 查询。
func TestSharedPeerRejectsOversizedToken(t *testing.T) {
	engine := newSharedPeerEngine()
	long := strings.Repeat("x", shareTokenMaxLen+1)
	w := postJSON(t, engine, "/api/shared-peer", `{"share_token":"`+long+`"}`)
	if w.Code == 500 {
		t.Fatalf("oversized token must not 500: %q", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":101`) {
		t.Fatalf("expected business error code 101, got %d %q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid share_token") {
		t.Fatalf("expected invalid-share_token rejection, got %q", w.Body.String())
	}
}

// TestSharedPeerRateLimitIsEnforced 第四轮复审 P2：shared-peer 每 IP
// 60/min 限流必须真实生效——前 60 次放行（share not found 101），
// 第 61 次被限流：仍是 code 101 业务错误形态（客户端契约不变、不 500），
// 但不再触发 DB 查询。
func TestSharedPeerRateLimitIsEnforced(t *testing.T) {
	setupSecurityTestEnv(t, "file:sp_pre_rl?mode=memory&cache=shared")
	engine := newSharedPeerEngine()
	body := `{"share_token":"00000000-0000-0000-0000-000000000000"}`

	for i := 0; i < sharedPeerRateLimitPerIP; i++ {
		w := postJSON(t, engine, "/api/shared-peer", body)
		if w.Code == 500 {
			t.Fatalf("request #%d: must not 500: %q", i, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":101`) {
			t.Fatalf("request #%d: expected code 101, got %d %q", i, w.Code, w.Body.String())
		}
	}
	w := postJSON(t, engine, "/api/shared-peer", body)
	if w.Code == 500 {
		t.Fatalf("rate-limited shared-peer must not 500: %q", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":101`) {
		t.Fatalf("rate-limited shared-peer must keep code 101 shape, got %d %q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "too many requests") {
		t.Fatalf("expected rate-limit rejection, got %q", w.Body.String())
	}
}
