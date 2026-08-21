package handlers

import (
	"context"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
)

func TestBuildModelTestRequestPreservesSelectedKey(t *testing.T) {
	req, _, err := buildModelTestRequest(context.Background(), &model.Channel{
		Type: llm.APIFormatOpenAIChatCompletion,
		CustomHeader: []model.CustomHeader{{
			HeaderKey:   "Authorization",
			HeaderValue: "Bearer wrong-fixed-header",
		}},
	}, "https://example.com", "selected-key", "model")
	if err != nil {
		t.Fatalf("buildModelTestRequest() error = %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer selected-key" {
		t.Fatalf("Authorization = %q, want selected key", got)
	}
}

func TestRunChannelKeyTestCancellationStopsBeforeKeyStateUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := runChannelKeyTest(ctx, &model.Channel{
		ID:   1,
		Name: "cancelled-channel",
		Type: llm.APIFormatOpenAIChatCompletion,
	}, model.ChannelKey{ID: 1, ChannelID: 1, Enabled: true, ChannelKey: "selected-key"}, "model", model.LogTypeTest, "")
	if result.Success {
		t.Fatal("runChannelKeyTest() succeeded after cancellation")
	}
	if result.Message != context.Canceled.Error() {
		t.Fatalf("message = %q, want %q", result.Message, context.Canceled.Error())
	}
}
