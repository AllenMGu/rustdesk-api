package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

func newHeartbeatEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/heartbeat", (&Index{}).Heartbeat)
	return engine
}

// TestHeartbeatAcceptsMatchingIdentity 身份一致的心跳正常更新在线状态
func TestHeartbeatAcceptsMatchingIdentity(t *testing.T) {
	setupSecurityTestEnv(t, "file:hb_ok?mode=memory&cache=shared")
	before := time.Now().Unix() - 3600 // 旧时间，确保 >=30s 门限会被更新
	service.DB.Create(&model.Peer{Id: "hbdev", Uuid: "hb-uuid", LastOnlineTime: before})

	w := postJSON(t, newHeartbeatEngine(), "/api/heartbeat",
		`{"id":"hbdev","uuid":"hb-uuid"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	pe := service.AllService.PeerService.FindById("hbdev")
	if pe.LastOnlineTime < before || pe.LastOnlineTime > time.Now().Unix()+1 {
		t.Errorf("LastOnlineTime should be refreshed, got %d", pe.LastOnlineTime)
	}
	if pe.LastOnlineIp == "" {
		t.Error("LastOnlineIp should be recorded")
	}
}

// TestHeartbeatRejectsForgedUuid 核心回归：知道设备 ID + 任意非空 uuid
// 不得伪造心跳、污染 LastOnlineTime/LastOnlineIp（审查 P1 阻塞项）。
// 响应保持静默 200（客户端契约），但数据库不得有任何变化。
func TestHeartbeatRejectsForgedUuid(t *testing.T) {
	setupSecurityTestEnv(t, "file:hb_forge?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "hbdev", Uuid: "real-uuid", LastOnlineTime: 0})

	w := postJSON(t, newHeartbeatEngine(), "/api/heartbeat",
		`{"id":"hbdev","uuid":"attacker-uuid-123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("rejection must stay silent 200, got %d body=%q", w.Code, w.Body.String())
	}
	pe := service.AllService.PeerService.FindById("hbdev")
	if pe.LastOnlineTime != 0 {
		t.Errorf("forged heartbeat must not update LastOnlineTime, got %d", pe.LastOnlineTime)
	}
	if pe.LastOnlineIp != "" {
		t.Errorf("forged heartbeat must not update LastOnlineIp, got %q", pe.LastOnlineIp)
	}
	if pe.Uuid != "real-uuid" {
		t.Errorf("bound uuid must not change, got %q", pe.Uuid)
	}
}

// TestHeartbeatRejectsEmptyUuidAndUnknown 空 uuid / 未知设备 → 静默 200 且无副作用
func TestHeartbeatRejectsEmptyUuidAndUnknown(t *testing.T) {
	setupSecurityTestEnv(t, "file:hb_empty?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "hbdev", Uuid: "real-uuid"})

	w1 := postJSON(t, newHeartbeatEngine(), "/api/heartbeat", `{"id":"hbdev","uuid":""}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("empty uuid must be silent 200, got %d", w1.Code)
	}
	w2 := postJSON(t, newHeartbeatEngine(), "/api/heartbeat", `{"id":"ghost","uuid":"whatever"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("unknown device must be silent 200, got %d", w2.Code)
	}
}

// TestHeartbeatRejectsOversizedIdentity 超长 id → 静默 200（不是 500）
func TestHeartbeatRejectsOversizedIdentity(t *testing.T) {
	setupSecurityTestEnv(t, "file:hb_oversize?mode=memory&cache=shared")
	// 200 字符 id，超过 PeerIdLimit=128
	longId := ""
	for i := 0; i < 200; i++ {
		longId += "a"
	}
	w := postJSON(t, newHeartbeatEngine(), "/api/heartbeat",
		`{"id":"`+longId+`","uuid":"u"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("oversized id must be silent 200, got %d body=%q", w.Code, w.Body.String())
	}
}
