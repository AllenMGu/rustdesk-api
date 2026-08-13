package api

import (
	"encoding/json"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"time"
)

type WebClientPeerPayload struct {
	ViewStyle   string                   `json:"view-style"`
	Tm          int64                    `json:"tm"`
	Info        WebClientPeerInfoPayload `json:"info"`
	Tmppwd      string                   `json:"tmppwd"`
	AddressBook bool                     `json:"address_book"`
	Managed     bool                     `json:"managed"`
}

type WebClientPeerInfoPayload struct {
	Username       string   `json:"username"`
	Hostname       string   `json:"hostname"`
	Platform       string   `json:"platform"`
	Hash           string   `json:"hash"`
	Id             string   `json:"id"`
	Alias          string   `json:"alias"`
	Tags           []string `json:"tags"`
	Online         bool     `json:"online"`
	LastOnlineTime int64    `json:"last_online_time"`
}

func (wcpp *WebClientPeerPayload) FromAddressBook(a *model.AddressBook) {
	wcpp.ViewStyle = "shrink"
	wcpp.AddressBook = true
	//24小时前
	wcpp.Tm = time.Now().Add(-time.Hour * 24).UnixNano()
	tags := make([]string, 0)
	_ = json.Unmarshal([]byte(a.Tags), &tags)
	wcpp.Info = WebClientPeerInfoPayload{
		Username: a.Username,
		Hostname: a.Hostname,
		Platform: a.Platform,
		Hash:     a.Hash,
		Id:       a.Id,
		Alias:    a.Alias,
		Tags:     tags,
		Online:   a.Online,
	}
}

func (wcpp *WebClientPeerPayload) FromPeer(p *model.Peer) {
	wcpp.ViewStyle = "shrink"
	wcpp.Managed = true
	wcpp.Info = WebClientPeerInfoPayload{
		Username:       p.Username,
		Hostname:       p.Hostname,
		Platform:       p.Os,
		Id:             p.Id,
		Alias:          p.Alias,
		Tags:           make([]string, 0),
		Online:         p.LastOnlineTime > time.Now().Add(-90*time.Second).Unix(),
		LastOnlineTime: p.LastOnlineTime,
	}
}

func (wcpp *WebClientPeerPayload) FromShareRecord(sr *model.ShareRecord) {
	wcpp.ViewStyle = "shrink"
	//24小时前
	wcpp.Tm = time.Now().UnixNano()
	wcpp.Tmppwd = sr.Password
	wcpp.Info = WebClientPeerInfoPayload{
		Username: "",
		Hostname: "",
		Platform: "",
		Id:       sr.PeerId,
		Tags:     make([]string, 0),
	}
}
