package api

import (
	"bytes"
	"net/http"
	"strings"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupSecurityTestEnv 为控制器安全测试准备最小运行环境：
// in-memory sqlite、限流器、日志与 i18n 桩
func setupSecurityTestEnv(t *testing.T, dsn string) {
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
	if err := db.AutoMigrate(&model.Peer{}, &model.LoginLog{}, &model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	previousDB := service.DB
	previousAllService := service.AllService
	previousRateLimiter := global.RateLimiter
	previousLogger := global.Logger
	previousLocalizer := global.Localizer

	service.DB = db
	service.AllService = &service.Service{}
	global.RateLimiter = utils.NewRateLimiter()
	global.Logger = logrus.New()
	global.Localizer = func(lang string) *i18n.Localizer {
		return i18n.NewLocalizer(i18n.NewBundle(language.English), "en")
	}

	t.Cleanup(func() {
		service.DB = previousDB
		service.AllService = previousAllService
		global.RateLimiter = previousRateLimiter
		global.Logger = previousLogger
		global.Localizer = previousLocalizer
		_ = sqlDB.Close()
	})
}

func postJSON(t *testing.T, engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func newSysInfoEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/sysinfo", (&Peer{}).SysInfo)
	return engine
}

func newAuditEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/audit/conn", (&Audit{}).AuditConn)
	engine.POST("/api/audit/file", (&Audit{}).AuditFile)
	return engine
}

// TestSysInfoRejectsUuidMismatch 已绑定 uuid 的设备，uuid 不符时必须拒绝写入
func TestSysInfoRejectsUuidMismatch(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_mismatch?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev1", Uuid: "real-uuid", Hostname: "real-host"})

	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo",
		`{"id":"dev1","uuid":"fake-uuid","hostname":"evil"}`)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%q", w.Code, w.Body.String())
	}
	if w.Body.String() == "SYSINFO_UPDATED" {
		t.Fatal("mismatched uuid must not be reported as updated")
	}
	pe := service.AllService.PeerService.FindById("dev1")
	if pe.Hostname != "real-host" {
		t.Errorf("hostname must not be overwritten, got %q", pe.Hostname)
	}
	if pe.Uuid != "real-uuid" {
		t.Errorf("bound uuid must not be replaced, got %q", pe.Uuid)
	}
}

// TestSysInfoAcceptsMatchingUuid uuid 一致时正常更新并返回 SYSINFO_UPDATED
func TestSysInfoAcceptsMatchingUuid(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_match?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev1", Uuid: "real-uuid", Hostname: "old-host"})

	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo",
		`{"id":"dev1","uuid":"real-uuid","hostname":"new-host"}`)

	if w.Code != http.StatusOK || w.Body.String() != "SYSINFO_UPDATED" {
		t.Fatalf("expected 200 SYSINFO_UPDATED, got %d %q", w.Code, w.Body.String())
	}
	pe := service.AllService.PeerService.FindById("dev1")
	if pe.Hostname != "new-host" {
		t.Errorf("hostname should be updated, got %q", pe.Hostname)
	}
}

// TestSysInfoBindsUnboundPeer 未绑定 uuid 的设备首次上报时绑定 uuid
func TestSysInfoBindsUnboundPeer(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_bind?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev2"})

	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo",
		`{"id":"dev2","uuid":"uuid-X","hostname":"h2"}`)
	if w.Code != http.StatusOK || w.Body.String() != "SYSINFO_UPDATED" {
		t.Fatalf("expected 200 SYSINFO_UPDATED, got %d %q", w.Code, w.Body.String())
	}
	pe := service.AllService.PeerService.FindById("dev2")
	if pe.Uuid != "uuid-X" {
		t.Errorf("uuid should be bound, got %q", pe.Uuid)
	}

	// 绑定后其它 uuid 被拒绝
	w = postJSON(t, newSysInfoEngine(), "/api/sysinfo",
		`{"id":"dev2","uuid":"uuid-Y","hostname":"h3"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 after binding, got %d", w.Code)
	}
}

// TestSysInfoCreatesUnknownPeer 未知设备（首次注册）允许创建
func TestSysInfoCreatesUnknownPeer(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_new?mode=memory&cache=shared")

	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo",
		`{"id":"newdev","uuid":"uuid-N","hostname":"h"}`)
	if w.Code != http.StatusOK || w.Body.String() != "SYSINFO_UPDATED" {
		t.Fatalf("expected 200 SYSINFO_UPDATED, got %d %q", w.Code, w.Body.String())
	}
	pe := service.AllService.PeerService.FindById("newdev")
	if pe.RowId == 0 {
		t.Fatal("peer should be created")
	}
	if pe.Uuid != "uuid-N" {
		t.Errorf("peer uuid wrong: %q", pe.Uuid)
	}
}

// TestSysInfoTruncatesOversizedFields 超长字段被截断，防止存储膨胀
func TestSysInfoTruncatesOversizedFields(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_trunc?mode=memory&cache=shared")

	long := strings.Repeat("a", 5000)
	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo",
		`{"id":"trunc","uuid":"u","hostname":"`+long+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %q", w.Code, w.Body.String())
	}
	pe := service.AllService.PeerService.FindById("trunc")
	if len(pe.Hostname) > 1024 {
		t.Errorf("hostname should be truncated, len=%d", len(pe.Hostname))
	}
}

