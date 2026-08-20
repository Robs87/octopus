package balancer

import (
	"testing"
	"time"
)

func TestIsTrippedHalfOpenProbeTimeout(t *testing.T) {
	channelID, keyID, modelName := 1, 2, "test-model"
	key := circuitKey(channelID, keyID, modelName)
	entry := getOrCreateEntry(key)

	entry.mu.Lock()
	entry.State = StateHalfOpen
	entry.HalfOpenSince = time.Now().Add(-time.Minute) // 超过 30s，允许重新试探
	entry.mu.Unlock()

	tripped, _ := IsTripped(channelID, keyID, modelName)
	if tripped {
		t.Fatalf("expected HalfOpen probe to be allowed after timeout, but it was tripped")
	}

	entry.mu.Lock()
	entry.State = StateHalfOpen
	entry.HalfOpenSince = time.Now() // 刚进入 HalfOpen，应拒绝其他请求
	entry.mu.Unlock()

	tripped, _ = IsTripped(channelID, keyID, modelName)
	if !tripped {
		t.Fatalf("expected HalfOpen to reject requests before probe timeout, but it was allowed")
	}
}
