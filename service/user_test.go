package service

import (
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	log "github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupLoginTestEnv(t *testing.T, dsn string) {
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
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.LoginLog{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	prevDB, prevConfig, prevJwt, prevLogger, prevAll := DB, Config, Jwt, Logger, AllService
	DB = db
	Config = &config.Config{}
	Jwt = &jwt.Jwt{} // 空 Key → 走 crypto/rand 不透明 token 路径
	Logger = log.New()
	AllService = &Service{} // 嵌入 nil 服务指针，方法走包级 DB 全局（包内惯例）
	t.Cleanup(func() {
		DB, Config, Jwt, Logger, AllService = prevDB, prevConfig, prevJwt, prevLogger, prevAll
		_ = sqlDB.Close()
	})
}

// TestLoginSucceedsAndLinksToken 正常路径：token 行与登录日志同事务落库，
// 且 login_logs.user_token_id 指向具体 token 行（ut.Id）
func TestLoginSucceedsAndLinksToken(t *testing.T) {
	setupLoginTestEnv(t, "file:login_ok?mode=memory&cache=shared")
	u := &model.User{Username: "tester"}
	if err := DB.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	us := &UserService{}
	llog := &model.LoginLog{UserId: u.Id, Client: model.LoginLogClientApp, Ip: "1.2.3.4", Uuid: "u-1", DeviceId: "d-1"}

	ut, err := us.Login(u, llog)
	if err != nil {
		t.Fatalf("login should succeed: %v", err)
	}
	if ut == nil || ut.Id == 0 || ut.Token == "" {
		t.Fatal("expected a concrete token row with non-empty token")
	}
	var stored *model.UserToken
	DB.Where("id = ?", ut.Id).First(&stored)
	if stored == nil || stored.Id == 0 {
		t.Fatal("token row must be persisted")
	}
	var storedLog *model.LoginLog
	DB.Where("user_id = ?", 1).First(&storedLog)
	if storedLog == nil || storedLog.Id == 0 {
		t.Fatal("login log must be persisted")
	}
	if storedLog.UserTokenId != ut.Id {
		t.Errorf("login log must reference the concrete token row, got %d want %d", storedLog.UserTokenId, ut.Id)
	}
}

// TestLoginFailsClosedWhenDBInsertFails 审查 P2：token 生成成功但 DB 写入
// 失败时，必须整体失败关闭——不返回 token、user_tokens 与 login_logs 都
// 不得留下半截数据。故障注入：触发器强制 login_logs INSERT 失败。
func TestLoginFailsClosedWhenDBInsertFails(t *testing.T) {
	setupLoginTestEnv(t, "file:login_fail?mode=memory&cache=shared")
	u := &model.User{Username: "tester"}
	if err := DB.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := DB.Exec("CREATE TRIGGER fail_login_log BEFORE INSERT ON login_logs " +
		"BEGIN SELECT RAISE(ABORT, 'injected-fault'); END;").Error; err != nil {
		t.Fatalf("create fault trigger: %v", err)
	}

	us := &UserService{}
	llog := &model.LoginLog{UserId: u.Id, Client: model.LoginLogClientApp, Ip: "1.2.3.4"}

	ut, err := us.Login(u, llog)
	if err == nil {
		t.Fatal("expected error when the login log insert fails")
	}
	if ut != nil {
		t.Fatal("expected nil token on failure")
	}
	var tokens, logs int64
	DB.Model(&model.UserToken{}).Count(&tokens)
	DB.Model(&model.LoginLog{}).Count(&logs)
	if tokens != 0 {
		t.Errorf("user_tokens must be rolled back, got %d rows", tokens)
	}
	if logs != 0 {
		t.Errorf("login_logs must be rolled back, got %d rows", logs)
	}
}
