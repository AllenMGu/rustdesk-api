package utils

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// 安全策略配置
type SecurityPolicy struct {
	CaptchaThreshold int // 尝试失败次数达到验证码阈值，小于0表示不启用, 0表示强制启用
	BanThreshold     int // 尝试失败次数达到封禁阈值，为0表示不启用
	AttemptsWindow   time.Duration
	BanDuration      time.Duration

	// AccountFailThreshold 同一 (IP, 用户名) 在 AttemptsWindow 内失败达到该次数后，
	// 临时阻断该账号组合。小于0表示不启用，0(或缺省)表示使用默认值 10。
	// 与 IP 级策略相互独立：即使 ban-threshold=0，账号级保护仍然生效，
	// 避免桌面端 /api/login 入口在默认配置下完全没有爆破防护。
	AccountFailThreshold int
	// AccountBanDuration 账号组合被阻断的基础时长；连续触发时按 2 倍指数递增，
	// 上限 AccountBanMaxDuration。为0时使用默认值 5m。
	AccountBanDuration time.Duration
}

const (
	// AccountBanDefaultThreshold 账号级失败的默认阈值
	AccountBanDefaultThreshold = 10
	// AccountBanBaseDuration 账号级阻断的默认基础时长
	AccountBanBaseDuration = 5 * time.Minute
	// AccountBanMaxDuration 账号级阻断时长的上限（指数退避封顶）
	AccountBanMaxDuration = 60 * time.Minute
)

// 验证码提供者接口
type CaptchaProvider interface {
	Generate() (id string, content string, answer string, err error)
	//Validate(ip, code string) bool
	Expiration() time.Duration           // 验证码过期时间, 应该小于 AttemptsWindow
	Draw(content string) (string, error) // 绘制验证码
}

// 验证码元数据
type CaptchaMeta struct {
	Id        string
	Content   string
	Answer    string
	ExpiresAt time.Time
}

// IP封禁记录
type BanRecord struct {
	ExpiresAt time.Time
	Reason    string
}

// accountStateTTL streak（连续阻断退避计数）的最长保留时长：
// 超过该时长没有任何失败活动即视为攻击停止，退避计数过期清除，
// 防止 accountStates 随攻击过的 (IP, 用户名) 组合无界累积。
const accountStateTTL = 24 * time.Hour

// accountState 记录单个 (IP, 用户名) 组合的失败尝试与临时阻断状态
type accountState struct {
	failures     []time.Time
	blockedUntil time.Time
	streak       int // 连续触发阻断的次数，用于指数递增阻断时长
	lastActive   time.Time // 最近一次失败计数/阻断触发时间，用于 streak 过期清理
}

// 登录限制器
type LoginLimiter struct {
	mu            sync.Mutex
	policy        SecurityPolicy
	attempts      map[string][]time.Time //
	captchas      map[string]CaptchaMeta
	bannedIPs     map[string]BanRecord
	accountStates map[string]*accountState
	provider      CaptchaProvider
	cleanupStop   chan struct{}
}

var defaultSecurityPolicy = SecurityPolicy{
	CaptchaThreshold: 3,
	BanThreshold:     5,
	AttemptsWindow:   5 * time.Minute,
	BanDuration:      30 * time.Minute,
}

func NewLoginLimiter(policy SecurityPolicy) *LoginLimiter {
	// 设置默认值
	if policy.AttemptsWindow == 0 {
		policy.AttemptsWindow = 5 * time.Minute
	}
	if policy.BanDuration == 0 {
		policy.BanDuration = 30 * time.Minute
	}
	// 账号级保护：小于0显式禁用；0(缺省)使用默认阈值，保证默认配置下生效
	if policy.AccountFailThreshold == 0 {
		policy.AccountFailThreshold = AccountBanDefaultThreshold
	}
	if policy.AccountBanDuration <= 0 {
		policy.AccountBanDuration = AccountBanBaseDuration
	}

	ll := &LoginLimiter{
		policy:        policy,
		attempts:      make(map[string][]time.Time),
		captchas:      make(map[string]CaptchaMeta),
		bannedIPs:     make(map[string]BanRecord),
		accountStates: make(map[string]*accountState),
		cleanupStop:   make(chan struct{}),
	}
	go ll.cleanupRoutine()
	return ll
}

// 注册验证码提供者
func (ll *LoginLimiter) RegisterProvider(p CaptchaProvider) {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	ll.provider = p
}

// isDisabled 检查是否禁用登录限制
func (ll *LoginLimiter) isDisabled() bool {
	return ll.policy.CaptchaThreshold < 0 && ll.policy.BanThreshold == 0
}

