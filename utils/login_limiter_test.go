package utils

import (
	"fmt"
	"github.com/google/uuid"
	"testing"
	"time"
)

type MockCaptchaProvider struct{}

func (p *MockCaptchaProvider) Generate() (string, string, string, error) {
	id := uuid.New().String()
	content := uuid.New().String()
	answer := uuid.New().String()
	return id, content, answer, nil
}

func (p *MockCaptchaProvider) Expiration() time.Duration {
	return 2 * time.Second
}
func (p *MockCaptchaProvider) Draw(content string) (string, error) {
	return "MOCK", nil
}

func TestSecurityWorkflow(t *testing.T) {
	policy := SecurityPolicy{
		CaptchaThreshold: 3,
		BanThreshold:     5,
		AttemptsWindow:   5 * time.Minute,
		BanDuration:      5 * time.Minute,
	}
	limiter := NewLoginLimiter(policy)
	ip := "192.168.1.100"

	// 测试正常失败记录
	for i := 0; i < 3; i++ {
		limiter.RecordFailedAttempt(ip)
	}
	isBanned, capRequired := limiter.CheckSecurityStatus(ip)
	fmt.Printf("IP: %s, Banned: %v, Captcha Required: %v\n", ip, isBanned, capRequired)
	if isBanned {
		t.Error("IP should not be banned yet")
	}
	if !capRequired {
		t.Error("Captcha should be required")
	}
	// 测试触发封禁
	for i := 0; i < 3; i++ {
		limiter.RecordFailedAttempt(ip)
		isBanned, capRequired = limiter.CheckSecurityStatus(ip)
		fmt.Printf("IP: %s, Banned: %v, Captcha Required: %v\n", ip, isBanned, capRequired)
	}

	// 测试封禁状态
	if isBanned, _ = limiter.CheckSecurityStatus(ip); !isBanned {
		t.Error("IP should be banned")
	}
}

func TestCaptchaFlow(t *testing.T) {
	policy := SecurityPolicy{CaptchaThreshold: 2}
	limiter := NewLoginLimiter(policy)
	limiter.RegisterProvider(&MockCaptchaProvider{})
	ip := "10.0.0.1"

	// 触发验证码要求
	limiter.RecordFailedAttempt(ip)
	limiter.RecordFailedAttempt(ip)

	// 检查状态
	if _, need := limiter.CheckSecurityStatus(ip); !need {
		t.Error("应该需要验证码")
	}

	// 生成验证码
	err, capc := limiter.RequireCaptcha()
	if err != nil {
		t.Fatalf("生成验证码失败: %v", err)
	}
	fmt.Printf("验证码内容: %#v\n", capc)

	// 验证成功
	if !limiter.VerifyCaptcha(capc.Id, capc.Answer) {
		t.Error("验证码应该验证成功")
	}

	// 验证已删除
	if limiter.VerifyCaptcha(capc.Id, capc.Answer) {
		t.Error("验证码应该已删除")
	}

	limiter.RemoveAttempts(ip)
	// 验证后状态
	if banned, need := limiter.CheckSecurityStatus(ip); banned || need {
		t.Error("验证成功后应该重置状态")
	}
}

func TestCaptchaMustFlow(t *testing.T) {
	policy := SecurityPolicy{CaptchaThreshold: 0}
	limiter := NewLoginLimiter(policy)
	limiter.RegisterProvider(&MockCaptchaProvider{})
	ip := "10.0.0.1"

	// 检查状态
	if _, need := limiter.CheckSecurityStatus(ip); !need {
		t.Error("应该需要验证码")
	}

	// 生成验证码
	err, capc := limiter.RequireCaptcha()
	if err != nil {
		t.Fatalf("生成验证码失败: %v", err)
	}
	fmt.Printf("验证码内容: %#v\n", capc)

	// 验证成功
	if !limiter.VerifyCaptcha(capc.Id, capc.Answer) {
		t.Error("验证码应该验证成功")
	}

	// 验证后状态
	if _, need := limiter.CheckSecurityStatus(ip); !need {
		t.Error("应该需要验证码")
	}
}
func TestAttemptTimeout(t *testing.T) {
	policy := SecurityPolicy{CaptchaThreshold: 2, AttemptsWindow: 1 * time.Second}
	limiter := NewLoginLimiter(policy)
	limiter.RegisterProvider(&MockCaptchaProvider{})
	ip := "10.0.0.1"

	// 触发验证码要求
	limiter.RecordFailedAttempt(ip)
	limiter.RecordFailedAttempt(ip)

	// 检查状态
	if _, need := limiter.CheckSecurityStatus(ip); !need {
		t.Error("应该需要验证码")
	}

	// 生成验证码
	err, _ := limiter.RequireCaptcha()
	if err != nil {
		t.Fatalf("生成验证码失败: %v", err)
	}
	// 等待超过 AttemptsWindow
	time.Sleep(2 * time.Second)
	// 触发验证码要求
	limiter.RecordFailedAttempt(ip)

	// 检查状态
	if _, need := limiter.CheckSecurityStatus(ip); need {
		t.Error("不应该需要验证码")
	}
}

