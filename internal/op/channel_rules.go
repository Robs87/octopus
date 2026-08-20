package op

import (
	"context"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// IsInsufficientBalance 判断上游失败是否为余额不足/配额耗尽。
// 常见的表现形式：HTTP 402，或错误响应体含 insufficient_quota / insufficient balance / 余额不足 等。
// 仅对失败状态码（>= 400）生效。转发链路与渠道测试共用。
func IsInsufficientBalance(statusCode int, body []byte) bool {
	if statusCode < http.StatusBadRequest { // 非失败状态不判定
		return false
	}
	if statusCode == http.StatusPaymentRequired { // 402
		return true
	}
	if len(body) == 0 {
		return false
	}
	text := strings.ToLower(string(body))
	for _, kw := range []string{"insufficient_quota", "insufficient quota", "insufficient balance", "余额不足", "exceeded your current quota"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// AllChannelKeysDisabled 判断渠道的所有 key 是否都已停用。
func AllChannelKeysDisabled(ch *model.Channel) bool {
	if ch == nil || len(ch.Keys) == 0 {
		return true
	}
	for _, k := range ch.Keys {
		if k.Enabled {
			return false
		}
	}
	return true
}

// DisableInsufficientKey 在余额不足时停用渠道 key；若渠道所有 key 均已停用，则停用渠道。
// 供转发链路与渠道测试共用。
func DisableInsufficientKey(channel *model.Channel, usedKey model.ChannelKey, statusCode int, body []byte) {
	if usedKey.ID == 0 || !IsInsufficientBalance(statusCode, body) {
		return
	}
	log.Warnf("channel %s key %q reported insufficient balance (status=%d), auto disabling key %d",
		channel.Name, usedKey.DisplayName(), statusCode, usedKey.ID)
	if err := ChannelKeySetEnabled(usedKey.ID, usedKey.ChannelID, false, context.Background()); err != nil {
		log.Warnf("failed to auto disable channel key %d: %v", usedKey.ID, err)
		return
	}
	// 重新读取渠道最新状态，判断是否所有 key 都已关闭。
	if ch, err := ChannelGet(channel.ID, context.Background()); err == nil && AllChannelKeysDisabled(ch) {
		log.Warnf("channel %s has no enabled keys, auto disabling channel %d", ch.Name, ch.ID)
		if err := ChannelEnabled(ch.ID, false, context.Background()); err != nil {
			log.Warnf("failed to auto disable channel %d: %v", ch.ID, err)
		}
	}
}
