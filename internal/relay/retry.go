package relay

import (
	"errors"
	"net"
	"net/http"
)

type attemptDecision uint8

const (
	attemptStopRequest attemptDecision = iota
	attemptNextChannel
	attemptNextKey
)

// failureDecision 将一次上游失败归类到最小的重试范围：
// key 凭据/限流/服务端故障可以换 key；模型在该渠道不存在时换渠道；
// 请求参数或本地处理错误不应重复发送到同一渠道的所有 key。
func (ra *relayAttempt) failureDecision() attemptDecision {
	if ra.upstreamNetworkError || ra.streamFailureRetryable {
		return attemptNextKey
	}
	if ra.upstreamStatusCode == 0 {
		// 没有 HTTP 状态通常表示当前渠道的请求构造/转换/空响应问题；
		// 不要对同一渠道的所有 key 重放，但允许其他渠道接管。
		return attemptNextChannel
	}

	switch ra.upstreamStatusCode {
	case http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusRequestTimeout,
		http.StatusTooManyRequests:
		return attemptNextKey
	case http.StatusNotFound:
		return attemptNextChannel
	}

	if ra.upstreamStatusCode >= http.StatusInternalServerError {
		return attemptNextKey
	}
	return attemptStopRequest
}

// isNetworkFailure 保留网络层失败的 key 兜底能力。http.Client 会把这类错误
// 包成 *url.Error，而读取响应时的超时/连接错误通常实现 net.Error；两者都能
// 被 errors.As 穿透 pipeline 的包装层。
func isNetworkFailure(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