// 记录登录失败尝试
func (ll *LoginLimiter) RecordFailedAttempt(ip string) {
	if ll.isDisabled() {
		return
	}
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if banned, _ := ll.isBanned(ip); banned {
		return
	}

	now := time.Now()
	windowStart := now.Add(-ll.policy.AttemptsWindow)

	// 清理过期尝试
	validAttempts := ll.pruneAttempts(ip, windowStart)

	// 记录新尝试
	validAttempts = append(validAttempts, now)
	ll.attempts[ip] = validAttempts

	// 检查封禁条件
	if ll.policy.BanThreshold > 0 && len(validAttempts) >= ll.policy.BanThreshold {
		ll.banIP(ip, "excessive failed attempts")
		return
	}

	return
}

// 生成验证码
func (ll *LoginLimiter) RequireCaptcha() (error, CaptchaMeta) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if ll.provider == nil {
		return errors.New("no captcha provider available"), CaptchaMeta{}
	}

	id, content, answer, err := ll.provider.Generate()
	if err != nil {
		return err, CaptchaMeta{}
	}

	// 存储验证码
	ll.captchas[id] = CaptchaMeta{
		Id:        id,
		Content:   content,
		Answer:    answer,
		ExpiresAt: time.Now().Add(ll.provider.Expiration()),
	}

	return nil, ll.captchas[id]
}

// 验证验证码
func (ll *LoginLimiter) VerifyCaptcha(id, answer string) bool {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	// 查找匹配验证码
	if ll.provider == nil {
		return false
	}

	// 获取并验证验证码
	captcha, exists := ll.captchas[id]
	if !exists {
		return false
	}

	// 清理过期验证码
	if time.Now().After(captcha.ExpiresAt) {
		delete(ll.captchas, id)
		return false
	}

	// 验证并清理状态
	if answer == captcha.Answer {
		delete(ll.captchas, id)
		return true
	}

	return false
}

func (ll *LoginLimiter) DrawCaptcha(content string) (err error, str string) {
	str, err = ll.provider.Draw(content)
	return
}

// 清除记录窗口
func (ll *LoginLimiter) RemoveAttempts(ip string) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	_, exists := ll.attempts[ip]
	if exists {
		delete(ll.attempts, ip)
	}
}

// CheckSecurityStatus 检查安全状态
func (ll *LoginLimiter) CheckSecurityStatus(ip string) (banned bool, captchaRequired bool) {
	if ll.isDisabled() {
		return
	}
	ll.mu.Lock()
	defer ll.mu.Unlock()

	// 检查封禁状态
	if banned, _ = ll.isBanned(ip); banned {
		return
	}

	// 清理过期数据
	ll.pruneAttempts(ip, time.Now().Add(-ll.policy.AttemptsWindow))

	// 检查验证码要求
	captchaRequired = len(ll.attempts[ip]) >= ll.policy.CaptchaThreshold

	return
}

// BanUntil 返回 IP 当前封禁的截止时间（未封禁或已过期返回零值）
func (ll *LoginLimiter) BanUntil(ip string) time.Time {
	if ll.isDisabled() {
		return time.Time{}
	}
	ll.mu.Lock()
	defer ll.mu.Unlock()
	if banned, record := ll.isBanned(ip); banned {
		return record.ExpiresAt
	}
	return time.Time{}
}

func accountKey(ip, username string) string {
	return strings.ToLower(strings.TrimSpace(ip)) + "|" + strings.ToLower(strings.TrimSpace(username))
}

// RecordAccountFailure 记录一次 (IP, 用户名) 登录失败。
// 在 AttemptsWindow 内失败次数达到 AccountFailThreshold 时临时阻断该组合，
// 阻断时长按 AccountBanDuration 的 2 的幂次指数递增（封顶 AccountBanMaxDuration），
// 登录成功（ClearAccountFailures）后计数与退避一并重置。
func (ll *LoginLimiter) RecordAccountFailure(ip, username string) {
	if ll.accountDisabled() || ip == "" || username == "" {
		return
	}
	ll.mu.Lock()
	defer ll.mu.Unlock()

	key := accountKey(ip, username)
	st := ll.accountState(key)
	now := time.Now()
	// 阻断期间不重复计数，也不延长阻断
	if now.Before(st.blockedUntil) {
		return
	}

	cutoff := now.Add(-ll.policy.AttemptsWindow)
	st.failures = filterAfter(st.failures, cutoff)
	st.failures = append(st.failures, now)
	st.lastActive = now

	if len(st.failures) >= ll.policy.AccountFailThreshold {
		st.blockedUntil = now.Add(ll.accountBanDurationFor(st.streak))
		st.streak++
		// 阻断后重新计数；streak 保留，使连续滥用逐级升级
		st.failures = nil
	}
}

