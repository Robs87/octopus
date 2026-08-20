package model

import (
	"testing"
	"time"
)

func mkKey(id int, enabled bool, key string, quota, totalCost float64, statusCode int, lastUse int64) ChannelKey {
	return ChannelKey{
		ID:               id,
		ChannelID:        1,
		Enabled:          enabled,
		ChannelKey:       key,
		Quota:            quota,
		TotalCost:        totalCost,
		StatusCode:       statusCode,
		LastUseTimeStamp: lastUse,
	}
}

func nowSec() int64 { return time.Now().Unix() }

// 规则：多个 key 都正常时，优先使用最近成功过的 key，不做轮询
func TestGetChannelKey_PreferMostRecentSuccess(t *testing.T) {
	now := nowSec()
	c := &Channel{Keys: []ChannelKey{
		mkKey(1, true, "k1", 100, 0, 200, now-60), // 成功过，但较旧
		mkKey(2, true, "k2", 10, 0, 200, now-10),  // 最近成功，应被选中
		mkKey(3, true, "k3", 50, 0, 200, now-30),
	}}
	got := c.GetChannelKey()
	if got.ID != 2 {
		t.Fatalf("expected key 2 (most recent success), got %d", got.ID)
	}
}

// GetChannelKeys 返回按“最近成功优先”排序的多个可用 key，供同渠道同模型故障时逐个换 key 重试
func TestGetChannelKeys_OrderedAvailableKeys(t *testing.T) {
	now := nowSec()
	c := &Channel{Keys: []ChannelKey{
		mkKey(1, true, "k1", 100, 0, 200, now-10),
		mkKey(2, false, "k2", 100, 0, 200, now-5), // 禁用，应被过滤
		mkKey(3, true, "k3", 100, 0, 200, now-60), // 成功过，但最旧
		mkKey(4, true, "k4", 100, 0, 429, now-1),  // 冷却中，应被过滤
		mkKey(5, true, "k5", 100, 0, 200, now-30),
	}}
	got := c.GetChannelKeys()
	if len(got) != 3 {
		t.Fatalf("expected 3 available keys, got %d", len(got))
	}
	wantIDs := []int{1, 5, 3}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Fatalf("expected key order %v at index %d, got %d", wantIDs, i, got[i].ID)
		}
	}
}

// 没有成功记录时，按 key ID 升序选择，保持稳定
func TestGetChannelKey_NoSuccessPrefersFirstID(t *testing.T) {
	c := &Channel{Keys: []ChannelKey{
		mkKey(2, true, "k2", 0, 0, 0, 0),
		mkKey(1, true, "k1", 0, 0, 0, 0),
		mkKey(3, true, "k3", 0, 0, 0, 0),
	}}
	got := c.GetChannelKey()
	if got.ID != 1 {
		t.Fatalf("expected key 1 (lowest ID with no success record), got %d", got.ID)
	}
}

// 故障 key 冷却结束后，只要还有健康成功 key，就继续用健康 key，而不是回去轮询故障 key
func TestGetChannelKey_HealthySuccessPreferredOverExpiredFailure(t *testing.T) {
	now := nowSec()
	c := &Channel{Keys: []ChannelKey{
		mkKey(1, true, "k1", 20, 11, 200, now-10),  // 最近成功，应被选中
		mkKey(2, true, "k2", 68, 1, 400, now-3600), // 1 小时前失败，冷却已过但不优先
	}}
	got := c.GetChannelKey()
	if got.ID != 1 {
		t.Fatalf("expected key 1 (healthy success), got %d", got.ID)
	}
}

// 规则：故障冷却（4xx/5xx/网络错误）5 分钟内跳过
func TestGetChannelKey_Cooldown(t *testing.T) {
	now := nowSec()
	c := &Channel{Keys: []ChannelKey{
		mkKey(1, true, "k1", 0, 0, 429, now-10), // 429 冷却中
		mkKey(2, true, "k2", 0, 0, 500, now-20), // 5xx 冷却中
		mkKey(3, true, "k3", 0, 0, 0, now-30),   // 网络错误冷却中
		mkKey(4, true, "k4", 0, 0, 200, now-1),  // 正常
	}}
	got := c.GetChannelKey()
	if got.ID != 4 {
		t.Fatalf("expected key 4 (only healthy), got %d", got.ID)
	}
}

// 冷却过期后，若没有其他成功 key，失败过的 key 也可用；有成功 key 时仍优先成功 key
func TestGetChannelKey_CooldownExpiredFallback(t *testing.T) {
	now := nowSec()
	c := &Channel{Keys: []ChannelKey{
		mkKey(1, true, "k1", 0, 0, 429, now-400), // 冷却已过，但不是成功状态
		mkKey(2, true, "k2", 0, 0, 200, now-1),   // 最近成功，应被选中
	}}
	got := c.GetChannelKey()
	if got.ID != 2 {
		t.Fatalf("expected key 2 (recent success), got %d", got.ID)
	}
}

// 余额不足已停用的 key 被跳过
func TestGetChannelKey_DisabledKeySkipped(t *testing.T) {
	now := nowSec()
	c := &Channel{Keys: []ChannelKey{
		mkKey(1, false, "k1", 10, 9, 200, now-1), // 已停用
		mkKey(2, true, "k2", 100, 0, 200, now-1),
	}}
	got := c.GetChannelKey()
	if got.ID != 2 {
		t.Fatalf("expected key 2, got %d", got.ID)
	}
}

// 剩余额度耗尽的 key 视为不可用
func TestGetChannelKey_QuotaExhausted(t *testing.T) {
	now := nowSec()
	c := &Channel{Keys: []ChannelKey{
		mkKey(1, true, "k1", 10, 10, 200, now-1), // 额度已耗尽
		mkKey(2, true, "k2", 100, 0, 200, now-1),
	}}
	got := c.GetChannelKey()
	if got.ID != 2 {
		t.Fatalf("expected key 2, got %d", got.ID)
	}
}

// 全部不可用时返回空 key
func TestGetChannelKey_NoAvailable(t *testing.T) {
	now := nowSec()
	c := &Channel{Keys: []ChannelKey{
		mkKey(1, false, "k1", 10, 0, 200, now-1),
		mkKey(2, true, "k2", 0, 0, 429, now-10),
	}}
	got := c.GetChannelKey()
	if got.ChannelKey != "" {
		t.Fatalf("expected empty key, got %q", got.ChannelKey)
	}
}
