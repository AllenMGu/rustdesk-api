package api

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
)

type WebClient struct {
}

// ServerConfig 服务配置
// @Tags WEBCLIENT
// @Summary 服务配置
// @Description 服务配置,给webclient提供api-server
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /server-config [get]
// @Security token
func (i *WebClient) ServerConfig(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)

	peers := map[string]*api.WebClientPeerPayload{}
	addressBooks := make([]*api.WebClientAddressBookPayload, 0)
	addAddressBook := func(owner *model.User, collectionID uint, name string, rule int) {
		guid := fmt.Sprintf("%d-%d-%d", owner.GroupId, owner.Id, collectionID)
		profile := &api.WebClientAddressBookPayload{
			Guid: guid, Name: name, Owner: owner.Username, Rule: rule, Tags: make([]string, 0),
		}
		seenTags := map[string]bool{}
		abs := service.AllService.AddressBookService.ListByUserIdAndCollectionId(owner.Id, collectionID, 1, 1000)
		for _, ab := range abs.AddressBooks {
			for _, tag := range api.WebClientAddressBookTags(ab) {
				if tag != "" && !seenTags[tag] {
					profile.Tags = append(profile.Tags, tag)
					seenTags[tag] = true
				}
			}
			pp, exists := peers[ab.Id]
			if !exists {
				pp = &api.WebClientPeerPayload{}
				pp.FromAddressBook(ab)
				peers[ab.Id] = pp
			}
			pp.MergeAddressBook(ab, guid)
		}
		addressBooks = append(addressBooks, profile)
	}

	addAddressBook(u, 0, u.Username, model.ShareAddressBookRuleRuleFullControl)
	ownedCollections := service.AllService.AddressBookService.ListCollectionByUserId(u.Id)
	for _, collection := range ownedCollections.AddressBookCollection {
		addAddressBook(u, collection.Id, collection.Name, model.ShareAddressBookRuleRuleFullControl)
	}

	maxRules := map[uint]int{}
	for _, rule := range service.AllService.AddressBookService.CollectionReadRules(u) {
		if rule.Rule > maxRules[rule.CollectionId] {
			maxRules[rule.CollectionId] = rule.Rule
		}
	}
	for _, collection := range service.AllService.AddressBookService.ListCollectionByIds(utils.Keys(maxRules)) {
		owner := service.AllService.UserService.InfoById(collection.UserId)
		if owner == nil || owner.Id == 0 || owner.Id == u.Id {
			continue
		}
		addAddressBook(owner, collection.Id, collection.Name, maxRules[collection.Id])
	}

	devices := service.AllService.PeerService.ListByUserIds([]uint{u.Id}, 1, 1000)
	for _, device := range devices.Peers {
		if existing, ok := peers[device.Id]; ok {
			existing.Managed = true
			existing.Info.Online = device.LastOnlineTime > time.Now().Add(-90*time.Second).Unix()
			existing.Info.LastOnlineTime = device.LastOnlineTime
			if existing.Info.Alias == "" {
				existing.Info.Alias = device.Alias
			}
			if existing.Info.Username == "" {
				existing.Info.Username = device.Username
			}
			if existing.Info.Hostname == "" {
				existing.Info.Hostname = device.Hostname
			}
			if existing.Info.Platform == "" {
				existing.Info.Platform = service.AllService.AddressBookService.PlatformFromOs(device.Os)
			}
			continue
		}
		pp := &api.WebClientPeerPayload{}
		pp.FromPeer(device)
		pp.Info.Platform = service.AllService.AddressBookService.PlatformFromOs(device.Os)
		peers[device.Id] = pp
	}
	response.Success(
		c,
		gin.H{
			"id_server":    global.Config.Rustdesk.IdServer,
			"key":          global.Config.Rustdesk.Key,
			"peers":        peers,
			"address_books": addressBooks,
		},
	)
}

// SharedPeer 分享的peer
// @Tags WEBCLIENT
// @Summary 分享的peer
// @Description 分享的peer
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /shared-peer [post]
func (i *WebClient) SharedPeer(c *gin.Context) {
	j := &gin.H{}
	c.ShouldBindJSON(j)
	t := (*j)["share_token"].(string)
	if t == "" {
		response.Fail(c, 101, "share_token is required")
		return
	}
	sr := service.AllService.AddressBookService.SharedPeer(t)
	if sr == nil || sr.Id == 0 {
		response.Fail(c, 101, "share not found")
		return
	}
	if sr.Expire != 0 {
		//判断是否过期,created_at + expire > now
		ca := time.Time(sr.CreatedAt)
		if ca.Add(time.Second * time.Duration(sr.Expire)).Before(time.Now()) {
			response.Fail(c, 101, "share expired")
			return
		}
	}

	ab := service.AllService.AddressBookService.InfoByUserIdAndId(sr.UserId, sr.PeerId)
	if ab.RowId == 0 {
		response.Fail(c, 101, "peer not found")
		return
	}
	pp := &api.WebClientPeerPayload{}
	pp.FromShareRecord(sr)
	pp.Info.Username = ab.Username
	pp.Info.Hostname = ab.Hostname
	response.Success(c, gin.H{
		"id_server": global.Config.Rustdesk.IdServer,
		"key":       global.Config.Rustdesk.Key,
		"peer":      pp,
	})
}
