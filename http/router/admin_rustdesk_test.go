package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupRouterTestEnv 为路由测试准备最小环境：in-memory sqlite 与全局依赖
func setupRouterTestEnv(t *testing.T, dsn string) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.ServerCmd{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	previousDB := service.DB
	previousAllService := service.AllService
	previousLogger := global.Logger
	previousLocalizer := global.Localizer

	service.DB = db
	service.AllService = &service.Service{}
	global.Logger = logrus.New()
	global.Localizer = func(lang string) *i18n.Localizer {
		return i18n.NewLocalizer(i18n.NewBundle(language.English), "en")
	}

	t.Cleanup(func() {
		service.DB = previousDB
		service.AllService = previousAllService
		global.Logger = previousLogger
		global.Localizer = previousLocalizer
		_ = sqlDB.Close()
	})
}

// TestRustdeskCmdBindDeniesNonAdmin 验证 /api/admin/rustdesk/* 的真实路由链：
// 普通登录用户（非管理员）被 AdminPrivilege 拒绝，无法执行服务器级命令
func TestRustdeskCmdBindDeniesNonAdmin(t *testing.T) {
	setupRouterTestEnv(t, "file:router_nonadmin?mode=memory&cache=shared")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// 模拟 /api/admin 组的 BackendUserAuth：登录用户（非管理员）
	adg := engine.Group("/api/admin")
	adg.Use(func(c *gin.Context) {
		isAdmin := false
		c.Set("curUser", &model.User{IsAdmin: &isAdmin})
	})
	RustdeskCmdBind(adg)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/admin/rustdesk/cmdList", ""},
		{http.MethodPost, "/api/admin/rustdesk/sendCmd", `{"cmd":"x","option":"y","target":"z"}`},
		{http.MethodPost, "/api/admin/rustdesk/cmdDelete", `{"id":1}`},
		{http.MethodPost, "/api/admin/rustdesk/cmdCreate", `{"id":0}`},
	} {
		w := httptest.NewRecorder()
		var req *http.Request
		if tc.body != "" {
			req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		} else {
			req = httptest.NewRequest(tc.method, tc.path, nil)
		}
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)

		var body response.Response
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s: invalid body %q: %v", tc.method, tc.path, w.Body.String(), err)
		}
		if body.Code != 403 {
			t.Errorf("%s %s: 非管理员应被拒绝 (code 403)，实际 code=%d body=%q", tc.method, tc.path, body.Code, w.Body.String())
		}
	}
}

// TestRustdeskCmdBindAllowsAdmin 验证管理员可正常访问（中间件放行）
func TestRustdeskCmdBindAllowsAdmin(t *testing.T) {
	setupRouterTestEnv(t, "file:router_admin?mode=memory&cache=shared")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	adg := engine.Group("/api/admin")
	adg.Use(func(c *gin.Context) {
		isAdmin := true
		c.Set("curUser", &model.User{IsAdmin: &isAdmin})
	})
	RustdeskCmdBind(adg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/rustdesk/cmdList", nil)
	engine.ServeHTTP(w, req)

	var body response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid body %q: %v", w.Body.String(), err)
	}
	if body.Code != 0 {
		t.Fatalf("管理员应被放行 (code 0)，实际 code=%d body=%q", body.Code, w.Body.String())
	}
}
