package op

import (
	"net/http"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestIsInsufficientBalance(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{"402 always true", http.StatusPaymentRequired, "{}", true},
		{"openai insufficient_quota", http.StatusTooManyRequests, `{"error":{"code":"insufficient_quota"}}`, true},
		{"anthropic insufficient_quota", http.StatusForbidden, `{"type":"error","error":{"type":"insufficient_quota"}}`, true},
		{"insufficient balance", http.StatusBadRequest, `{"error":{"message":"Insufficient Balance"}}`, true},
		{"chinese balance", http.StatusBadRequest, `{"message":"余额不足"}`, true},
		{"exceeded current quota", http.StatusTooManyRequests, `{"error":{"message":"You exceeded your current quota"}}`, true},
		{"rate limit not balance", http.StatusTooManyRequests, `{"error":{"code":"rate_limit_exceeded"}}`, false},
		{"empty body not balance", http.StatusBadRequest, "", false},
		{"ok status not balance", http.StatusOK, `{"error":{"code":"insufficient_quota"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsInsufficientBalance(tc.statusCode, []byte(tc.body))
			if got != tc.want {
				t.Fatalf("IsInsufficientBalance(%d, %q) = %v, want %v", tc.statusCode, tc.body, got, tc.want)
			}
		})
	}
}

func TestAllChannelKeysDisabled(t *testing.T) {
	mk := func(enabled bool) model.ChannelKey {
		return model.ChannelKey{Enabled: enabled}
	}
	if !AllChannelKeysDisabled(nil) {
		t.Fatal("nil channel should be considered all disabled")
	}
	if !AllChannelKeysDisabled(&model.Channel{}) {
		t.Fatal("empty keys should be considered all disabled")
	}
	if AllChannelKeysDisabled(&model.Channel{Keys: []model.ChannelKey{mk(true), mk(false)}}) {
		t.Fatal("channel with one enabled key should NOT be all disabled")
	}
	if !AllChannelKeysDisabled(&model.Channel{Keys: []model.ChannelKey{mk(false), mk(false)}}) {
		t.Fatal("channel with all disabled keys should be all disabled")
	}
}
