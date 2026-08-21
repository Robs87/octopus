package relay

import (
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
)

// relayRun 保存一次客户端请求在负载均衡循环中共享的状态。
type relayRun struct {
	c               *gin.Context
	inAdapter       transformer.Inbound
	internalRequest *llm.Request
	metrics         *RelayMetrics
	iter            *balancer.Iterator
	group           dbmodel.Group
}

// relayAttempt 保存一次上游通道尝试的状态。
type relayAttempt struct {
	*relayRun

	outAdapter transformer.Outbound
	channel    *dbmodel.Channel
	usedKey    dbmodel.ChannelKey

	// upstreamErrBody 保存本次尝试上游失败响应的原始响应体，用于余额不足等错误识别。
	upstreamErrBody []byte

	// 下面几个字段只描述本次 attempt，避免把本地转换错误误判成 key 故障，
	// 也让 relayRun 能决定是换 key、换渠道还是直接返回。
	upstreamStatusCode     int
	upstreamNetworkError   bool
	streamFailureRetryable bool
}
