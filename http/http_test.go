package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// whoamiEngine 构造一个回显 ClientIP() 的引擎
func whoamiEngine(trustProxy string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	g := gin.New()
	applyTrustedProxyConfig(g, trustProxy)
	g.GET("/whoami", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
	return g
}

// TestTrustedProxyEmptyIgnoresXFF 未配置 trust-proxy 时，
// 伪造的 X-Forwarded-For / X-Real-IP 不得影响 ClientIP()
func TestTrustedProxyEmptyIgnoresXFF(t *testing.T) {
	engine := whoamiEngine("")

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.RemoteAddr = "203.0.113.7:51234"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.1.2.3")
	req.Header.Set("X-Real-IP", "198.51.100.9")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Body.String(); got != "203.0.113.7" {
		t.Fatalf("with no trusted proxies ClientIP must be the remote addr 203.0.113.7, got %q", got)
	}
}

// TestTrustedProxyExplicitListHonorsXFF 显式配置受信任代理段时，
// 来自该段的请求其 X-Forwarded-For 才被采信
func TestTrustedProxyExplicitListHonorsXFF(t *testing.T) {
	engine := whoamiEngine("203.0.113.0/24")

	// 远端在信任段内：采信 XFF 中第一个不可信 IP
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.RemoteAddr = "203.0.113.7:51234"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if got := w.Body.String(); got != "198.51.100.9" {
		t.Fatalf("trusted proxy should forward XFF client 198.51.100.9, got %q", got)
	}

	// 远端不在信任段内：即使带 XFF 也不采信
	req = httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.RemoteAddr = "192.0.2.5:51234"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if got := w.Body.String(); got != "192.0.2.5" {
		t.Fatalf("untrusted remote must not honor XFF, expected 192.0.2.5, got %q", got)
	}
}
