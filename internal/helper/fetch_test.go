package helper

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
)

func TestFetchModelsFallsBackToNextChannelKey(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.Header.Get("Authorization") {
		case "Bearer key-1":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"invalid key"}}`)
		case "Bearer key-2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"model-from-key-2"}]}`)
		default:
			http.Error(w, "unexpected authorization", http.StatusForbidden)
		}
	}))
	defer server.Close()

	models, err := FetchModels(context.Background(), model.Channel{
		ID:   1,
		Type: llm.APIFormatOpenAIChatCompletion,
		BaseUrls: []model.BaseUrl{{
			URL: server.URL,
		}},
		Keys: []model.ChannelKey{
			{ID: 1, ChannelID: 1, Enabled: true, ChannelKey: "key-1"},
			{ID: 2, ChannelID: 1, Enabled: true, ChannelKey: "key-2"},
		},
		CustomHeader: []model.CustomHeader{{
			HeaderKey:   "Authorization",
			HeaderValue: "Bearer wrong-fixed-header",
		}},
	})
	if err != nil {
		t.Fatalf("FetchModels() error = %v", err)
	}
	if len(models) != 1 || models[0] != "model-from-key-2" {
		t.Fatalf("FetchModels() = %v, want [model-from-key-2]", models)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("server calls = %d, want 2", got)
	}
}

func TestFetchModelsRequiresAnAvailableKey(t *testing.T) {
	_, err := FetchModels(context.Background(), model.Channel{
		ID:   1,
		Type: llm.APIFormatOpenAIChatCompletion,
		Keys: []model.ChannelKey{{ID: 1, ChannelID: 1, Enabled: false, ChannelKey: "disabled"}},
	})
	if err == nil {
		t.Fatal("FetchModels() error = nil, want unavailable-key error")
	}
}
