package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
	log "github.com/sirupsen/logrus"
)

const (
	ldapSettingsVersion     = 1
	defaultLDAPSettingsPath = "data/ldap-settings.json"
	settingsKeyEnvironment  = "RUSTDESK_API_SETTINGS_KEY"
	settingsPathEnvironment = "RUSTDESK_API_LDAP_SETTINGS_FILE"
)

var (
	ErrLDAPSettingsKeyMissing  = errors.New("RUSTDESK_API_SETTINGS_KEY is required to store the bind password")
	ErrLDAPRollbackUnavailable = errors.New("no previous LDAP configuration is available")
)

// LdapEditableConfig is the non-secret representation shared by the admin API
// and the encrypted settings file. Durations are seconds to keep the JSON easy
// to inspect and edit with deployment tooling.
type LdapEditableConfig struct {
	Enable                  bool                   `json:"enable"`
	URL                     string                 `json:"url"`
	TLSCAFile               string                 `json:"tls_ca_file"`
	TLSVerify               bool                   `json:"tls_verify"`
	BaseDN                  string                 `json:"base_dn"`
	BindDN                  string                 `json:"bind_dn"`
	ConnectTimeoutSeconds   int                    `json:"connect_timeout_seconds"`
	OperationTimeoutSeconds int                    `json:"operation_timeout_seconds"`
	NestedGroups            bool                   `json:"nested_groups"`
	EmergencyLocalAdmin     bool                   `json:"emergency_local_admin"`
	User                    LdapEditableUserConfig `json:"user"`
}

type LdapEditableUserConfig struct {
	BaseDN          string `json:"base_dn"`
	EnableAttr      string `json:"enable_attr"`
	EnableAttrValue string `json:"enable_attr_value"`
	Filter          string `json:"filter"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Sync            bool   `json:"sync"`
	AdminGroup      string `json:"admin_group"`
	AllowGroup      string `json:"allow_group"`
}

type LdapSettingsDocument struct {
	Config             LdapEditableConfig `json:"config"`
	PasswordConfigured bool               `json:"password_configured"`
	Sources            map[string]string  `json:"sources"`
	LockedFields       []string           `json:"locked_fields"`
	Persisted          bool               `json:"persisted"`
	CanSavePassword    bool               `json:"can_save_password"`
	CanRollback        bool               `json:"can_rollback"`
	LoadError          string             `json:"load_error,omitempty"`
	UpdatedAt          *time.Time         `json:"updated_at,omitempty"`
}

type LdapSaveRequest struct {
	Config            LdapEditableConfig `json:"config"`
	BindPassword      string             `json:"bind_password"`
	ClearBindPassword bool               `json:"clear_bind_password"`
}

type persistedLdapConfig struct {
	Config                LdapEditableConfig `json:"config"`
	EncryptedBindPassword string             `json:"encrypted_bind_password,omitempty"`
	UseBaseBindPassword   bool               `json:"use_base_bind_password,omitempty"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

type ldapSettingsFile struct {
	Version  int                  `json:"version"`
	Current  *persistedLdapConfig `json:"current,omitempty"`
	Previous *persistedLdapConfig `json:"previous,omitempty"`
}

type ldapConfigManager struct {
	base    config.Ldap
	current atomic.Pointer[config.Ldap]
	path    string
	key     []byte
	logger  *log.Logger
	loadErr string
	mu      sync.Mutex
	state   ldapSettingsFile
}

func newLDAPConfigManager(base config.Ldap, logger *log.Logger) *ldapConfigManager {
	settingsPath := strings.TrimSpace(os.Getenv(settingsPathEnvironment))
	if settingsPath == "" {
		settingsPath = defaultLDAPSettingsPath
	}

	m := &ldapConfigManager{
		base:   normalizeLDAPConfig(base),
		path:   settingsPath,
		logger: logger,
		state:  ldapSettingsFile{Version: ldapSettingsVersion},
	}
	if rawKey := os.Getenv(settingsKeyEnvironment); rawKey != "" {
		if len([]byte(rawKey)) < 32 || strings.HasPrefix(rawKey, "replace-with-") {
			if logger != nil {
				logger.Warn("RUSTDESK_API_SETTINGS_KEY is a placeholder or shorter than 32 bytes; LDAP password saves are disabled")
			}
		} else {
			digest := sha256.Sum256([]byte(rawKey))
			m.key = digest[:]
		}
	}

	initial := m.base
	if err := m.load(); err != nil {
		m.loadErr = err.Error()
		if logger != nil {
			logger.WithError(err).Warn("LDAP saved settings could not be loaded; using environment/config.yaml")
		}
	} else if m.state.Current != nil {
		if effective, err := m.effectiveConfig(m.state.Current); err == nil {
			initial = effective
		} else {
			// Keep the file untouched so restoring the original key and restarting
			// can recover it, but do not report an unapplied file as active.
			m.state = ldapSettingsFile{Version: ldapSettingsVersion}
			m.loadErr = err.Error()
			if logger != nil {
				logger.WithError(err).Warn("LDAP saved password could not be decrypted; using environment/config.yaml")
			}
		}
	}
	m.current.Store(cloneLDAPConfig(initial))
	return m
}

func normalizeLDAPConfig(cfg config.Ldap) config.Ldap {
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 5 * time.Second
	}
	if cfg.OperationTimeout <= 0 {
		cfg.OperationTimeout = 10 * time.Second
	}
	return cfg
}