func TestCaptchaTimeout(t *testing.T) {
	policy := SecurityPolicy{CaptchaThreshold: 2}
	limiter := NewLoginLimiter(policy)
	limiter.RegisterProvider(&MockCaptchaProvider{})
	ip := "10.0.0.1"

	// 触发验证码要求
	limiter.RecordFailedAttempt(ip)
	limiter.RecordFailedAttempt(ip)

	// 检查状态
	if _, need := limiter.CheckSecurityStatus(ip); !need {
		t.Error("应该需要验证码")
	}

	// 生成验证码
	err, capc := limiter.RequireCaptcha()
	if err != nil {
		t.Fatalf("生成验证码失败: %v", err)
	}

	// 等待超过 CaptchaValidPeriod
	time.Sleep(3 * time.Second)

	// 验证成功
	if limiter.VerifyCaptcha(capc.Id, capc.Answer) {
		t.Error("验证码应该已过期")
	}

}

func TestBanFlow(t *testing.T) {
	policy := SecurityPolicy{BanThreshold: 5}
	limiter := NewLoginLimiter(policy)
	ip := "10.0.0.1"
	// 触发ban
	for i := 0; i < 5; i++ {
		limiter.RecordFailedAttempt(ip)
	}

	// 检查状态
	if banned, _ := limiter.CheckSecurityStatus(ip); !banned {
		t.Error("should be banned")
	}
}
func TestBanDisableFlow(t *testing.T) {
	policy := SecurityPolicy{BanThreshold: 0}
	limiter := NewLoginLimiter(policy)
	ip := "10.0.0.1"
	// 触发ban
	for i := 0; i < 5; i++ {
		limiter.RecordFailedAttempt(ip)
	}

	// 检查状态
	if banned, _ := limiter.CheckSecurityStatus(ip); banned {
		t.Error("should not be banned")
	}
}
func TestBanTimeout(t *testing.T) {
	policy := SecurityPolicy{BanThreshold: 5, BanDuration: 1 * time.Second}
	limiter := NewLoginLimiter(policy)
	ip := "10.0.0.1"
	// 触发ban
	// 触发ban
	for i := 0; i < 5; i++ {
		limiter.RecordFailedAttempt(ip)
	}

	time.Sleep(2 * time.Second)

	// 检查状态
	if banned, _ := limiter.CheckSecurityStatus(ip); banned {
		t.Error("should not be banned")
	}
}

func TestLimiterDisabled(t *testing.T) {
	policy := SecurityPolicy{BanThreshold: 0, CaptchaThreshold: -1}
	limiter := NewLoginLimiter(policy)
	ip := "10.0.0.1"
	// 触发ban
	for i := 0; i < 5; i++ {
		limiter.RecordFailedAttempt(ip)
	}

	// 检查状态
	if banned, capNeed := limiter.CheckSecurityStatus(ip); banned || capNeed {
		fmt.Printf("IP: %s, Banned: %v, Captcha Required: %v\n", ip, banned, capNeed)
		t.Error("should not be banned or need captcha")
	}
}

