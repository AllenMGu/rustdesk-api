package api

import (
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
)

func TestWebClientPeerFromAddressBook(t *testing.T) {
	peer := &WebClientPeerPayload{}
	peer.FromAddressBook(&model.AddressBook{
		Id:       "123456789",
		Username: "allen",
		Hostname: "WS-01",
		Alias:    "Validation PC",
		Platform: "Windows",
		Tags:     custom_types.AutoJson([]byte(`["GMP","Wuxi"]`)),
		Online:   true,
	})

	if !peer.AddressBook || peer.Managed {
		t.Fatalf("unexpected source flags: address_book=%v managed=%v", peer.AddressBook, peer.Managed)
	}
	if peer.Info.Id != "123456789" || peer.Info.Alias != "Validation PC" {
		t.Fatalf("address book identity was not preserved: %#v", peer.Info)
	}
	if len(peer.Info.Tags) != 2 || peer.Info.Tags[0] != "GMP" {
		t.Fatalf("address book tags were not preserved: %#v", peer.Info.Tags)
	}
	peer.MergeAddressBook(&model.AddressBook{
		Id:   "123456789",
		Tags: custom_types.AutoJson([]byte(`["GMP","Shared"]`)),
	}, "1-2-3")
	peer.AddAddressBook("1-2-3")
	if len(peer.AddressBooks) != 1 || peer.AddressBooks[0] != "1-2-3" {
		t.Fatalf("address book membership was not preserved: %#v", peer.AddressBooks)
	}
	if len(peer.Info.Tags) != 3 || peer.Info.Tags[2] != "Shared" {
		t.Fatalf("address book tags were not merged: %#v", peer.Info.Tags)
	}
}

func TestWebClientPeerFromManagedDevice(t *testing.T) {
	peer := &WebClientPeerPayload{}
	peer.FromPeer(&model.Peer{
		Id:             "987654321",
		Username:       "operator",
		Hostname:       "LAB-01",
		Alias:          "Lab workstation",
		Os:             "Windows 11",
		LastOnlineTime: time.Now().Unix(),
	})

	if !peer.Managed || peer.AddressBook {
		t.Fatalf("unexpected source flags: address_book=%v managed=%v", peer.AddressBook, peer.Managed)
	}
	if !peer.Info.Online || peer.Info.LastOnlineTime == 0 {
		t.Fatalf("managed device online state was not preserved: %#v", peer.Info)
	}
}