// TestSysInfoMissingId 缺少 id 时返回 400
func TestSysInfoMissingId(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_noid?mode=memory&cache=shared")

	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo", `{"uuid":"u"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestSysInfoRejectsCaseMismatchUuid Base64 uuid 大小写敏感：仅大小写不同的 uuid 必须拒绝
func TestSysInfoRejectsCaseMismatchUuid(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_case?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev1", Uuid: "AbC-def_GhI", Hostname: "real-host"})

	// 客户端上报的是 encode64(get_uuid())，Base64 大小写敏感
	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo",
		`{"id":"dev1","uuid":"abc-def_ghi","hostname":"evil"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("case-mismatched Base64 uuid must be rejected with 403, got %d %q", w.Code, w.Body.String())
	}
	pe := service.AllService.PeerService.FindById("dev1")
	if pe.Hostname != "real-host" || pe.Uuid != "AbC-def_GhI" {
		t.Errorf("peer must not be modified on case mismatch, got hostname=%q uuid=%q", pe.Hostname, pe.Uuid)
	}
}

// TestSysInfoRejectsNewDeviceWithoutUuid 新设备注册必须携带非空 uuid
func TestSysInfoRejectsNewDeviceWithoutUuid(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_newnouuid?mode=memory&cache=shared")

	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo", `{"id":"newdev","hostname":"h"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for new device without uuid, got %d %q", w.Code, w.Body.String())
	}
	if pe := service.AllService.PeerService.FindById("newdev"); pe.RowId != 0 {
		t.Error("peer must not be created without uuid")
	}
}

// TestSysInfoRejectsLegacyPeerWithoutUuid 未绑定 uuid 的旧 peer 上报空 uuid 时拒绝且不绑定
func TestSysInfoRejectsLegacyPeerWithoutUuid(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_legacynouuid?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "legacy"})

	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo", `{"id":"legacy","hostname":"h"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d %q", w.Code, w.Body.String())
	}
	if pe := service.AllService.PeerService.FindById("legacy"); pe.Uuid != "" {
		t.Errorf("uuid must not be bound, got %q", pe.Uuid)
	}
}

// TestSysInfoRejectsOversizedId id 超长直接拒绝（而不是截断后查询/建库，避免重复 peer）
func TestSysInfoRejectsOversizedId(t *testing.T) {
	setupSecurityTestEnv(t, "file:sysinfo_longid?mode=memory&cache=shared")

	long := strings.Repeat("a", 200) // 超过 PeerIdLimit=128
	w := postJSON(t, newSysInfoEngine(), "/api/sysinfo",
		`{"id":"`+long+`","uuid":"u"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized id, got %d %q", w.Code, w.Body.String())
	}
	if pe := service.AllService.PeerService.FindById(long); pe.RowId != 0 {
		t.Error("peer must not be created for oversized id")
	}
}

// TestAuditRejectsLegacyPeerWithoutClaim 审计端点对未绑定 uuid 的既有 peer 必须拒绝，
// 且不得借校验路径“抢绑” uuid（攻击者可用任意 uuid 抢占旧 peer 的身份）
func TestAuditRejectsLegacyPeerWithoutClaim(t *testing.T) {
	setupSecurityTestEnv(t, "file:audit_legacy?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "legacy", Hostname: "old"})
	engine := newAuditEngine()

	w := postJSON(t, engine, "/api/audit/conn",
		`{"action":"new","conn_id":7,"id":"legacy","uuid":"attacker-uuid","peer":["x"],"type":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected silent 200, got %d %q", w.Code, w.Body.String())
	}
	var count int64
	service.DB.Model(&model.AuditConn{}).Count(&count)
	if count != 0 {
		t.Error("audit row must not be created for unbound peer")
	}
	pe := service.AllService.PeerService.FindById("legacy")
	if pe.Uuid != "" {
		t.Errorf("audit path must not claim/bind uuid on legacy peer, got %q", pe.Uuid)
	}
	if pe.Hostname != "old" {
		t.Errorf("peer must not be modified, hostname=%q", pe.Hostname)
	}
}

// TestAuditMalformedBodyIsSilent 报文异常按静默 200 处理，不落库、不泄露原因
func TestAuditMalformedBodyIsSilent(t *testing.T) {
	setupSecurityTestEnv(t, "file:audit_malformed?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev1", Uuid: "real-uuid"})
	engine := newAuditEngine()

	w := postJSON(t, engine, "/api/audit/conn", `{invalid json`)
	if w.Code != http.StatusOK {
		t.Fatalf("malformed body must yield silent 200, got %d %q", w.Code, w.Body.String())
	}
	var count int64
	service.DB.Model(&model.AuditConn{}).Count(&count)
	if count != 0 {
		t.Error("audit row must not be created for malformed body")
	}
}

