package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

func TestCorsReflectsOnlyAllowedOrigins(t *testing.T) {
	previousConfig := global.Config
	t.Cleanup(func() {
		global.Config = previousConfig
	})
	global.Config.Gin.CorsOrigins = []string{"https://console.example.com"}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Cors())
	engine.GET("/probe", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	do := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	// 白名单内的 Origin：精确反射
	w := do("https://console.example.com")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example.com" {
		t.Errorf("allowed origin should be reflected, got %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("credentials should be allowed for whitelisted origin")
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Error("Vary: Origin expected")
	}

	// 白名单外的 Origin：不得反射
	w = do("https://evil.example.com")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unlisted origin must not be reflected, got %q", got)
	}
	// 前缀/相似 Origin：不得反射（必须精确匹配）
	w = do("https://console.example.com.evil.com")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("prefix-similar origin must not be reflected, got %q", got)
	}

	// 无 Origin：不产生跨域头
	w = do("")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("no origin request must not get ACAO, got %q", got)
	}
}

func TestCorsEmptyAllowlistBlocksAll(t *testing.T) {
	previousConfig := global.Config
	t.Cleanup(func() {
		global.Config = previousConfig
	})
	global.Config.Gin.CorsOrigins = nil

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Cors())
	engine.GET("/probe", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	engine.OPTIONS("/probe", func(c *gin.Context) {
		c.String(http.StatusOK, "should-not-reach")
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Origin", "https://console.example.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("empty allowlist must not reflect any origin, got %q", got)
	}

	// OPTIONS 预检：204 且无放行头
	req = httptest.NewRequest(http.MethodOptions, "/probe", nil)
	req.Header.Set("Origin", "https://console.example.com")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("preflight should return 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("preflight must not allow unlisted origin, got %q", got)
	}
}

// TestCorsPreflightAllowMethods 预检响应必须回显可用方法列表，
// 不能回显预检请求自身的方法（OPTIONS），否则浏览器会拒绝后续实际请求
func TestCorsPreflightAllowMethods(t *testing.T) {
	previousConfig := global.Config
	t.Cleanup(func() {
		global.Config = previousConfig
	})
	global.Config.Gin.CorsOrigins = []string{"https://console.example.com"}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Cors())
	engine.GET("/probe", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodOptions, "/probe", nil)
	req.Header.Set("Origin", "https://console.example.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight should return 204, got %d", w.Code)
	}
	allowMethods := w.Header().Get("Access-Control-Allow-Methods")
	if allowMethods == http.MethodOptions {
		t.Fatal("Allow-Methods must not echo the preflight method OPTIONS")
	}
	for _, m := range []string{"GET", "POST", "PUT", "DELETE"} {
		if !strings.Contains(allowMethods, m) {
			t.Errorf("Allow-Methods should include %s, got %q", m, allowMethods)
		}
	}
}