func TestB64CaptchaFlow(t *testing.T) {
	limiter := NewLoginLimiter(defaultSecurityPolicy)
	limiter.RegisterProvider(B64StringCaptchaProvider{})
	ip := "10.0.0.1"

	// 触发验证码要求
	limiter.RecordFailedAttempt(ip)
	limiter.RecordFailedAttempt(ip)
	limiter.RecordFailedAttempt(ip)

	// 检查状态
	if _, need := limiter.CheckSecurityStatus(ip); !need {
		t.Error("应该需要验证码")
	}

	// 生成验证码
	err, capc := limiter.RequireCaptcha()
	if err != nil {
		t.Fatalf("生成验证码失败: %v", err)
	}
	fmt.Printf("验证码内容: %#v\n", capc)

	//draw
	err, b64 := limiter.DrawCaptcha(capc.Content)
	if err != nil {
		t.Fatalf("绘制验证码失败: %v", err)
	}
	fmt.Printf("验证码内容: %#v\n", b64)

	// 验证成功
	if !limiter.VerifyCaptcha(capc.Id, capc.Answer) {
		t.Error("验证码应该验证成功")
	}
	limiter.RemoveAttempts(ip)
	// 验证后状态
	if banned, need := limiter.CheckSecurityStatus(ip); banned || need {
		t.Error("验证成功后应该重置状态")
	}
}

func TestAccountFailureBlocksAtThreshold(t *testing.T) {
	policy := SecurityPolicy{
		CaptchaThreshold:     -1,
		BanThreshold:         0,
		AttemptsWindow:       time.Hour,
		AccountFailThreshold: 3,
		AccountBanDuration:   time.Hour,
	}
	limiter := NewLoginLimiter(policy)
	ip, user := "192.168.1.100", "alice"

	// 阈值内失败：不阻断
	limiter.RecordAccountFailure(ip, user)
	limiter.RecordAccountFailure(ip, user)
	if blocked, _ := limiter.CheckAccountBlock(ip, user); blocked {
		t.Error("threshold not reached, should not be blocked")
	}

	// 达到阈值：阻断
	limiter.RecordAccountFailure(ip, user)
	blocked, until := limiter.CheckAccountBlock(ip, user)
	if !blocked {
		t.Fatal("threshold reached, should be blocked")
	}
	if until.Before(time.Now().Add(time.Hour - time.Minute)) {
		t.Errorf("unexpected block duration until %v", until)
	}

	// 阻断期间重复失败不延长阻断
	before := until
	limiter.RecordAccountFailure(ip, user)
	if blocked, after := limiter.CheckAccountBlock(ip, user); !blocked || after != before {
		t.Error("block during an active block should not be extended")
	}

	// 其它 (IP, 用户名) 组合不受影响
	if blocked, _ := limiter.CheckAccountBlock("192.168.1.100", "bob"); blocked {
		t.Error("other user should not be blocked")
	}
	if blocked, _ := limiter.CheckAccountBlock("10.0.0.1", user); blocked {
		t.Error("other ip should not be blocked")
	}
}

func TestAccountFailureEscalatesUntilSuccess(t *testing.T) {
	policy := SecurityPolicy{
		CaptchaThreshold:     -1,
		BanThreshold:         0,
		AttemptsWindow:       time.Hour,
		AccountFailThreshold: 2,
		AccountBanDuration:   200 * time.Millisecond,
	}
	limiter := NewLoginLimiter(policy)
	ip, user := "192.168.1.100", "alice"

	// 第 1 次阻断：基础时长
	for i := 0; i < 2; i++ {
		limiter.RecordAccountFailure(ip, user)
	}
	blocked, until1 := limiter.CheckAccountBlock(ip, user)
	if !blocked {
		t.Fatal("first block expected")
	}
	firstDur := until1.Sub(time.Now())
	time.Sleep(300 * time.Millisecond)

	// 第 2 次阻断：时长翻倍（指数退避）
	for i := 0; i < 2; i++ {
		limiter.RecordAccountFailure(ip, user)
	}
	blocked, until2 := limiter.CheckAccountBlock(ip, user)
	if !blocked {
		t.Fatal("second block expected")
	}
	secondDur := until2.Sub(time.Now())
	if secondDur <= firstDur {
		t.Errorf("expected escalating ban duration, first=%v second=%v", firstDur, secondDur)
	}

	// 登录成功后计数与退避一并重置
	limiter.ClearAccountFailures(ip, user)
	time.Sleep(300 * time.Millisecond)
	for i := 0; i < 2; i++ {
		limiter.RecordAccountFailure(ip, user)
	}
	blocked, until3 := limiter.CheckAccountBlock(ip, user)
	if !blocked {
		t.Fatal("block expected after re-reaching threshold")
	}
	if until3.After(time.Now().Add(300*time.Millisecond + 50*time.Millisecond)) {
		t.Errorf("ban duration should reset to base after success, got %v", until3.Sub(time.Now()))
	}
}

