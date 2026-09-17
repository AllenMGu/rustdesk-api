package service

import (
	"sync"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 为 service 包测试初始化 in-memory sqlite 数据库
func setupTestDB(t *testing.T, dsn string) {
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
	if err := db.AutoMigrate(&model.Peer{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	previous := DB
	DB = db
	t.Cleanup(func() {
		DB = previous
		_ = sqlDB.Close()
	})
}

func TestVerifyDeviceIdentity(t *testing.T) {
	setupTestDB(t, "file:verify_identity?mode=memory&cache=shared")
	ps := &PeerService{}

	// 已绑定 uuid 的 peer：uuid 必须精确一致（Base64 大小写敏感）
	DB.Create(&model.Peer{Id: "peer1", Uuid: "uuid-A"})
	if !ps.VerifyDeviceIdentity("peer1", "uuid-A") {
		t.Error("exactly matching uuid should be accepted")
	}
	// Base64 大小写敏感：uuid-a != uuid-A，必须拒绝（客户端上报 encode64(uuid)）
	if ps.VerifyDeviceIdentity("peer1", "uuid-a") {
		t.Error("uuid differing only in case must be rejected (Base64 is case-sensitive)")
	}
	if ps.VerifyDeviceIdentity("peer1", "uuid-B") {
		t.Error("mismatched uuid should be rejected")
	}
	if ps.VerifyDeviceIdentity("peer1", "") {
		t.Error("empty uuid should be rejected for peer with bound uuid")
	}

	// 未绑定 uuid 的旧 peer：校验路径绝不写库、一律拒绝
	// （防止匿名端点借“首次识别”抢占既有 peer 的 uuid 绑定）
	DB.Create(&model.Peer{Id: "peer2"})
	if ps.VerifyDeviceIdentity("peer2", "uuid-C") {
		t.Error("unbound peer must be rejected by the read-only verification path")
	}
	pe2 := ps.FindById("peer2")
	if pe2.Uuid != "" {
		t.Errorf("verification must not write uuid, got %q", pe2.Uuid)
	}

	// 未知 peer → 拒绝
	if ps.VerifyDeviceIdentity("ghost", "uuid-A") {
		t.Error("unknown id should be rejected")
	}
	// 完全未知 → 拒绝
	if ps.VerifyDeviceIdentity("ghost", "uuid-unknown") {
		t.Error("fully unknown (id, uuid) should be rejected")
	}
	// id 为空 → 拒绝
	if ps.VerifyDeviceIdentity("", "uuid-A") {
		t.Error("empty id should be rejected")
	}
}

func TestBindLegacyDeviceIdentity(t *testing.T) {
	setupTestDB(t, "file:bind_legacy?mode=memory&cache=shared")
	ps := &PeerService{}

	// 未绑定 uuid 的既有 peer：允许一次性绑定（按 FindById 得到的具体行）
	DB.Create(&model.Peer{Id: "legacy1"})
	pe0 := ps.FindById("legacy1")
	if pe0.RowId == 0 {
		t.Fatal("test peer must exist")
	}
	if !ps.BindLegacyDeviceIdentity(pe0.RowId, "uuid-L1") {
		t.Fatal("legacy unbound peer should be bindable")
	}
	if pe := ps.FindById("legacy1"); pe.Uuid != "uuid-L1" {
		t.Errorf("uuid should be bound, got %q", pe.Uuid)
	}
	// 绑定后即可通过只读校验
	if !ps.VerifyDeviceIdentity("legacy1", "uuid-L1") {
		t.Error("bound legacy peer should pass verification with the bound uuid")
	}
	// 已绑定后不允许改绑（uuid 是设备稳定标识）
	if ps.BindLegacyDeviceIdentity(pe0.RowId, "uuid-L2") {
		t.Error("already-bound peer must not be rebindable")
	}
	if pe := ps.FindById("legacy1"); pe.Uuid != "uuid-L1" {
		t.Errorf("uuid must not change after first binding, got %q", pe.Uuid)
	}

	// 无效 rowId / 空 uuid → 拒绝
	if ps.BindLegacyDeviceIdentity(0, "uuid-X") {
		t.Error("zero rowId should be rejected")
	}
	if ps.BindLegacyDeviceIdentity(pe0.RowId, "") {
		t.Error("empty uuid should be rejected")
	}
	// 不存在的行 → 拒绝
	if ps.BindLegacyDeviceIdentity(999999, "uuid-X") {
		t.Error("non-existent row should be rejected")
	}
}

// TestBindLegacyDeviceIdentityConcurrent 并发首次绑定必须恰好一个成功：
// 绑定是单条带条件 UPDATE（CAS），两个并发请求都看到空 uuid 时，
// 只有第一个的 UPDATE 命中行（RowsAffected==1），另一个命中 0 行失败。
// 若实现退化为"先查再改"，两个都会成功，且后写者覆盖先写者。
func TestBindLegacyDeviceIdentityConcurrent(t *testing.T) {
	setupTestDB(t, "file:bind_legacy_race?mode=memory&cache=shared")
	ps := &PeerService{}
	DB.Create(&model.Peer{Id: "race1"})
	rowId := ps.FindById("race1").RowId
	if rowId == 0 {
		t.Fatal("test peer must exist")
	}

	const n = 8
	results := make([]bool, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = ps.BindLegacyDeviceIdentity(rowId, "uuid-racer")
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one concurrent bind should win, got %d (results=%v)", wins, results)
	}
	if pe := ps.FindById("race1"); pe.Uuid != "uuid-racer" {
		t.Errorf("bound uuid must be the winner's, got %q", pe.Uuid)
	}
}

// TestBindLegacyDeviceIdentityDuplicateIdLegacyDB 旧库存在同 id 多行
// （唯一索引 idx_peers_id_unique 尚未建成）时，按 row_id 的 CAS 必须
// 只改动目标行：不得批量改动同 id 的其它行（按 id 的 UPDATE 会命中
// 多行，且 RowsAffected!=1 的"失败"返回时副作用已发生）。
func TestBindLegacyDeviceIdentityDuplicateIdLegacyDB(t *testing.T) {
	setupTestDB(t, "file:bind_legacy_dup?mode=memory&cache=shared")
	ps := &PeerService{}
	// 模拟"唯一索引尚未建成"的旧库：撤掉唯一索引后插入同 id 两行
	if err := DB.Exec("DROP INDEX IF EXISTS idx_peers_id_unique").Error; err != nil {
		t.Fatalf("drop unique index: %v", err)
	}
	if err := DB.Exec("INSERT INTO peers (id, uuid) VALUES ('dup1',''),('dup1','')").Error; err != nil {
		t.Fatalf("insert duplicate-id rows: %v", err)
	}
	var rows []model.Peer
	DB.Where("id = ?", "dup1").Order("row_id").Find(&rows)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for dup1, got %d", len(rows))
	}
	first, other := rows[0].RowId, rows[1].RowId

	// 绑定第一行
	if !ps.BindLegacyDeviceIdentity(first, "uuid-first") {
		t.Fatal("first row should be bindable")
	}
	// 同 id 的另一行必须原样未动（零副作用）
	var otherPe model.Peer
	DB.Where("row_id = ?", other).First(&otherPe)
	if otherPe.Uuid != "" {
		t.Errorf("binding one row must not touch sibling rows, got %q", otherPe.Uuid)
	}
	// FindById（First）看到的就是已绑定的第一行
	if pe := ps.FindById("dup1"); pe.RowId != first || pe.Uuid != "uuid-first" {
		t.Errorf("FindById must return the bound first row, got row_id=%d uuid=%q", pe.RowId, pe.Uuid)
	}
	// 另一行仍是未绑定遗留行，可独立绑定（两条独立的设备记录）
	if !ps.BindLegacyDeviceIdentity(other, "uuid-second") {
		t.Fatal("second row should be independently bindable")
	}
	// 已绑定的行不可改绑
	if ps.BindLegacyDeviceIdentity(first, "uuid-again") {
		t.Error("already-bound row must not be rebindable")
	}
}

// 说明：BindLegacyDeviceIdentity 的 WHERE 同时覆盖 uuid = '' 与 uuid IS NULL，
// 是对第三方/遗留数据库的防御。本应用自身 schema 中 uuid 为
// `text NOT NULL DEFAULT ''`（已核查生产库 DDL），不存在 NULL 行，
// 因此不单独对 NULL 场景写测试。