func cloneLDAPConfig(cfg config.Ldap) *config.Ldap {
	copy := cfg
	return &copy
}

func (m *ldapConfigManager) config() *config.Ldap {
	if cfg := m.current.Load(); cfg != nil {
		return cloneLDAPConfig(*cfg)
	}
	return cloneLDAPConfig(m.base)
}

func (m *ldapConfigManager) load() error {
	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state ldapSettingsFile
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode LDAP settings: %w", err)
	}
	if state.Version != ldapSettingsVersion {
		return fmt.Errorf("unsupported LDAP settings version %d", state.Version)
	}
	m.state = state
	return nil
}

func (m *ldapConfigManager) document() LdapSettingsDocument {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.config()
	locked := ldapLockedFields()
	sources := make(map[string]string, len(ldapFieldEnvironments))
	for field := range ldapFieldEnvironments {
		source := "config"
		if m.state.Current != nil {
			source = "frontend"
		}
		if ldapFieldLocked(field) {
			source = "environment"
		}
		sources[field] = source
	}

	doc := LdapSettingsDocument{
		Config:             editableLDAPConfig(*cfg),
		PasswordConfigured: cfg.BindPassword != "",
		Sources:            sources,
		LockedFields:       locked,
		Persisted:          m.state.Current != nil,
		CanSavePassword:    len(m.key) > 0,
		CanRollback:        m.state.Previous != nil,
		LoadError:          m.loadErr,
	}
	if m.state.Current != nil {
		updatedAt := m.state.Current.UpdatedAt
		doc.UpdatedAt = &updatedAt
	}
	return doc
}

func (m *ldapConfigManager) save(req LdapSaveRequest) (LdapSettingsDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loadErr != "" {
		return LdapSettingsDocument{}, errors.New("saved LDAP settings are unreadable; restore RUSTDESK_API_SETTINGS_KEY or move the existing settings file before saving")
	}

	next := ldapConfigFromEditable(req.Config)
	current := m.config()
	password := current.BindPassword
	useBasePassword := false
	if req.ClearBindPassword {
		password = ""
	} else if req.BindPassword != "" {
		password = req.BindPassword
	} else if len(m.key) == 0 {
		if current.BindPassword != "" {
			return LdapSettingsDocument{}, ErrLDAPSettingsKeyMissing
		}
		// A configuration with no bind secret can still be saved without a
		// settings key. It inherits the empty password from config.yaml.
		password = ""
		useBasePassword = true
	}

	// Environment-owned secrets must never be copied into the settings file.
	if ldapFieldLocked("bind_password") {
		password = ""
		useBasePassword = false
	}
	editableConfig := editableLDAPConfig(next)
	encryptedPassword, err := m.encrypt(password, ldapConfigAAD(editableConfig))
	if err != nil {
		return LdapSettingsDocument{}, err
	}

	now := time.Now().UTC()
	nextPersisted := &persistedLdapConfig{
		Config:                editableConfig,
		EncryptedBindPassword: encryptedPassword,
		UseBaseBindPassword:   useBasePassword,
		UpdatedAt:             now,
	}
	nextState := ldapSettingsFile{
		Version:  ldapSettingsVersion,
		Current:  nextPersisted,
		Previous: m.state.Current,
	}
	effective, err := m.effectiveConfig(nextPersisted)
	if err != nil {
		return LdapSettingsDocument{}, err
	}
	if err := m.writeState(nextState); err != nil {
		return LdapSettingsDocument{}, err
	}
	m.state = nextState
	m.current.Store(cloneLDAPConfig(effective))
	return m.documentUnlocked(), nil
}

