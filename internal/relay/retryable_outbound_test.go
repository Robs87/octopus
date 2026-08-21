package relay

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/looplj/axonhub/llm/pipeline"
)

func TestSameChannelRetryOutboundCanRetryEmptyResponses(t *testing.T) {
	outbound := &sameChannelRetryOutbound{}
	retryableErrors := []error{
		pipeline.ErrEmptyResponse,
		pipeline.ErrEmptyStreamChunks,
		pipeline.ErrEmptyAggregatedBody,
		fmt.Errorf("wrapped: %w", pipeline.ErrEmptyResponse),
	}

	for _, err := range retryableErrors {
		if !outbound.CanRetry(err) {
			t.Errorf("CanRetry(%v) = false, want true", err)
		}
	}

	for _, err := range []error{nil, errors.New("unexpected EOF"), errors.New("bad request")} {
		if outbound.CanRetry(err) {
			t.Errorf("CanRetry(%v) = true, want false", err)
		}
	}

	if err := outbound.PrepareForRetry(context.Background()); err != nil {
		t.Fatalf("PrepareForRetry() error = %v", err)
	}
}
