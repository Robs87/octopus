package relay

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
)

func TestWriteRelayErrorOpenAIShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeRelayError(ctx, llm.APIFormatOpenAIChatCompletion, http.StatusBadGateway, errors.New("empty response"))

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Message != "empty response" || body.Error.Type != "server_error" || body.Error.Code != "502" {
		t.Fatalf("error body = %+v", body.Error)
	}
}

func TestWriteRelayErrorAnthropicShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeRelayError(ctx, llm.APIFormatAnthropicMessage, http.StatusBadGateway, errors.New("empty response"))

	var body struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Type != "error" || body.Error.Type != "api_error" || body.Error.Message != "empty response" {
		t.Fatalf("error body = %+v", body)
	}
}
