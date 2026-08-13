package api

import (
	"encoding/json"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

type WebClientPeerPayload struct {
	ViewStyle   string                   `json:"view-style"`
	Tm          int64                    `json:"tm"`
	Info        WebClientPeerInfoPayload `json:"info"`
	Tmppwd      string                   `json:"tmppwd"`
	AddressBook  bool                     `json:"address_book"`
	AddressBooks []string                 `json:"address_books"`
	Managed      bool                     `json:"managed"`
}

type WebClientAddressBookPayload struct {
	Guid  string   `json:"guid"`
	Name  string   `json:"name"`
	Owner string   `json:"owner"`
	Rule  int      `json:"rule"`
	Tags  []string `json:"tags"`
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
	tags := WebClientAddressBookTags(a)
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

func WebClientAddressBookTags(a *model.AddressBook) []string {
	tags := make([]string, 0)
	_ = json.Unmarshal([]byte(a.Tags), &tags)
	return tags
}

func (wcpp *WebClientPeerPayload) AddAddressBook(guid string) {
	wcpp.AddressBook = true
	for _, existing := range wcpp.AddressBooks {
		if existing == guid {
			return
		}
	}
	wcpp.AddressBooks = append(wcpp.AddressBooks, guid)
}

func (wcpp *WebClientPeerPayload) MergeAddressBook(a *model.AddressBook, guid string) {
	wcpp.AddAddressBook(guid)
	tags := WebClientAddressBookTags(a)
	seen := make(map[string]bool, len(wcpp.Info.Tags))
	for _, tag := range wcpp.Info.Tags {
		seen[tag] = true
	}
	for _, tag := range tags {
		if tag != "" && !seen[tag] {
			wcpp.Info.Tags = append(wcpp.Info.Tags, tag)
			seen[tag] = true
		}
	}
	if wcpp.Info.Username == "" {
		wcpp.Info.Username = a.Username
	}
	if wcpp.Info.Hostname == "" {
		wcpp.Info.Hostname = a.Hostname
	}
	if wcpp.Info.Platform == "" {
		wcpp.Info.Platform = a.Platform
	}
	if wcpp.Info.Hash == "" {
		wcpp.Info.Hash = a.Hash
	}
	if wcpp.Info.Alias == "" {
		wcpp.Info.Alias = a.Alias
	}
	wcpp.Info.Online = wcpp.Info.Online || a.Online
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
