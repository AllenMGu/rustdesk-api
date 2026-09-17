package api

import (
	"errors"
	"fmt"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

// 匿名端点（/api/sysinfo、/api/audit/*）上报字段的长度上限：
// 正常客户端上报内容都很短，超限值按异常数据处理，防止存储膨胀。
const (
	// PeerIdLimit peer id 长度上限。客户端 id 是 10 位数字；
	// 超限视为异常数据，直接拒绝（不能截断：截断会改变查询键，
	// 导致同一设备重复建 peer 或身份校验失配）
	PeerIdLimit = 128
	// PeerUuidLimit peer uuid 长度上限。客户端 uuid 是 22 位 Base64（encode64(get_uuid())）
	PeerUuidLimit = 256
	peerVerLimit   = 64
	peerFieldLimit = 1024
)

// ValidatePeerIdentity 校验匿名端点上报的设备身份 (id, uuid) 的合法性：
// id 必填且不超过 PeerIdLimit，uuid 不超过 PeerUuidLimit（是否允许为空由
// 调用方按端点语义决定）。长度以 rune 计，避免多字节字符误判。
func ValidatePeerIdentity(id, uuid string) error {
	if id == "" {
		return errors.New("id is required")
	}
	if len([]rune(id)) > PeerIdLimit {
		return fmt.Errorf("id exceeds %d characters", PeerIdLimit)
	}
	if len([]rune(uuid)) > PeerUuidLimit {
		return fmt.Errorf("uuid exceeds %d characters", PeerUuidLimit)
	}
	return nil
}

// limitUTF8 将字符串截断到不超过 limit 个 rune，避免在多字节字符中间截断
func limitUTF8(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

type AddressBookFormData struct {
	Tags      []string             `json:"tags"`
	Peers     []*model.AddressBook `json:"peers"`
	TagColors string               `json:"tag_colors"`
}

type AddressBookForm struct {
	Data string `json:"data" example:"{\"tags\":[\"tag1\",\"tag2\",\"tag3\"],\"peers\":[{\"id\":\"abc\",\"username\":\"abv-l\",\"hostname\":\"\",\"platform\":\"Windows\",\"alias\":\"\",\"tags\":[\"tag1\",\"tag2\"],\"hash\":\"hash\"}],\"tag_colors\":\"{\\\"tag1\\\":4288585374,\\\"tag2\\\":4278238420,\\\"tag3\\\":4291681337}\"}"`
}

type PeerForm struct {
	Cpu      string `json:"cpu"`
	Hostname string `json:"hostname"`
	Id       string `json:"id"`
	Memory   string `json:"memory"`
	Os       string `json:"os"`
	Username string `json:"username"`
	Uuid     string `json:"uuid"`
	Version  string `json:"version"`
}

// ToPeer 展示类字段超长时截断入库；id/uuid 是身份标识与查询键，
// 由调用方先经 ValidatePeerIdentity 校验长度（超限拒绝），此处原样保留
func (pf *PeerForm) ToPeer() *model.Peer {
	return &model.Peer{
		Cpu:      limitUTF8(pf.Cpu, peerFieldLimit),
		Hostname: limitUTF8(pf.Hostname, peerFieldLimit),
		Id:       pf.Id,
		Memory:   limitUTF8(pf.Memory, peerFieldLimit),
		Os:       limitUTF8(pf.Os, peerFieldLimit),
		Username: limitUTF8(pf.Username, peerFieldLimit),
		Uuid:     pf.Uuid,
		Version:  limitUTF8(pf.Version, peerVerLimit),
	}
}

// PersonalAddressBookForm 个人地址簿表单
type PersonalAddressBookForm struct {
	model.AddressBook
	ForceAlwaysRelay string `json:"forceAlwaysRelay"`
}

func (pabf *PersonalAddressBookForm) ToAddressBook() *model.AddressBook {
	return &model.AddressBook{
		RowId:            pabf.RowId,
		Id:               pabf.Id,
		Username:         pabf.Username,
		Password:         pabf.Password,
		Hostname:         pabf.Hostname,
		Alias:            pabf.Alias,
		Platform:         pabf.Platform,
		Tags:             pabf.Tags,
		Hash:             pabf.Hash,
		UserId:           pabf.UserId,
		ForceAlwaysRelay: pabf.ForceAlwaysRelay == "true",
		RdpPort:          pabf.RdpPort,
		RdpUsername:      pabf.RdpUsername,
		Online:           pabf.Online,
		LoginName:        pabf.LoginName,
		SameServer:       pabf.SameServer,
	}
}

type TagRenameForm struct {
	Old string `json:"old"`
	New string `json:"new"`
}
type TagColorForm struct {
	Name  string `json:"name"`
	Color uint   `json:"color"`
}

type PeerInfoInHeartbeat struct {
	Id   string `json:"id"`
	Uuid string `json:"uuid"`
	Ver  int    `json:"ver"`
}
