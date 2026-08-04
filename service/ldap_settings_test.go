package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
)

func testLDAPConfig() config.Ldap {
	return config.Ldap{
		Enable:              true,
		Url:                 "ldap://ldap.example.com:389",
		BaseDn:              "DC=example,DC=com",
		BindDn:              "CN=svc,OU=Service,DC=example,DC=com",
		BindPassword:        "yaml-secret",
		ConnectTimeout:      5 * time.Second,
		OperationTimeout:    10 * time.Second,
		NestedGroups:        true,
		EmergencyLocalAdmin: true,
		User: config.LdapUser{
			Filter:     "(objectClass=user)",
			Username:   "sAMAccountName,userPrincipalName",
			Email:      "mail",
			FirstName:  "givenName",
			LastName:   "sn",
			AdminGroup: "RustDesk-Admins",
			AllowGroup: "RustDesk-Users",
		},
	}
}

func TestLDAPSettingsEncryptPersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ldap-settings.json")
	t.Setenv(settingsPathEnvironment, path)
	t.Setenv(settingsKeyEnvironment, "a-long-test-key-that-is-never-persisted")

	manager := newLDAPConfigManager(testLDAPConfig(), nil)
	request := LdapSaveRequest{
		Config:       editableLDAPConfig(testLDAPConfig()),
		BindPassword: "frontend-secret",
	}
	document, err := manager.save(request)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if !document.PasswordConfigured || !document.Persisted {
		t.Fatalf("unexpected document: %+v", document)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if strings.Contains(string(contents), "frontend-secret") || strings.Contains(string(contents), "yaml-secret") {
		t.Fatal("settings file contains a plaintext password")
	}
	if !strings.Contains(string(contents), `"encrypted_bind_password": "v1:`) {
		t.Fatal("settings file does not contain an encrypted password")
	}

	reloaded := newLDAPConfigManager(testLDAPConfig(), nil)
	if got := reloaded.config().BindPassword; got != "frontend-secret" {
		t.Fatalf("reloaded bind password = %q", got)
	}
}