func (m *ldapConfigManager) rollback() (LdapSettingsDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loadErr != "" {
		return LdapSettingsDocument{}, errors.New("saved LDAP settings are unreadable; restore RUSTDESK_API_SETTINGS_KEY before rollback")
	}
	if m.state.Previous == nil {
		return LdapSettingsDocument{}, ErrLDAPRollbackUnavailable
	}
	nextState := ldapSettingsFile{
		Version:  ldapSettingsVersion,
		Current:  m.state.Previous,
		Previous: m.state.Current,
	}
	effective, err := m.effectiveConfig(nextState.Current)
	if err != nil {
		return LdapSettingsDocument{}, err
	}
	if err := m.writeState(nextState); err != nil {
		return LdapSettingsDocument{}, err
	}
	m.state = nextState
	m.current.Store(cloneLDAPConfig(effective))
	return m.documentUnlocked(), nil
}

func (m *ldapConfigManager) documentUnlocked() LdapSettingsDocument {
	cfg := m.config()
	sources := make(map[string]string, len(ldapFieldEnvironments))
	for field := range ldapFieldEnvironments {
		sources[field] = "frontend"
		if ldapFieldLocked(field) {
			sources[field] = "environment"
		}
	}
	doc := LdapSettingsDocument{
		Config:             editableLDAPConfig(*cfg),
		PasswordConfigured: cfg.BindPassword != "",
		Sources:            sources,
		LockedFields:       ldapLockedFields(),
		Persisted:          m.state.Current != nil,
		CanSavePassword:    len(m.key) > 0,
		CanRollback:        m.state.Previous != nil,
		LoadError:          m.loadErr,
	}
	if m.state.Current != nil {
		updatedAt := m.state.Current.UpdatedAt
		doc.UpdatedAt = &updatedAt
	}
	return doc
}

func (m *ldapConfigManager) effectiveConfig(saved *persistedLdapConfig) (config.Ldap, error) {
	if saved == nil {
		return m.base, nil
	}
	cfg := ldapConfigFromEditable(saved.Config)
	password := ""
	if saved.UseBaseBindPassword {
		password = m.base.BindPassword
	} else {
		var err error
		password, err = m.decrypt(saved.EncryptedBindPassword, ldapConfigAAD(saved.Config))
		if err != nil {
			return config.Ldap{}, err
		}
	}
	cfg.BindPassword = password
	applyLDAPEnvironmentOverrides(&cfg, m.base)
	return normalizeLDAPConfig(cfg), nil
}

func (m *ldapConfigManager) writeState(state ldapSettingsFile) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".ldap-settings-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, m.path)
}

func ldapConfigAAD(cfg LdapEditableConfig) []byte {
	data, _ := json.Marshal(cfg)
	return data
}

func (m *ldapConfigManager) encrypt(plain string, additionalData []byte) (string, error) {
	if len(m.key) == 0 {
		if plain == "" {
			return "", nil
		}
		return "", ErrLDAPSettingsKeyMissing
	}
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), additionalData)
	return "v1:" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (m *ldapConfigManager) decrypt(ciphertext string, additionalData []byte) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if len(m.key) == 0 {
		return "", ErrLDAPSettingsKeyMissing
	}
	if !strings.HasPrefix(ciphertext, "v1:") {
		return "", errors.New("unsupported encrypted LDAP password format")
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, "v1:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("encrypted LDAP password is truncated")
	}
	nonce, payload := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, payload, additionalData)
	if err != nil {
		return "", errors.New("unable to decrypt LDAP bind password")
	}
	return string(plain), nil
}

