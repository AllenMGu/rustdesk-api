package service

import (
	"strings"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type PeerService struct {
}

// FindById 根据id查找
func (ps *PeerService) FindById(id string) *model.Peer {
	p := &model.Peer{}
	DB.Where("id = ?", id).First(p)
	return p
}
func (ps *PeerService) FindByUuid(uuid string) *model.Peer {
	p := &model.Peer{}
	DB.Where("uuid = ?", uuid).First(p)
	return p
}
func (ps *PeerService) InfoByRowId(id uint) *model.Peer {
	p := &model.Peer{}
	DB.Where("row_id = ?", id).First(p)
	return p
}

// FindByUserIdAndUuid 根据用户id和uuid查找peer
func (ps *PeerService) FindByUserIdAndUuid(uuid string, userId uint) *model.Peer {
	p := &model.Peer{}
	DB.Where("uuid = ? and user_id = ?", uuid, userId).First(p)
	return p
}

// UuidBindUserId 绑定用户id
func (ps *PeerService) UuidBindUserId(deviceId string, uuid string, userId uint) {
	peer := ps.FindByUuid(uuid)
	// 如果存在则更新
	if peer.RowId > 0 {
		peer.UserId = userId
		ps.Update(peer)
	} else {
		// 不存在则创建
		/*if deviceId != "" {
			DB.Create(&model.Peer{
				Id:     deviceId,
				Uuid:   uuid,
				UserId: userId,
			})
		}*/
	}
}

// VerifyDeviceIdentity 只读校验匿名端点（/api/sysinfo、/api/audit/*）上报的
// 设备身份 (id, uuid)。官方 RustDesk 客户端上报这两个端点时不携带登录凭证，
// 而 (id, uuid) 是同一台机器上稳定成对出现的标识，可作为弱设备身份使用。
//
// 注意：
//   - 本方法绝不写库：校验路径上任何分支（包括旧 peer 尚未绑定 uuid 的情况）
//     都不得产生绑定副作用，否则匿名审计端点可被用来抢占/改写既有 peer 的
//     uuid，进而劫持其他端点的身份绑定。
//   - uuid 大小写敏感：客户端上报的 uuid 是 encode64(uuid) 的 Base64 串，
//     Base64 编码大小写敏感，必须精确匹配（不能 EqualFold）。
//   - id/uuid 任一为空一律拒绝；未绑定 uuid 的旧 peer 也一律拒绝，
//     其首次绑定只能发生在 /api/sysinfo 的 BindLegacyDeviceIdentity 路径。
func (ps *PeerService) VerifyDeviceIdentity(id, uuid string) bool {
	id = strings.TrimSpace(id)
	uuid = strings.TrimSpace(uuid)
	if id == "" || uuid == "" {
		return false
	}
	pe := ps.FindById(id)
	return pe.RowId != 0 && pe.Uuid != "" && pe.Uuid == uuid
}

// BindLegacyDeviceIdentity 为尚未绑定 uuid 的既有 peer 做一次性绑定，
// 仅允许 /api/sysinfo（设备自报系统信息的端点）调用：
// peer 行必须存在、Uuid 为空、上报的 uuid 非空，绑定后立即生效。
// 已绑定 uuid 的 peer 一律拒绝改绑（uuid 是设备稳定标识，不允许被改写）。
//
// 并发安全：绑定用单条带条件的 UPDATE（CAS，compare-and-swap）完成，
// 而不是"先查再改"——两个并发请求同时看到空 uuid 时，只有第一个的
// UPDATE 命中行（RowsAffected==1），第二个因 uuid 已非空而命中 0 行失败，
// 从而保证"一次性绑定"在竞态下依然成立。
//
// 按主键 row_id 做 CAS，而不是业务 id：旧库若存在同 id 多行
// （唯一索引尚未建成），按 id 的 UPDATE 会一次改动多行，且
// RowsAffected != 1 的"失败"返回时副作用已经发生；按 row_id 则
// 永远至多命中一行，失败零副作用。
func (ps *PeerService) BindLegacyDeviceIdentity(rowId uint, uuid string) bool {
	uuid = strings.TrimSpace(uuid)
	if rowId == 0 || uuid == "" {
		return false
	}
	res := DB.Model(&model.Peer{}).
		Where("row_id = ? AND (uuid = '' OR uuid IS NULL)", rowId).
		Update("uuid", uuid)
	if res.Error != nil {
		return false
	}
	return res.RowsAffected == 1
}

// UuidUnbindUserId 解绑用户id, 用于用户注销
func (ps *PeerService) UuidUnbindUserId(uuid string, userId uint) {
	peer := ps.FindByUserIdAndUuid(uuid, userId)
	if peer.RowId > 0 {
		DB.Model(peer).Update("user_id", 0)
	}
}

// EraseUserId 清除用户id, 用于用户删除
func (ps *PeerService) EraseUserId(userId uint) error {
	return DB.Model(&model.Peer{}).Where("user_id = ?", userId).Update("user_id", 0).Error
}

// ListByUserIds 根据用户id取列表
func (ps *PeerService) ListByUserIds(userIds []uint, page, pageSize uint) (res *model.PeerList) {
	res = &model.PeerList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Peer{})
	tx.Where("user_id in (?)", userIds)
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Peers)
	return
}

func (ps *PeerService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.PeerList) {
	res = &model.PeerList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Peer{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Peers)
	return
}

// ListFilterByUserId 根据用户id过滤Peer列表
func (ps *PeerService) ListFilterByUserId(page, pageSize uint, where func(tx *gorm.DB), userId uint) (res *model.PeerList) {
	userWhere := func(tx *gorm.DB) {
		tx.Where("user_id = ?", userId)
		// 如果还有额外的筛选条件，执行它
		if where != nil {
			where(tx)
		}
	}
	return ps.List(page, pageSize, userWhere)
}

// Create 创建
func (ps *PeerService) Create(u *model.Peer) error {
	res := DB.Create(u).Error
	return res
}

// Delete 删除, 同时也应该删除token
func (ps *PeerService) Delete(u *model.Peer) error {
	uuid := u.Uuid
	err := DB.Delete(u).Error
	if err != nil {
		return err
	}
	// 删除token
	return AllService.UserService.FlushTokenByUuid(uuid)
}

// GetUuidListByIDs 根据ids获取uuid列表
func (ps *PeerService) GetUuidListByIDs(ids []uint) ([]string, error) {
	var uuids []string
	err := DB.Model(&model.Peer{}).
		Where("row_id in (?)", ids).
		Pluck("uuid", &uuids).Error
	//过滤uuids中的空字符串
	var newUuids []string
	for _, uuid := range uuids {
		if uuid != "" {
			newUuids = append(newUuids, uuid)
		}
	}
	return newUuids, err
}

// BatchDelete 批量删除, 同时也应该删除token
func (ps *PeerService) BatchDelete(ids []uint) error {
	uuids, err := ps.GetUuidListByIDs(ids)
	err = DB.Where("row_id in (?)", ids).Delete(&model.Peer{}).Error
	if err != nil {
		return err
	}
	// 删除token
	return AllService.UserService.FlushTokenByUuids(uuids)
}

// Update 更新
func (ps *PeerService) Update(u *model.Peer) error {
	return DB.Model(u).Updates(u).Error
}