// TestAuditOversizedIdIsSilent 身份字段超长按静默 200 拒绝，不落库
func TestAuditOversizedIdIsSilent(t *testing.T) {
	setupSecurityTestEnv(t, "file:audit_longid?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev1", Uuid: "real-uuid"})
	engine := newAuditEngine()

	long := strings.Repeat("a", 200)
	w := postJSON(t, engine, "/api/audit/conn",
		`{"action":"new","conn_id":9,"id":"`+long+`","uuid":"real-uuid","peer":["x"],"type":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected silent 200, got %d %q", w.Code, w.Body.String())
	}
	var count int64
	service.DB.Model(&model.AuditConn{}).Count(&count)
	if count != 0 {
		t.Error("audit row must not be created for oversized id")
	}
}

// TestAuditRateLimitIsSilent 触发限流后仍是静默 200（单次响应、无泄露），不落库
func TestAuditRateLimitIsSilent(t *testing.T) {
	setupSecurityTestEnv(t, "file:audit_ratelimit?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev1", Uuid: "real-uuid"})
	engine := newAuditEngine()

	const limit = 120 // auditRateLimit
	// 前 limit 次应全部成功写入
	for i := 0; i < limit; i++ {
		w := postJSON(t, engine, "/api/audit/file",
			`{"id":"dev1","uuid":"real-uuid","peer_id":"p","info":"{}"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}
	// 第 limit+1 次被限流：响应必须仍是单个 200（不是 400/429，更不是双段响应）
	w := postJSON(t, engine, "/api/audit/file",
		`{"id":"dev1","uuid":"real-uuid","peer_id":"p","info":"{}"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("rate-limited request must yield silent 200, got %d %q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Count(body, "{") != 1 {
		t.Fatalf("response must be a single JSON document, got %q", body)
	}
	var count int64
	service.DB.Model(&model.AuditFile{}).Count(&count)
	if count != limit {
		t.Errorf("exactly %d rows should be written, got %d", limit, count)
	}
}

// TestAuditRejectsUnknownDevice 未知设备身份不得写入审计记录
func TestAuditRejectsUnknownDevice(t *testing.T) {
	setupSecurityTestEnv(t, "file:audit_unknown?mode=memory&cache=shared")
	engine := newAuditEngine()

	w := postJSON(t, engine, "/api/audit/conn",
		`{"action":"new","conn_id":1,"id":"ghost","uuid":"nope","peer":["x"],"type":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var count int64
	service.DB.Model(&model.AuditConn{}).Count(&count)
	if count != 0 {
		t.Errorf("unknown device must not create audit rows, got %d", count)
	}

	w = postJSON(t, engine, "/api/audit/file",
		`{"id":"ghost","uuid":"nope","peer_id":"x","info":"{}"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	service.DB.Model(&model.AuditFile{}).Count(&count)
	if count != 0 {
		t.Errorf("unknown device must not create audit file rows, got %d", count)
	}
}

// TestAuditAcceptsKnownDevice 已知设备（id, uuid）一致时正常写入审计
func TestAuditAcceptsKnownDevice(t *testing.T) {
	setupSecurityTestEnv(t, "file:audit_known?mode=memory&cache=shared")
	service.DB.Create(&model.Peer{Id: "dev1", Uuid: "real-uuid"})
	engine := newAuditEngine()

	w := postJSON(t, engine, "/api/audit/conn",
		`{"action":"new","conn_id":42,"id":"dev1","uuid":"real-uuid","peer":["p1","n1"],"type":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %q", w.Code, w.Body.String())
	}
	var conn model.AuditConn
	service.DB.Where("peer_id = ? and conn_id = ?", "dev1", 42).First(&conn)
	if conn.Id == 0 {
		t.Fatal("audit conn row should be created")
	}
	if conn.FromPeer != "p1" || conn.FromName != "n1" {
		t.Errorf("audit conn fields wrong: %+v", conn)
	}

	w = postJSON(t, engine, "/api/audit/file",
		`{"id":"dev1","uuid":"real-uuid","peer_id":"p2","info":"{\"ip\":\"1.1.1.1\",\"name\":\"f\",\"num\":2}"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %q", w.Code, w.Body.String())
	}
	var file model.AuditFile
	service.DB.Where("peer_id = ?", "dev1").First(&file)
	if file.Id == 0 {
		t.Fatal("audit file row should be created")
	}
	if file.FromName != "f" {
		t.Errorf("audit file from name wrong: %q", file.FromName)
	}

	// uuid 不符的设备不得写入
	w = postJSON(t, engine, "/api/audit/conn",
		`{"action":"new","conn_id":43,"id":"dev1","uuid":"fake","peer":["x"],"type":0}`)
	var count int64
	service.DB.Where("conn_id = 43").Model(&model.AuditConn{}).Count(&count)
	if count != 0 {
		t.Error("mismatched uuid must not create audit rows")
	}
}