func editableLDAPConfig(cfg config.Ldap) LdapEditableConfig {
	return LdapEditableConfig{
		Enable:                  cfg.Enable,
		URL:                     cfg.Url,
		TLSCAFile:               cfg.TlsCaFile,
		TLSVerify:               cfg.TlsVerify,
		BaseDN:                  cfg.BaseDn,
		BindDN:                  cfg.BindDn,
		ConnectTimeoutSeconds:   int(cfg.ConnectTimeout / time.Second),
		OperationTimeoutSeconds: int(cfg.OperationTimeout / time.Second),
		NestedGroups:            cfg.NestedGroups,
		EmergencyLocalAdmin:     cfg.EmergencyLocalAdmin,
		User: LdapEditableUserConfig{
			BaseDN:          cfg.User.BaseDn,
			EnableAttr:      cfg.User.EnableAttr,
			EnableAttrValue: cfg.User.EnableAttrValue,
			Filter:          cfg.User.Filter,
			Username:        cfg.User.Username,
			Email:           cfg.User.Email,
			FirstName:       cfg.User.FirstName,
			LastName:        cfg.User.LastName,
			Sync:            cfg.User.Sync,
			AdminGroup:      cfg.User.AdminGroup,
			AllowGroup:      cfg.User.AllowGroup,
		},
	}
}

func ldapConfigFromEditable(editable LdapEditableConfig) config.Ldap {
	return normalizeLDAPConfig(config.Ldap{
		Enable:              editable.Enable,
		Url:                 strings.TrimSpace(editable.URL),
		TlsCaFile:           strings.TrimSpace(editable.TLSCAFile),
		TlsVerify:           editable.TLSVerify,
		BaseDn:              strings.TrimSpace(editable.BaseDN),
		BindDn:              strings.TrimSpace(editable.BindDN),
		ConnectTimeout:      time.Duration(editable.ConnectTimeoutSeconds) * time.Second,
		OperationTimeout:    time.Duration(editable.OperationTimeoutSeconds) * time.Second,
		NestedGroups:        editable.NestedGroups,
		EmergencyLocalAdmin: editable.EmergencyLocalAdmin,
		User: config.LdapUser{
			BaseDn:          strings.TrimSpace(editable.User.BaseDN),
			EnableAttr:      strings.TrimSpace(editable.User.EnableAttr),
			EnableAttrValue: strings.TrimSpace(editable.User.EnableAttrValue),
			Filter:          strings.TrimSpace(editable.User.Filter),
			Username:        strings.TrimSpace(editable.User.Username),
			Email:           strings.TrimSpace(editable.User.Email),
			FirstName:       strings.TrimSpace(editable.User.FirstName),
			LastName:        strings.TrimSpace(editable.User.LastName),
			Sync:            editable.User.Sync,
			AdminGroup:      strings.TrimSpace(editable.User.AdminGroup),
			AllowGroup:      strings.TrimSpace(editable.User.AllowGroup),
		},
	})
}