func TestAccountFailureDisabled(t *testing.T) {
	policy := SecurityPolicy{
		CaptchaThreshold:     -1,
		BanThreshold:         0,
		AccountFailThreshold: -1,
	}
	limiter := NewLoginLimiter(policy)
	ip, user := "192.168.1.100", "alice"

	for i := 0; i < 10; i++ {
		limiter.RecordAccountFailure(ip, user)
	}
	if blocked, _ := limiter.CheckAccountBlock(ip, user); blocked {
		t.Error("account limiting disabled, should not be blocked")
	}
}

func TestAccountFailureDefaultsEnabled(t *testing.T) {
	// 缺省策略（0 值）下账号级保护应启用，保证默认配置有爆破防护
	limiter := NewLoginLimiter(SecurityPolicy{})
	ip, user := "192.168.1.100", "alice"

	for i := 0; i < AccountBanDefaultThreshold; i++ {
		limiter.RecordAccountFailure(ip, user)
	}
	if blocked, _ := limiter.CheckAccountBlock(ip, user); !blocked {
		t.Error("default policy should block after default threshold")
	}
}

// TestAccountStreakSurvivesCleanup 审查 P2：阻断过期后后台清理不得把
// streak（指数退避计数）一并丢弃，否则 5m→10m→20m→60m 永远停在基础时长。
// 本测试精确复现该场景：第 1 次阻断 → 阻断过期 → cleanupExpired() →
// 第 2 次阻断必须按 2 倍基础时长升级。
func TestAccountStreakSurvivesCleanup(t *testing.T) {
	policy := SecurityPolicy{
		CaptchaThreshold:     -1,
		BanThreshold:         0,
		AttemptsWindow:       time.Hour,
		AccountFailThreshold: 2,
		AccountBanDuration:   150 * time.Millisecond,
	}
	limiter := NewLoginLimiter(policy)
	ip, user := "192.168.1.200", "bob"

	// 第 1 次阻断（基础时长），streak 变为 1
	for i := 0; i < 2; i++ {
		limiter.RecordAccountFailure(ip, user)
	}
	if blocked, _ := limiter.CheckAccountBlock(ip, user); !blocked {
		t.Fatal("first block expected")
	}
	time.Sleep(220 * time.Millisecond) // 阻断过期
	limiter.cleanupExpired() // 旧实现在这里删除 accountState，连带丢弃 streak
	limiter.mu.Lock()
	st := limiter.accountStates[accountKey(ip, user)]
	streak := 0
	if st != nil {
		streak = st.streak
	}
	limiter.mu.Unlock()
	if streak != 1 {
		t.Fatalf("streak must survive cleanup, got %d", streak)
	}

	// 第 2 次阻断：必须升级为 2 倍基础时长
	for i := 0; i < 2; i++ {
		limiter.RecordAccountFailure(ip, user)
	}
	blocked, until := limiter.CheckAccountBlock(ip, user)
	if !blocked {
		t.Fatal("second block expected")
	}
	dur := until.Sub(time.Now())
	if dur <= policy.AccountBanDuration+50*time.Millisecond {
		t.Errorf("second ban should be escalated (streak preserved), got %v (base=%v)", dur, policy.AccountBanDuration)
	}
}

// TestAccountStreakExpiresAfterTTL streak 超过 accountStateTTL 无任何活动后
// 过期清除，防止 accountStates 无界累积
func TestAccountStreakExpiresAfterTTL(t *testing.T) {
	policy := SecurityPolicy{
		CaptchaThreshold:     -1,
		BanThreshold:         0,
		AttemptsWindow:       time.Hour,
		AccountFailThreshold: 2,
		AccountBanDuration:   100 * time.Millisecond,
	}
	limiter := NewLoginLimiter(policy)
	ip, user := "192.168.1.201", "carol"
	for i := 0; i < 2; i++ {
		limiter.RecordAccountFailure(ip, user)
	}
	time.Sleep(150 * time.Millisecond) // 阻断过期
	key := accountKey(ip, user)
	limiter.mu.Lock()
	if st := limiter.accountStates[key]; st != nil {
		st.lastActive = time.Now().Add(-accountStateTTL - time.Hour)
	}
	limiter.mu.Unlock()
	limiter.cleanupExpired()
	limiter.mu.Lock()
	_, exists := limiter.accountStates[key]
	limiter.mu.Unlock()
	if exists {
		t.Error("streak state should be removed after TTL of inactivity")
	}
}
