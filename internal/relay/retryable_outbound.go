package relay

import (
	"context"
	"errors"
	"time"

	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/transformer"
)

const (
	// Empty upstream streams are safe to replay because the pipeline buffers the
	// response until the empty-response check has passed.
	relayMaxSameChannelRetries = 2
	relayRetryDelay            = time.Second
)

// sameChannelRetryOutbound adds the pipeline's same-channel retry contract to
// an otherwise stateless channel transformer. The relay creates a fresh wrapper
// for every channel/key attempt, so no transformer state needs to be reset.
type sameChannelRetryOutbound struct {
	transformer.Outbound
}

var _ pipeline.ChannelRetryable = (*sameChannelRetryOutbound)(nil)

func (o *sameChannelRetryOutbound) CanRetry(err error) bool {
	return errors.Is(err, pipeline.ErrEmptyResponse) ||
		errors.Is(err, pipeline.ErrEmptyStreamChunks) ||
		errors.Is(err, pipeline.ErrEmptyAggregatedBody)
}

func (o *sameChannelRetryOutbound) PrepareForRetry(context.Context) error {
	return nil
}