var ldapFieldEnvironments = map[string]string{
	"enable":                    "RUSTDESK_API_LDAP_ENABLE",
	"url":                       "RUSTDESK_API_LDAP_URL",
	"tls_ca_file":               "RUSTDESK_API_LDAP_TLS_CA_FILE",
	"tls_verify":                "RUSTDESK_API_LDAP_TLS_VERIFY",
	"base_dn":                   "RUSTDESK_API_LDAP_BASE_DN",
	"bind_dn":                   "RUSTDESK_API_LDAP_BIND_DN",
	"bind_password":             "RUSTDESK_API_LDAP_BIND_PASSWORD",
	"connect_timeout_seconds":   "RUSTDESK_API_LDAP_CONNECT_TIMEOUT",
	"operation_timeout_seconds": "RUSTDESK_API_LDAP_OPERATION_TIMEOUT",
	"nested_groups":             "RUSTDESK_API_LDAP_NESTED_GROUPS",
	"emergency_local_admin":     "RUSTDESK_API_LDAP_EMERGENCY_LOCAL_ADMIN",
	"user.base_dn":              "RUSTDESK_API_LDAP_USER_BASE_DN",
	"user.enable_attr":          "RUSTDESK_API_LDAP_USER_ENABLE_ATTR",
	"user.enable_attr_value":    "RUSTDESK_API_LDAP_USER_ENABLE_ATTR_VALUE",
	"user.filter":               "RUSTDESK_API_LDAP_USER_FILTER",
	"user.username":             "RUSTDESK_API_LDAP_USER_USERNAME",
	"user.email":                "RUSTDESK_API_LDAP_USER_EMAIL",
	"user.first_name":           "RUSTDESK_API_LDAP_USER_FIRST_NAME",
	"user.last_name":            "RUSTDESK_API_LDAP_USER_LAST_NAME",
	"user.sync":                 "RUSTDESK_API_LDAP_USER_SYNC",
	"user.admin_group":          "RUSTDESK_API_LDAP_USER_ADMIN_GROUP",
	"user.allow_group":          "RUSTDESK_API_LDAP_USER_ALLOW_GROUP",
}

func ldapFieldLocked(field string) bool {
	envName, ok := ldapFieldEnvironments[field]
	if !ok {
		return false
	}
	_, present := os.LookupEnv(envName)
	return present
}

func ldapLockedFields() []string {
	locked := make([]string, 0)
	for field := range ldapFieldEnvironments {
		if ldapFieldLocked(field) {
			locked = append(locked, field)
		}
	}
	sort.Strings(locked)
	return locked
}

func applyLDAPEnvironmentOverrides(dst *config.Ldap, env config.Ldap) {
	if ldapFieldLocked("enable") {
		dst.Enable = env.Enable
	}
	if ldapFieldLocked("url") {
		dst.Url = env.Url
	}
	if ldapFieldLocked("tls_ca_file") {
		dst.TlsCaFile = env.TlsCaFile
	}
	if ldapFieldLocked("tls_verify") {
		dst.TlsVerify = env.TlsVerify
	}
	if ldapFieldLocked("base_dn") {
		dst.BaseDn = env.BaseDn
	}
	if ldapFieldLocked("bind_dn") {
		dst.BindDn = env.BindDn
	}
	if ldapFieldLocked("bind_password") {
		dst.BindPassword = env.BindPassword
	}
	if ldapFieldLocked("connect_timeout_seconds") {
		dst.ConnectTimeout = env.ConnectTimeout
	}
	if ldapFieldLocked("operation_timeout_seconds") {
		dst.OperationTimeout = env.OperationTimeout
	}
	if ldapFieldLocked("nested_groups") {
		dst.NestedGroups = env.NestedGroups
	}
	if ldapFieldLocked("emergency_local_admin") {
		dst.EmergencyLocalAdmin = env.EmergencyLocalAdmin
	}
	if ldapFieldLocked("user.base_dn") {
		dst.User.BaseDn = env.User.BaseDn
	}
	if ldapFieldLocked("user.enable_attr") {
		dst.User.EnableAttr = env.User.EnableAttr
	}
	if ldapFieldLocked("user.enable_attr_value") {
		dst.User.EnableAttrValue = env.User.EnableAttrValue
	}
	if ldapFieldLocked("user.filter") {
		dst.User.Filter = env.User.Filter
	}
	if ldapFieldLocked("user.username") {
		dst.User.Username = env.User.Username
	}
	if ldapFieldLocked("user.email") {
		dst.User.Email = env.User.Email
	}
	if ldapFieldLocked("user.first_name") {
		dst.User.FirstName = env.User.FirstName
	}
	if ldapFieldLocked("user.last_name") {
		dst.User.LastName = env.User.LastName
	}
	if ldapFieldLocked("user.sync") {
		dst.User.Sync = env.User.Sync
	}
	if ldapFieldLocked("user.admin_group") {
		dst.User.AdminGroup = env.User.AdminGroup
	}
	if ldapFieldLocked("user.allow_group") {
		dst.User.AllowGroup = env.User.AllowGroup
	}
}
