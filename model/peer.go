package model

type Peer struct {
	RowId          uint   `json:"row_id" gorm:"primaryKey;"`
	// Id 设备 ID 唯一：防止并发首次注册插入同 ID 的重复 peer 行
	//（重复行会使 FindById/身份校验的结果不确定）。
	// 用显式索引名：既有生产库中已存在同名非唯一索引 idx_peers_id，
	// GORM 按名称判断"已存在"而不会重建为唯一索引，显式命名可强制新建。
	// 生产库已确认无重复 ID（2026-09 核查：349 行、0 重复），建索引安全。
	Id             string `json:"id"  gorm:"default:'';not null;uniqueIndex:idx_peers_id_unique"`
	Cpu            string `json:"cpu"  gorm:"default:'';not null;"`
	Hostname       string `json:"hostname"  gorm:"default:'';not null;"`
	Memory         string `json:"memory"  gorm:"default:'';not null;"`
	Os             string `json:"os"  gorm:"default:'';not null;"`
	Username       string `json:"username"  gorm:"default:'';not null;"`
	Uuid           string `json:"uuid"  gorm:"default:'';not null;index"`
	Version        string `json:"version"  gorm:"default:'';not null;"`
	UserId         uint   `json:"user_id"  gorm:"default:0;not null;index"`
	User           *User  `json:"user,omitempty"`
	LastOnlineTime int64  `json:"last_online_time"  gorm:"default:0;not null;"`
	LastOnlineIp   string `json:"last_online_ip"  gorm:"default:'';not null;"`
	GroupId        uint   `json:"group_id"  gorm:"default:0;not null;index"`
	Alias          string `json:"alias" gorm:"default:'';not null;index"`
	TimeModel
}

type PeerList struct {
	Peers []*Peer `json:"list"`
	Pagination
}