func TestLDAPSettingsEnvironmentOverridesFrontend(t *testing.T) {
	t.Setenv(settingsPathEnvironment, filepath.Join(t.TempDir(), "ldap-settings.json"))
	t.Setenv(settingsKeyEnvironment, "environment-priority-test-key-0001")
	t.Setenv("RUSTDESK_API_LDAP_URL", "ldap://environment.example.com:389")

	base := testLDAPConfig()
	base.Url = "ldap://environment.example.com:389"
	manager := newLDAPConfigManager(base, nil)
	editable := editableLDAPConfig(base)
	editable.URL = "ldap://frontend.example.com:389"
	if _, err := manager.save(LdapSaveRequest{Config: editable, BindPassword: "secret"}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if got := manager.config().Url; got != base.Url {
		t.Fatalf("effective URL = %q, want environment URL %q", got, base.Url)
	}
	document := manager.document()
	if document.Sources["url"] != "environment" {
		t.Fatalf("URL source = %q", document.Sources["url"])
	}
}

func TestLDAPSettingsRequireKeyWhenBindPasswordExists(t *testing.T) {
	t.Setenv(settingsPathEnvironment, filepath.Join(t.TempDir(), "ldap-settings.json"))
	t.Setenv(settingsKeyEnvironment, "")
	manager := newLDAPConfigManager(testLDAPConfig(), nil)
	request := LdapSaveRequest{Config: editableLDAPConfig(testLDAPConfig())}
	if _, err := manager.save(request); !errors.Is(err, ErrLDAPSettingsKeyMissing) {
		t.Fatalf("save error = %v, want ErrLDAPSettingsKeyMissing", err)
	}
}

func TestLDAPSettingsRollback(t *testing.T) {
	t.Setenv(settingsPathEnvironment, filepath.Join(t.TempDir(), "ldap-settings.json"))
	t.Setenv(settingsKeyEnvironment, "rollback-settings-test-key-0000001")

	manager := newLDAPConfigManager(testLDAPConfig(), nil)
	first := editableLDAPConfig(testLDAPConfig())
	first.BaseDN = "DC=first,DC=example"
	if _, err := manager.save(LdapSaveRequest{Config: first, BindPassword: "first-secret"}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	second := first
	second.BaseDN = "DC=second,DC=example"
	if _, err := manager.save(LdapSaveRequest{Config: second, BindPassword: "second-secret"}); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if _, err := manager.rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := manager.config().BaseDn; got != first.BaseDN {
		t.Fatalf("base DN after rollback = %q", got)
	}
	if got := manager.config().BindPassword; got != "first-secret" {
		t.Fatalf("password after rollback = %q", got)
	}
}

func TestLDAPSettingsWrongKeyDoesNotOverwriteSavedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ldap-settings.json")
	t.Setenv(settingsPathEnvironment, path)
	t.Setenv(settingsKeyEnvironment, "original-settings-test-key-0000001")
	manager := newLDAPConfigManager(testLDAPConfig(), nil)
	request := LdapSaveRequest{
		Config:       editableLDAPConfig(testLDAPConfig()),
		BindPassword: "saved-secret",
	}
	if _, err := manager.save(request); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original settings: %v", err)
	}

	t.Setenv(settingsKeyEnvironment, "wrong-settings-test-key-00000000001")
	reloaded := newLDAPConfigManager(testLDAPConfig(), nil)
	if reloaded.document().LoadError == "" {
		t.Fatal("wrong settings key was not reported")
	}
	if _, err := reloaded.save(request); err == nil {
		t.Fatal("save unexpectedly overwrote settings loaded with the wrong key")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings after rejected save: %v", err)
	}
	if string(after) != string(original) {
		t.Fatal("settings file changed after rejected save")
	}
}

func TestLDAPSettingsDetectsConfigurationTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ldap-settings.json")
	t.Setenv(settingsPathEnvironment, path)
	t.Setenv(settingsKeyEnvironment, "tamper-detection-test-key-000000001")
	manager := newLDAPConfigManager(testLDAPConfig(), nil)
	request := LdapSaveRequest{
		Config:       editableLDAPConfig(testLDAPConfig()),
		BindPassword: "saved-secret",
	}
	if _, err := manager.save(request); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var state ldapSettingsFile
	if err := json.Unmarshal(contents, &state); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	state.Current.Config.URL = "ldap://tampered.example.com:389"
	tampered, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered settings: %v", err)
	}
	if err := os.WriteFile(path, tampered, 0600); err != nil {
		t.Fatalf("write tampered settings: %v", err)
	}

	reloaded := newLDAPConfigManager(testLDAPConfig(), nil)
	if reloaded.document().LoadError == "" {
		t.Fatal("tampered LDAP configuration was accepted")
	}
}

func TestLDAPFilterEscapesUserInput(t *testing.T) {
	service := &LdapService{}
	filter := service.filterField("uid", `alice*)(|(uid=*))`)
	if strings.Contains(filter, "(|(uid=*)") {
		t.Fatalf("filter injection was not escaped: %s", filter)
	}
	if !strings.Contains(filter, `\2a`) || !strings.Contains(filter, `\28`) {
		t.Fatalf("filter does not contain LDAP escapes: %s", filter)
	}
}

func TestLDAPValidateMultipleUsernameAttributes(t *testing.T) {
	service := &LdapService{}
	cfg := testLDAPConfig()
	if err := service.ValidateConfig(&cfg); err != nil {
		t.Fatalf("valid AD config rejected: %v", err)
	}
	cfg.User.Username = "sAMAccountName)(uid=*"
	if err := service.ValidateConfig(&cfg); err == nil {
		t.Fatal("invalid username attribute was accepted")
	}
}
