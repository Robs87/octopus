package relay

import (
	"net/http"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestRelayAttemptFailureDecision(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		network    bool
		stream     bool
		want       attemptDecision
	}{
		{name: "bad request stops", statusCode: http.StatusBadRequest, want: attemptStopRequest},
		{name: "unprocessable entity stops", statusCode: http.StatusUnprocessableEntity, want: attemptStopRequest},
		{name: "not found moves channel", statusCode: http.StatusNotFound, want: attemptNextChannel},
		{name: "unauthorized moves key", statusCode: http.StatusUnauthorized, want: attemptNextKey},
		{name: "payment required moves key", statusCode: http.StatusPaymentRequired, want: attemptNextKey},
		{name: "forbidden moves key", statusCode: http.StatusForbidden, want: attemptNextKey},
		{name: "request timeout moves key", statusCode: http.StatusRequestTimeout, want: attemptNextKey},
		{name: "rate limit moves key", statusCode: http.StatusTooManyRequests, want: attemptNextKey},
		{name: "server error moves key", statusCode: http.StatusBadGateway, want: attemptNextKey},
		{name: "local pipeline error moves channel", want: attemptNextChannel},
		{name: "network error moves key", network: true, want: attemptNextKey},
		{name: "stream error moves key", stream: true, want: attemptNextKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempt := &relayAttempt{
				upstreamStatusCode:     tt.statusCode,
				upstreamNetworkError:   tt.network,
				streamFailureRetryable: tt.stream,
			}
			if got := attempt.failureDecision(); got != tt.want {
				t.Fatalf("failureDecision() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrioritizeChannelKey(t *testing.T) {
	keys := []model.ChannelKey{{ID: 1}, {ID: 2}, {ID: 3}}
	got := prioritizeChannelKey(keys, 3)
	for i, want := range []int{3, 1, 2} {
		if got[i].ID != want {
			t.Fatalf("key order = %v, want [3 1 2]", []int{got[0].ID, got[1].ID, got[2].ID})
		}
	}
}
