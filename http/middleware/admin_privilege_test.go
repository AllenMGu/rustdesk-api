package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
)

// stubGlobals 设置 TranslateMsg 依赖的全局变量（i18n 与日志），并注册恢复
func stubGlobals(t *testing.T) {
	t.Helper()
	previousAllService := service.AllService
	previousLogger := global.Logger
	previousLocalizer := global.Localizer

	service.AllService = &service.Service{}
	global.Logger = logrus.New()
	global.Localizer = func(lang string) *i18n.Localizer {
		return i18n.NewLocalizer(i18n.NewBundle(language.English), "en")
	}

	t.Cleanup(func() {
		service.AllService = previousAllService
		global.Logger = previousLogger
		global.Localizer = previousLocalizer
	})
}

// TestAdminPrivilegeRejectsNonAdmin 普通登录用户（非管理员）必须被拒绝
func TestAdminPrivilegeRejectsNonAdmin(t *testing.T) {
	stubGlobals(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		isAdmin := false
		c.Set("curUser", &model.User{IsAdmin: &isAdmin})
		c.Next()
	})
	engine.GET("/probe", AdminPrivilege(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 wrapper, got %d", w.Code)
	}
	var body response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json body %q: %v", w.Body.String(), err)
	}
	if body.Code != http.StatusForbidden {
		t.Fatalf("expected code 403 for non-admin, got %d", body.Code)
	}
}

// TestAdminPrivilegeAllowsAdmin 管理员放行
func TestAdminPrivilegeAllowsAdmin(t *testing.T) {
	stubGlobals(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		isAdmin := true
		c.Set("curUser", &model.User{IsAdmin: &isAdmin})
		c.Next()
	})
	engine.GET("/probe", AdminPrivilege(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Body.String(); got != `{"ok":true}` {
		t.Fatalf("expected stub handler to run, body %q", got)
	}
}

// TestAdminPrivilegeRejectsAnonymous 未登录（无 curUser）必须被拒绝
func TestAdminPrivilegeRejectsAnonymous(t *testing.T) {
	stubGlobals(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/probe", AdminPrivilege(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))

	var body response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json body %q: %v", w.Body.String(), err)
	}
	if body.Code != http.StatusForbidden {
		t.Fatalf("expected code 403 for anonymous, got %d", body.Code)
	}
}
