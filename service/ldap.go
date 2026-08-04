package service

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/model"
	log "github.com/sirupsen/logrus"
)

var (
	ErrUrlParseFailed        = errors.New("UrlParseFailed")
	ErrFileReadFailed        = errors.New("FileReadFailed")
	ErrLdapNotEnabled        = errors.New("LdapNotEnabled")
	ErrLdapUserDisabled      = errors.New("UserDisabledAtLdap")
	ErrLdapUserNotFound      = errors.New("UserNotFound")
	ErrLdapMailNotMatch      = errors.New("MailNotMatch")
	ErrLdapConnectFailed     = errors.New("LdapConnectFailed")
	ErrLdapSearchFailed      = errors.New("LdapSearchRequestFailed")
	ErrLdapTlsFailed         = errors.New("LdapStartTLSFailed")
	ErrLdapBindService       = errors.New("LdapBindServiceFailed")
	ErrLdapBindFailed        = errors.New("LdapBindFailed")
	ErrLdapToLocalUserFailed = errors.New("LdapToLocalUserFailed")
	ErrLdapCreateUserFailed  = errors.New("LdapCreateUserFailed")
	ErrLdapPasswordNotMatch  = errors.New("PasswordNotMatch")
	ErrLdapGroupNotFound     = errors.New("LdapGroupNotFound")
)

const activeDirectoryMatchingRuleInChain = "1.2.840.113556.1.4.1941"

var ldapAttributePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9.-]*$`)

// LdapService authenticates users against an atomic LDAP configuration
// snapshot. The snapshot can be replaced by the admin settings API without
// racing concurrent logins.
type LdapService struct {
	configManager *ldapConfigManager
}

func NewLdapService(base config.Ldap, logger *log.Logger) *LdapService {
	return &LdapService{configManager: newLDAPConfigManager(base, logger)}
}

func (ls *LdapService) currentConfig() *config.Ldap {
	if ls != nil && ls.configManager != nil {
		return ls.configManager.config()
	}
	if Config != nil {
		return cloneLDAPConfig(normalizeLDAPConfig(Config.Ldap))
	}
	return cloneLDAPConfig(normalizeLDAPConfig(config.Ldap{}))
}

func (ls *LdapService) Enabled() bool {
	return ls.currentConfig().Enable
}

func (ls *LdapService) EmergencyLocalAdminEnabled() bool {
	return ls.currentConfig().EmergencyLocalAdmin
}

func (ls *LdapService) SettingsDocument() LdapSettingsDocument {
	return ls.configManager.document()
}

func (ls *LdapService) SaveSettings(req LdapSaveRequest) (LdapSettingsDocument, error) {
	cfg := ldapConfigFromEditable(req.Config)
	current := ls.currentConfig()
	if req.ClearBindPassword {
		cfg.BindPassword = ""
	} else if req.BindPassword != "" {
		cfg.BindPassword = req.BindPassword
	} else {
		cfg.BindPassword = current.BindPassword
	}
	applyLDAPEnvironmentOverrides(&cfg, *current)
	if err := ls.ValidateConfig(&cfg); err != nil {
		return LdapSettingsDocument{}, err
	}
	return ls.configManager.save(req)
}

func (ls *LdapService) RollbackSettings() (LdapSettingsDocument, error) {
	return ls.configManager.rollback()
}

// LdapUser represents user attributes returned by LDAP.
type LdapUser struct {
	Dn              string   `json:"dn"`
	Username        string   `json:"username"`
	Email           string   `json:"email"`
	FirstName       string   `json:"first_name"`
	LastName        string   `json:"last_name"`
	MemberOf        []string `json:"-"`
	EnableAttrValue string   `json:"-"`
	Enabled         bool     `json:"enabled"`
}

func (lu *LdapUser) Name() string {
	return strings.TrimSpace(fmt.Sprintf("%s %s", lu.FirstName, lu.LastName))
}

func (lu *LdapUser) ToUser(u *model.User) *model.User {
	if u == nil {
		u = &model.User{}
	}
	u.Username = lu.Username
	u.Email = lu.Email
	u.Nickname = lu.Name()
	if lu.Enabled {
		u.Status = model.COMMON_STATUS_ENABLE
	} else {
		u.Status = model.COMMON_STATUS_DISABLED
	}
	return u
}

func (ls *LdapService) tlsConfig(cfg *config.Ldap, parsedURL *url.URL) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         parsedURL.Hostname(),
		InsecureSkipVerify: !cfg.TlsVerify, // #nosec G402 -- explicitly controlled by the administrator.
	}
	if cfg.TlsCaFile == "" {
		return tlsConfig, nil
	}
	caCert, err := os.ReadFile(cfg.TlsCaFile)
	if err != nil {
		return nil, errors.Join(ErrFileReadFailed, err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, errors.Join(ErrLdapTlsFailed, errors.New("failed to append CA certificate"))
	}
	tlsConfig.RootCAs = caCertPool
	return tlsConfig, nil
}

func (ls *LdapService) dial(cfg *config.Ldap) (*ldap.Conn, error) {
	parsedURL, err := url.Parse(cfg.Url)
	if err != nil {
		return nil, errors.Join(ErrUrlParseFailed, err)
	}
	if parsedURL.Host == "" || (parsedURL.Scheme != "ldap" && parsedURL.Scheme != "ldaps") {
		return nil, errors.Join(ErrUrlParseFailed, errors.New("LDAP URL must use ldap:// or ldaps:// and include a host"))
	}

	dialer := &net.Dialer{Timeout: cfg.ConnectTimeout}
	options := []ldap.DialOpt{ldap.DialWithDialer(dialer)}
	if parsedURL.Scheme == "ldaps" {
		tlsConfig, err := ls.tlsConfig(cfg, parsedURL)
		if err != nil {
			return nil, err
		}
		options = append(options, ldap.DialWithTLSConfig(tlsConfig))
	}
	conn, err := ldap.DialURL(cfg.Url, options...)
	if err != nil {
		return nil, errors.Join(ErrLdapConnectFailed, err)
	}
	conn.SetTimeout(cfg.OperationTimeout)
	return conn, nil
}

func (ls *LdapService) connectAndBind(cfg *config.Ldap, username, password string) (*ldap.Conn, error) {
	conn, err := ls.dial(cfg)
	if err != nil {
		return nil, err
	}
	if username != "" || password != "" {
		if username == "" || password == "" {
			conn.Close()
			return nil, errors.Join(ErrLdapBindService, errors.New("bind DN and password must both be set"))
		}
		if err := conn.Bind(username, password); err != nil {
			conn.Close()
			return nil, errors.Join(ErrLdapBindService, err)
		}
	}
	return conn, nil
}

func (ls *LdapService) connectAndBindAdmin(cfg *config.Ldap) (*ldap.Conn, error) {
	return ls.connectAndBind(cfg, cfg.BindDn, cfg.BindPassword)
}

func (ls *LdapService) verifyCredentials(cfg *config.Ldap, username, password string) error {
	if strings.TrimSpace(username) == "" || password == "" {
		return ErrLdapPasswordNotMatch
	}
	ldapConn, err := ls.connectAndBind(cfg, username, password)
	if err != nil {
		return ErrLdapPasswordNotMatch
	}
	ldapConn.Close()
	return nil
}

func (ls *LdapService) Authenticate(username, password string) (*model.User, error) {
	cfg := ls.currentConfig()
	ldapUser, err := ls.getUserInfoByUsername(cfg, username)
	if err != nil {
		return nil, err
	}
	if !ldapUser.Enabled {
		return nil, ErrLdapUserDisabled
	}

	isAdmin := ls.isUserAdmin(cfg, ldapUser)
	if !isAdmin && cfg.User.AllowGroup != "" && !ls.isUserInGroup(cfg, ldapUser, cfg.User.AllowGroup) {
		return nil, errors.New("user not in allowed group")
	}
	if err := ls.verifyCredentials(cfg, ldapUser.Dn, password); err != nil {
		return nil, err
	}
	user, err := ls.mapToLocalUser(cfg, ldapUser)
	if err != nil {
		return nil, errors.Join(ErrLdapToLocalUserFailed, err)
	}
	return user, nil
}

func (ls *LdapService) resolveGroupDN(cfg *config.Ldap, group string) (string, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return "", ErrLdapGroupNotFound
	}
	if _, err := ldap.ParseDN(group); err == nil && strings.Contains(group, "=") {
		return group, nil
	}
	escaped := ldap.EscapeFilter(group)
	request := ldap.NewSearchRequest(
		cfg.BaseDn,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		2,
		operationLimitSeconds(cfg),
		false,
		fmt.Sprintf("(&(objectClass=group)(|(cn=%s)(sAMAccountName=%s)))", escaped, escaped),
		[]string{"dn"},
		nil,
	)
	result, err := ls.searchResult(cfg, request)
	if err != nil || len(result.Entries) != 1 {
		return "", ErrLdapGroupNotFound
	}
	return result.Entries[0].DN, nil
}

func (ls *LdapService) isUserInGroup(cfg *config.Ldap, ldapUser *LdapUser, group string) bool {
	groupDN, err := ls.resolveGroupDN(cfg, group)
	if err != nil {
		return false
	}
	for _, memberOf := range ldapUser.MemberOf {
		if strings.EqualFold(memberOf, groupDN) {
			return true
		}
	}

	if cfg.NestedGroups {
		request := ldap.NewSearchRequest(
			ldapUser.Dn,
			ldap.ScopeBaseObject,
			ldap.NeverDerefAliases,
			1,
			operationLimitSeconds(cfg),
			false,
			fmt.Sprintf("(memberOf:%s:=%s)", activeDirectoryMatchingRuleInChain, ldap.EscapeFilter(groupDN)),
			[]string{"dn"},
			nil,
		)
		if result, err := ls.searchResult(cfg, request); err == nil && len(result.Entries) > 0 {
			return true
		}
	}

	request := ldap.NewSearchRequest(
		groupDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		operationLimitSeconds(cfg),
		false,
		fmt.Sprintf("(member=%s)", ldap.EscapeFilter(ldapUser.Dn)),
		[]string{"dn"},
		nil,
	)
	result, err := ls.searchResult(cfg, request)
	return err == nil && len(result.Entries) > 0
}

func (ls *LdapService) mapToLocalUser(cfg *config.Ldap, lu *LdapUser) (*model.User, error) {
	userService := &UserService{}
	localUser := userService.InfoByUsername(lu.Username)
	isAdmin := ls.isUserAdmin(cfg, lu)
	if localUser.Id == 0 {
		newUser := lu.ToUser(nil)
		newUser.IsAdmin = &isAdmin
		newUser.GroupId = 1
		if err := DB.Create(newUser).Error; err != nil {
			return nil, errors.Join(ErrLdapCreateUserFailed, err)
		}
		return userService.InfoByUsername(lu.Username), nil
	}

	if cfg.User.Sync {
		originalEmail := localUser.Email
		originalNickname := localUser.Nickname
		originalIsAdmin := localUser.IsAdmin
		originalStatus := localUser.Status
		lu.ToUser(localUser)
		localUser.IsAdmin = &isAdmin
		if err := userService.Update(localUser); err != nil {
			localUser.Email = originalEmail
			localUser.Nickname = originalNickname
			localUser.IsAdmin = originalIsAdmin
			localUser.Status = originalStatus
		}
	}
	return localUser, nil
}

func (ls *LdapService) IsUsernameExists(username string) bool {
	cfg := ls.currentConfig()
	if !cfg.Enable {
		return false
	}
	sr, err := ls.usernameSearchResult(cfg, username)
	return err == nil && len(sr.Entries) > 0
}

func (ls *LdapService) IsEmailExists(email string) bool {
	cfg := ls.currentConfig()
	if !cfg.Enable {
		return false
	}
	sr, err := ls.emailSearchResult(cfg, email)
	return err == nil && len(sr.Entries) > 0
}

func (ls *LdapService) GetUserInfoByUsernameLdap(username string) (*LdapUser, error) {
	return ls.getUserInfoByUsername(ls.currentConfig(), username)
}

func (ls *LdapService) getUserInfoByUsername(cfg *config.Ldap, username string) (*LdapUser, error) {
	if !cfg.Enable {
		return nil, ErrLdapNotEnabled
	}
	sr, err := ls.usernameSearchResult(cfg, username)
	if err != nil {
		return nil, errors.Join(ErrLdapSearchFailed, err)
	}
	if len(sr.Entries) != 1 {
		return nil, ErrLdapUserNotFound
	}
	return ls.userResultToLdapUser(cfg, sr.Entries[0]), nil
}

func (ls *LdapService) GetUserInfoByUsernameLocal(username string) (*model.User, error) {
	cfg := ls.currentConfig()
	ldapUser, err := ls.getUserInfoByUsername(cfg, username)
	if err != nil {
		return &model.User{}, err
	}
	return ls.mapToLocalUser(cfg, ldapUser)
}

func (ls *LdapService) GetUserInfoByEmailLdap(email string) (*LdapUser, error) {
	return ls.getUserInfoByEmail(ls.currentConfig(), email)
}

func (ls *LdapService) getUserInfoByEmail(cfg *config.Ldap, email string) (*LdapUser, error) {
	if !cfg.Enable {
		return nil, ErrLdapNotEnabled
	}
	sr, err := ls.emailSearchResult(cfg, email)
	if err != nil {
		return nil, errors.Join(ErrLdapSearchFailed, err)
	}
	if len(sr.Entries) != 1 {
		return nil, ErrLdapUserNotFound
	}
	return ls.userResultToLdapUser(cfg, sr.Entries[0]), nil
}

func (ls *LdapService) GetUserInfoByEmailLocal(email string) (*model.User, error) {
	cfg := ls.currentConfig()
	ldapUser, err := ls.getUserInfoByEmail(cfg, email)
	if err != nil {
		return &model.User{}, err
	}
	return ls.mapToLocalUser(cfg, ldapUser)
}

func (ls *LdapService) usernameSearchResult(cfg *config.Ldap, username string) (*ldap.SearchResult, error) {
	fields := ls.fieldUsernames(cfg)
	filters := make([]string, 0, len(fields))
	for _, field := range fields {
		filters = append(filters, ls.filterField(field, username))
	}
	filter := filters[0]
	if len(filters) > 1 {
		filter = "(|" + strings.Join(filters, "") + ")"
	}
	return ls.searchResult(cfg, ls.buildUserSearchRequest(cfg, filter))
}

func (ls *LdapService) emailSearchResult(cfg *config.Ldap, email string) (*ldap.SearchResult, error) {
	return ls.searchResult(cfg, ls.buildUserSearchRequest(cfg, ls.filterField(ls.fieldEmail(cfg), email)))
}

func (ls *LdapService) searchResult(cfg *config.Ldap, searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error) {
	ldapConn, err := ls.connectAndBindAdmin(cfg)
	if err != nil {
		return nil, err
	}
	defer ldapConn.Close()
	return ldapConn.Search(searchRequest)
}

func operationLimitSeconds(cfg *config.Ldap) int {
	seconds := int(cfg.OperationTimeout / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func (ls *LdapService) buildUserSearchRequest(cfg *config.Ldap, filter string) *ldap.SearchRequest {
	filterConfig := cfg.User.Filter
	if filterConfig == "" {
		filterConfig = "(objectClass=person)"
	}
	return ldap.NewSearchRequest(
		ls.baseDnUser(cfg),
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		2,
		operationLimitSeconds(cfg),
		false,
		fmt.Sprintf("(&%s%s)", filterConfig, filter),
		ls.buildUserAttributes(cfg),
		nil,
	)
}

func appendUniqueAttribute(attributes []string, field string) []string {
	if field == "" {
		return attributes
	}
	for _, existing := range attributes {
		if strings.EqualFold(existing, field) {
			return attributes
		}
	}
	return append(attributes, field)
}

func (ls *LdapService) buildUserAttributes(cfg *config.Ldap) []string {
	attributes := []string{"dn"}
	for _, field := range ls.fieldUsernames(cfg) {
		attributes = appendUniqueAttribute(attributes, field)
	}
	attributes = appendUniqueAttribute(attributes, ls.fieldEmail(cfg))
	attributes = appendUniqueAttribute(attributes, ls.fieldFirstName(cfg))
	attributes = appendUniqueAttribute(attributes, ls.fieldLastName(cfg))
	attributes = appendUniqueAttribute(attributes, "memberOf")
	attributes = appendUniqueAttribute(attributes, cfg.User.EnableAttr)
	return attributes
}

func (ls *LdapService) userResultToLdapUser(cfg *config.Ldap, entry *ldap.Entry) *LdapUser {
	lu := &LdapUser{
		Dn:              entry.DN,
		Username:        entry.GetAttributeValue(ls.fieldUsername(cfg)),
		Email:           entry.GetAttributeValue(ls.fieldEmail(cfg)),
		FirstName:       entry.GetAttributeValue(ls.fieldFirstName(cfg)),
		LastName:        entry.GetAttributeValue(ls.fieldLastName(cfg)),
		MemberOf:        entry.GetAttributeValues("memberOf"),
		EnableAttrValue: entry.GetAttributeValue(cfg.User.EnableAttr),
	}
	ls.isUserEnabled(cfg, lu)
	return lu
}

func (ls *LdapService) filterField(field, value string) string {
	return fmt.Sprintf("(%s=%s)", field, ldap.EscapeFilter(value))
}

func (ls *LdapService) fieldUsernames(cfg *config.Ldap) []string {
	raw := cfg.User.Username
	if strings.TrimSpace(raw) == "" {
		return []string{"uid"}
	}
	fields := make([]string, 0, 2)
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field != "" {
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return []string{"uid"}
	}
	return fields
}

func (ls *LdapService) fieldUsername(cfg *config.Ldap) string { return ls.fieldUsernames(cfg)[0] }
func (ls *LdapService) fieldEmail(cfg *config.Ldap) string {
	if cfg.User.Email == "" {
		return "mail"
	}
	return cfg.User.Email
}
func (ls *LdapService) fieldFirstName(cfg *config.Ldap) string {
	if cfg.User.FirstName == "" {
		return "givenName"
	}
	return cfg.User.FirstName
}
func (ls *LdapService) fieldLastName(cfg *config.Ldap) string {
	if cfg.User.LastName == "" {
		return "sn"
	}
	return cfg.User.LastName
}
func (ls *LdapService) baseDnUser(cfg *config.Ldap) string {
	if cfg.User.BaseDn == "" {
		return cfg.BaseDn
	}
	return cfg.User.BaseDn
}

func (ls *LdapService) isUserAdmin(cfg *config.Ldap, ldapUser *LdapUser) bool {
	return cfg.User.AdminGroup != "" && ls.isUserInGroup(cfg, ldapUser, cfg.User.AdminGroup)
}

func (ls *LdapService) isUserEnabled(cfg *config.Ldap, ldapUser *LdapUser) bool {
	enableAttr := cfg.User.EnableAttr
	enableAttrValue := cfg.User.EnableAttrValue
	if enableAttr == "" || enableAttrValue == "" {
		ldapUser.Enabled = true
		return true
	}
	if strings.EqualFold(enableAttr, "userAccountControl") {
		userAccountControl, err := strconv.Atoi(ldapUser.EnableAttrValue)
		if err != nil {
			ldapUser.Enabled = false
			return false
		}
		ldapUser.Enabled = userAccountControl&0x2 == 0
		return ldapUser.Enabled
	}
	ldapUser.Enabled = ldapUser.EnableAttrValue == enableAttrValue
	return ldapUser.Enabled
}

func (ls *LdapService) ValidateConfig(cfg *config.Ldap) error {
	if cfg.ConnectTimeout < time.Second || cfg.ConnectTimeout > 120*time.Second {
		return errors.New("connect timeout must be between 1 and 120 seconds")
	}
	if cfg.OperationTimeout < time.Second || cfg.OperationTimeout > 120*time.Second {
		return errors.New("operation timeout must be between 1 and 120 seconds")
	}
	if !cfg.Enable && strings.TrimSpace(cfg.Url) == "" {
		return nil
	}
	parsedURL, err := url.Parse(cfg.Url)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "ldap" && parsedURL.Scheme != "ldaps") {
		return errors.New("LDAP URL must use ldap:// or ldaps:// and include a host")
	}
	if cfg.BaseDn == "" {
		return errors.New("base DN is required")
	}
	if _, err := ldap.ParseDN(cfg.BaseDn); err != nil {
		return fmt.Errorf("invalid base DN: %w", err)
	}
	if cfg.User.BaseDn != "" {
		if _, err := ldap.ParseDN(cfg.User.BaseDn); err != nil {
			return fmt.Errorf("invalid user base DN: %w", err)
		}
	}
	if cfg.BindDn != "" {
		if _, err := ldap.ParseDN(cfg.BindDn); err != nil && !strings.Contains(cfg.BindDn, "@") {
			return fmt.Errorf("invalid bind DN: %w", err)
		}
	}
	if cfg.Enable && (cfg.BindDn == "") != (cfg.BindPassword == "") {
		return errors.New("bind DN and bind password must both be set, or both be empty for anonymous search")
	}
	filter := cfg.User.Filter
	if filter == "" {
		filter = "(objectClass=person)"
	}
	if _, err := ldap.CompileFilter(filter); err != nil {
		return fmt.Errorf("invalid user filter: %w", err)
	}
	attributes := append([]string{}, ls.fieldUsernames(cfg)...)
	attributes = append(attributes, ls.fieldEmail(cfg), ls.fieldFirstName(cfg), ls.fieldLastName(cfg))
	if cfg.User.EnableAttr != "" {
		attributes = append(attributes, cfg.User.EnableAttr)
	}
	for _, attribute := range attributes {
		if !ldapAttributePattern.MatchString(attribute) {
			return fmt.Errorf("invalid LDAP attribute %q", attribute)
		}
	}
	return nil
}

type LdapTestRequest struct {
	Config       LdapEditableConfig `json:"config"`
	BindPassword string             `json:"bind_password"`
	TestUsername string             `json:"test_username"`
	TestPassword string             `json:"test_password"`
}

type LdapTestStep struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type LdapTestResult struct {
	Success   bool           `json:"success"`
	Steps     []LdapTestStep `json:"steps"`
	User      *LdapUser      `json:"user,omitempty"`
	IsAdmin   bool           `json:"is_admin"`
	IsAllowed bool           `json:"is_allowed"`
}

func (ls *LdapService) TestConfiguration(req LdapTestRequest) LdapTestResult {
	result := LdapTestResult{Success: false}
	cfg := ldapConfigFromEditable(req.Config)
	current := ls.currentConfig()
	if req.BindPassword != "" {
		cfg.BindPassword = req.BindPassword
	} else {
		cfg.BindPassword = current.BindPassword
	}
	applyLDAPEnvironmentOverrides(&cfg, *current)
	if err := ls.ValidateConfig(&cfg); err != nil {
		result.Steps = append(result.Steps, LdapTestStep{Name: "validation", Message: err.Error()})
		return result
	}
	result.Steps = append(result.Steps, LdapTestStep{Name: "validation", Success: true, Message: "configuration is valid"})

	conn, err := ls.connectAndBindAdmin(&cfg)
	if err != nil {
		result.Steps = append(result.Steps, LdapTestStep{Name: "service_bind", Message: err.Error()})
		return result
	}
	conn.Close()
	result.Steps = append(result.Steps, LdapTestStep{Name: "service_bind", Success: true, Message: "connected and bound successfully"})

	if strings.TrimSpace(req.TestUsername) == "" {
		result.Success = true
		return result
	}
	searchResult, err := ls.usernameSearchResult(&cfg, req.TestUsername)
	if err != nil {
		result.Steps = append(result.Steps, LdapTestStep{Name: "user_search", Message: err.Error()})
		return result
	}
	if len(searchResult.Entries) != 1 {
		result.Steps = append(result.Steps, LdapTestStep{Name: "user_search", Message: ErrLdapUserNotFound.Error()})
		return result
	}
	user := ls.userResultToLdapUser(&cfg, searchResult.Entries[0])
	result.User = user
	result.Steps = append(result.Steps, LdapTestStep{Name: "user_search", Success: true, Message: "user found"})
	result.IsAdmin = ls.isUserAdmin(&cfg, user)
	result.IsAllowed = result.IsAdmin || cfg.User.AllowGroup == "" || ls.isUserInGroup(&cfg, user, cfg.User.AllowGroup)
	if !result.IsAllowed {
		result.Steps = append(result.Steps, LdapTestStep{Name: "group_access", Message: "user is not in an allowed group"})
		return result
	}
	result.Steps = append(result.Steps, LdapTestStep{Name: "group_access", Success: true, Message: "group access granted"})
	if req.TestPassword != "" {
		if err := ls.verifyCredentials(&cfg, user.Dn, req.TestPassword); err != nil {
			result.Steps = append(result.Steps, LdapTestStep{Name: "user_bind", Message: err.Error()})
			return result
		}
		result.Steps = append(result.Steps, LdapTestStep{Name: "user_bind", Success: true, Message: "user credentials are valid"})
	}
	result.Success = true
	return result
}
