package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestBodyLimit 超限请求体必须在读取时失败（JSON 绑定随之报参数错误），
// 小请求体不受影响
func TestBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var readErr error
	engine.POST("/probe", BodyLimit(64), func(c *gin.Context) {
		_, readErr = io.ReadAll(c.Request.Body)
		if readErr != nil {
			c.String(http.StatusBadRequest, "too large")
			return
		}
		c.String(http.StatusOK, "ok")
	})

	// 超限：读取报错
	req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(strings.Repeat("a", 128)))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("oversized body must fail the read, got %d", w.Code)
	}
	if readErr == nil {
		t.Error("reading over the limit must return an error")
	}

	// 未超限：正常通过
	req = httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(strings.Repeat("a", 16)))
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("small body must pass, got %d %q", w.Code, w.Body.String())
	}
	if readErr != nil {
		t.Errorf("small body must not fail the read: %v", readErr)
	}
}