// CheckAccountBlock 返回 (IP, 用户名) 组合当前是否被临时阻断及阻断截止时间。
func (ll *LoginLimiter) CheckAccountBlock(ip, username string) (bool, time.Time) {
	if ll.accountDisabled() || ip == "" || username == "" {
		return false, time.Time{}
	}
	ll.mu.Lock()
	defer ll.mu.Unlock()

	st, ok := ll.accountStates[accountKey(ip, username)]
	if !ok || time.Now().After(st.blockedUntil) {
		return false, time.Time{}
	}
	return true, st.blockedUntil
}

// ClearAccountFailures 登录成功后清除 (IP, 用户名) 的失败计数与阻断状态。
func (ll *LoginLimiter) ClearAccountFailures(ip, username string) {
	if ll.accountDisabled() || ip == "" || username == "" {
		return
	}
	ll.mu.Lock()
	defer ll.mu.Unlock()
	delete(ll.accountStates, accountKey(ip, username))
}

func (ll *LoginLimiter) accountDisabled() bool {
	return ll.policy.AccountFailThreshold < 0
}

// accountBanDurationFor 计算第 streak 次阻断的时长（指数退避，带上限）
func (ll *LoginLimiter) accountBanDurationFor(streak int) time.Duration {
	dur := ll.policy.AccountBanDuration
	if dur <= 0 {
		dur = AccountBanBaseDuration
	}
	for i := 0; i < streak && dur < AccountBanMaxDuration; i++ {
		dur *= 2
	}
	if dur > AccountBanMaxDuration {
		dur = AccountBanMaxDuration
	}
	return dur
}

// accountState 返回组合对应的状态（不存在则创建）；调用方必须持有 ll.mu
func (ll *LoginLimiter) accountState(key string) *accountState {
	st, ok := ll.accountStates[key]
	if !ok {
		st = &accountState{}
		ll.accountStates[key] = st
	}
	return st
}

func filterAfter(times []time.Time, cutoff time.Time) []time.Time {
	valid := make([]time.Time, 0, len(times))
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	return valid
}

// 后台清理任务
func (ll *LoginLimiter) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ll.cleanupExpired()
		case <-ll.cleanupStop:
			return
		}
	}
}

// 内部工具方法
func (ll *LoginLimiter) isBanned(ip string) (bool, BanRecord) {
	record, exists := ll.bannedIPs[ip]
	if !exists {
		return false, BanRecord{}
	}
	if time.Now().After(record.ExpiresAt) {
		delete(ll.bannedIPs, ip)
		return false, BanRecord{}
	}
	return true, record
}

func (ll *LoginLimiter) banIP(ip, reason string) {
	ll.bannedIPs[ip] = BanRecord{
		ExpiresAt: time.Now().Add(ll.policy.BanDuration),
		Reason:    reason,
	}
	delete(ll.attempts, ip)
	delete(ll.captchas, ip)
}

func (ll *LoginLimiter) pruneAttempts(ip string, cutoff time.Time) []time.Time {
	var valid []time.Time
	for _, t := range ll.attempts[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		delete(ll.attempts, ip)
	} else {
		ll.attempts[ip] = valid
	}
	return valid
}

func (ll *LoginLimiter) pruneCaptchas(id string) {
	if captcha, exists := ll.captchas[id]; exists {
		if time.Now().After(captcha.ExpiresAt) {
			delete(ll.captchas, id)
		}
	}
}

func (ll *LoginLimiter) cleanupExpired() {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	now := time.Now()

	// 清理封禁记录
	for ip, record := range ll.bannedIPs {
		if now.After(record.ExpiresAt) {
			delete(ll.bannedIPs, ip)
		}
	}

	// 清理尝试记录
	for ip := range ll.attempts {
		ll.pruneAttempts(ip, now.Add(-ll.policy.AttemptsWindow))
	}

	// 清理验证码
	for id := range ll.captchas {
		ll.pruneCaptchas(id)
	}

	// 清理账号级失败记录：
	// 仅当"无有效失败 + 未被阻断 + 无退避计数"时才移除。
	// 关键修复：阻断过期后 streak 仍保留（指数退避 5m→10m→20m→60m 必须跨周期
	// 持续生效），直到登录成功（ClearAccountFailures）重置，或超过
	// accountStateTTL（24h）无任何失败活动后过期清除（防状态无界累积）。
	// 此前实现会在阻断过期后直接删除 accountState，连带把 streak 一并丢弃，
	// 导致"指数退避"实际永远停在基础时长。
	for key, st := range ll.accountStates {
		st.failures = filterAfter(st.failures, now.Add(-ll.policy.AttemptsWindow))
		streakExpired := st.streak > 0 && (st.lastActive.IsZero() || now.After(st.lastActive.Add(accountStateTTL)))
		if len(st.failures) == 0 && !now.Before(st.blockedUntil) && (st.streak == 0 || streakExpired) {
			delete(ll.accountStates, key)
		}
	}
}
