package utils

import (
	"sync"
	"time"
)

// RateLimiter 基于滑动窗口的按 key 计数限流器。
// 用于约束匿名遥测端点（/api/sysinfo、/api/audit/* 等）的写入频率，
// 防止未认证的来源持续伪造记录造成存储型 DoS。
type RateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	stop chan struct{}
}

// NewRateLimiter 创建限流器并启动后台清理任务
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		hits: make(map[string][]time.Time),
		stop: make(chan struct{}),
	}
	go rl.cleanupRoutine()
	return rl
}

// Allow 记录 key 的一次访问，并在 window 内不超过 limit 次时返回 true。
// limit <= 0 表示不限制。
func (rl *RateLimiter) Allow(key string, limit int, window time.Duration) bool {
	if rl == nil || limit <= 0 || window <= 0 {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	valid := filterAfter(rl.hits[key], cutoff)
	if len(valid) >= limit {
		rl.hits[key] = valid
		return false
	}
	rl.hits[key] = append(valid, now)
	return true
}

// Stop 停止后台清理任务
func (rl *RateLimiter) Stop() {
	if rl != nil {
		close(rl.stop)
	}
}

func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup(time.Now())
		case <-rl.stop:
			return
		}
	}
}

func (rl *RateLimiter) cleanup(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 清理超过 10 分钟窗口的历史计数，避免 map 无界增长
	cutoff := now.Add(-10 * time.Minute)
	for key, times := range rl.hits {
		valid := filterAfter(times, cutoff)
		if len(valid) == 0 {
			delete(rl.hits, key)
		} else {
			rl.hits[key] = valid
		}
	}
}
