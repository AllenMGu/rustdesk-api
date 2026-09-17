package utils

import (
	"testing"
	"time"
)

func TestRateLimiterEnforcesLimit(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "192.168.1.1"
	// 限额 3 次/分钟
	for i := 0; i < 3; i++ {
		if !rl.Allow(key, 3, time.Minute) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow(key, 3, time.Minute) {
		t.Error("4th request within window should be rejected")
	}
	// 其它 key 不受影响
	if !rl.Allow("10.0.0.1", 3, time.Minute) {
		t.Error("other key should be allowed")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "192.168.1.1"
	if !rl.Allow(key, 1, 100*time.Millisecond) {
		t.Fatal("first request should be allowed")
	}
	if rl.Allow(key, 1, 100*time.Millisecond) {
		t.Fatal("second request within window should be rejected")
	}
	time.Sleep(150 * time.Millisecond)
	if !rl.Allow(key, 1, 100*time.Millisecond) {
		t.Error("request after window expiry should be allowed")
	}
}

func TestRateLimiterUnlimited(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "192.168.1.1"
	for i := 0; i < 10; i++ {
		if !rl.Allow(key, 0, time.Minute) {
			t.Fatal("limit 0 means unlimited")
		}
	}
}
