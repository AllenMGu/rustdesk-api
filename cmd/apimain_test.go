package main

import (
	"io"
	"strings"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupMigrateTestEnv 初始化 Migrate() 所需的全局依赖（in-memory sqlite、
// 静默日志、空 bundle localizer），测试结束恢复原值。
// dsn 必须每个测试唯一：cache=shared 的内存库在进程内按名字共享，
// 复用同名会导致测试间互相污染。
func setupMigrateTestEnv(t *testing.T, dsn string) *gorm.DB {
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

	prevDB, prevConfig, prevLogger, prevLocalizer := global.DB, global.Config, global.Logger, global.Localizer
	prevServiceDB, prevAllService := service.DB, service.AllService
	global.DB = db
	service.DB = db
	global.Config = config.Config{}
	lg := logrus.New()
	lg.SetOutput(io.Discard)
	global.Logger = lg
	global.Localizer = func(lang string) *i18n.Localizer {
		return i18n.NewLocalizer(i18n.NewBundle(language.English), "en")
	}
	service.AllService = &service.Service{}
	t.Cleanup(func() {
		global.DB, global.Config, global.Logger, global.Localizer = prevDB, prevConfig, prevLogger, prevLocalizer
		service.DB, service.AllService = prevServiceDB, prevAllService
		_ = sqlDB.Close()
	})
	return db
}

func maxVersion(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	var v model.Version
	if err := db.Order("version DESC").First(&v).Error; err != nil {
		t.Fatalf("versions table must have a row: %v", err)
	}
	return v.Version
}

// TestMigrate266FailsClosedOnDuplicatePeers 按"旧库存在重复 peers.id"的
// 真实迁移场景验证 v266 的失败语义：
//
//	重复 ID 存在 → Migrate(266) 建唯一索引失败 → versions 不得出现 266
//	清理重复 ID → 再次 Migrate(266) → 索引建成 → versions = 266
//
// 修复前 Migrate() 在 AutoMigrate 失败后仍写入版本号，会把环境永久
// 标记为"迁移成功"（假迁移成功）；修复后失败保持 265，下次启动重试。
func TestMigrate266FailsClosedOnDuplicatePeers(t *testing.T) {
	db := setupMigrateTestEnv(t, "file:migrate_guard_dup?mode=memory&cache=shared")

	// 基线：按 265 完成迁移（等价于"266 之前的旧部署"）
	Migrate(265)
	if got := maxVersion(t, db); got != 265 {
		t.Fatalf("baseline version should be 265, got %d", got)
	}
	// 模拟旧库状态：当时还没有唯一索引，且历史上存在重复 id 行
	if err := db.Exec("DROP INDEX IF EXISTS idx_peers_id_unique").Error; err != nil {
		t.Fatalf("drop unique index: %v", err)
	}
	if err := db.Exec("INSERT INTO peers (id, uuid) VALUES ('dup-1',''),('dup-1','')").Error; err != nil {
		t.Fatalf("insert duplicate-id rows: %v", err)
	}
	var dupCount int64
	db.Model(&model.Peer{}).Where("id = ?", "dup-1").Count(&dupCount)
	if dupCount != 2 {
		t.Fatalf("expected 2 rows for dup-1, got %d", dupCount)
	}

	// 第一次迁移 266：唯一索引创建必须失败，且不得记录 266
	Migrate(266)
	if got := maxVersion(t, db); got != 265 {
		t.Fatalf("failed migration must NOT record version 266, versions max = %d", got)
	}
	var idxCount int64
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_peers_id_unique'").Scan(&idxCount)
	if idxCount != 0 {
		t.Fatal("unique index must not exist after failed migration")
	}
	// 重复行仍在（服务未中断，数据未被破坏）
	db.Model(&model.Peer{}).Where("id = ?", "dup-1").Count(&dupCount)
	if dupCount != 2 {
		t.Fatalf("duplicate rows must remain intact, got %d", dupCount)
	}

	// 管理员清理重复数据（保留一行）后，再次迁移必须成功
	if err := db.Exec("DELETE FROM peers WHERE id = 'dup-1' AND row_id <> (SELECT MIN(row_id) FROM peers WHERE id = 'dup-1')").Error; err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	Migrate(266)
	if got := maxVersion(t, db); got != 266 {
		t.Fatalf("successful retry must record version 266, versions max = %d", got)
	}
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_peers_id_unique'").Scan(&idxCount)
	if idxCount != 1 {
		t.Fatal("unique index must exist after successful migration")
	}
	var idxSQL string
	db.Raw("SELECT sql FROM sqlite_master WHERE type='index' AND name='idx_peers_id_unique'").Scan(&idxSQL)
	if idxSQL == "" || !strings.Contains(strings.ToUpper(idxSQL), "UNIQUE INDEX") {
		t.Fatalf("index must be a UNIQUE index, got %q", idxSQL)
	}
}

// TestMigrate266GuardRejectsWrongColumnIndex 第四轮复审 P2 回归：
// "假迁移成功"的另一类来源——旧库/手工改库中已存在同名、且是 UNIQUE
// 的索引，但建在错误的列上（如 row_id）。AutoMigrate 对同名索引会
// 静默跳过，若守卫只查"索引名 + UNIQUE"就会把 266 记为成功。
// 守卫必须确认索引列恰好是单列 id，否则拒绝记录版本号。
func TestMigrate266GuardRejectsWrongColumnIndex(t *testing.T) {
	db := setupMigrateTestEnv(t, "file:migrate_guard_wrongcol?mode=memory&cache=shared")

	Migrate(265)
	if got := maxVersion(t, db); got != 265 {
		t.Fatalf("baseline version should be 265, got %d", got)
	}
	// 模拟手工改库：同名唯一索引存在，但列是 row_id
	// （Migrate(265) 的 AutoMigrate 已按 model 标签建好正确的索引，先删掉）
	if err := db.Exec("DROP INDEX IF EXISTS idx_peers_id_unique").Error; err != nil {
		t.Fatalf("drop auto-created index: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX idx_peers_id_unique ON peers (row_id)").Error; err != nil {
		t.Fatalf("create wrong-column unique index: %v", err)
	}

	Migrate(266)
	if got := maxVersion(t, db); got != 265 {
		t.Fatalf("wrong-column unique index must NOT be counted as successful migration, versions max = %d", got)
	}

	// 管理员把索引建到正确的列后，重试必须成功
	if err := db.Exec("DROP INDEX idx_peers_id_unique").Error; err != nil {
		t.Fatalf("drop wrong index: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX idx_peers_id_unique ON peers (id)").Error; err != nil {
		t.Fatalf("create correct index: %v", err)
	}
	Migrate(266)
	if got := maxVersion(t, db); got != 266 {
		t.Fatalf("retry with correct single-column index must record 266, got %d", got)
	}
}

// TestEnsurePeersIdUniqueIndexColumnChecks 守卫的列级校验矩阵：
// 无索引 / 错列唯一索引 / 复合列唯一索引 / 非唯一索引 都必须拒绝，
// 只有"peers 表 + UNIQUE + 恰好单列 id"才通过。
func TestEnsurePeersIdUniqueIndexColumnChecks(t *testing.T) {
	db := setupMigrateTestEnv(t, "file:migrate_guard_cols?mode=memory&cache=shared")
	Migrate(265)

	exec := func(sql string) {
		t.Helper()
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}

	exec("DROP INDEX IF EXISTS idx_peers_id_unique")
	if ensurePeersIdUniqueIndex() {
		t.Fatal("no index: guard must reject")
	}

	exec("CREATE UNIQUE INDEX idx_peers_id_unique ON peers (row_id)")
	if ensurePeersIdUniqueIndex() {
		t.Fatal("unique index on wrong column (row_id): guard must reject")
	}

	exec("DROP INDEX idx_peers_id_unique")
	exec("CREATE UNIQUE INDEX idx_peers_id_unique ON peers (id, uuid)")
	if ensurePeersIdUniqueIndex() {
		t.Fatal("composite-column unique index (id, uuid): guard must reject")
	}

	exec("DROP INDEX idx_peers_id_unique")
	exec("CREATE INDEX idx_peers_id_unique ON peers (id)")
	if ensurePeersIdUniqueIndex() {
		t.Fatal("non-unique index on id: guard must reject")
	}

	exec("DROP INDEX idx_peers_id_unique")
	exec("CREATE UNIQUE INDEX idx_peers_id_unique ON peers (id)")
	if !ensurePeersIdUniqueIndex() {
		t.Fatal("unique single-column index on id: guard must accept")
	}
}
