package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/dlclark/regexp2"
	"github.com/looplj/axonhub/llm"
	llmhttpclient "github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/transformer"
)

func FetchModels(ctx context.Context, request model.Channel) ([]string, error) {
	keys := request.GetChannelKeys()
	if len(keys) == 0 {
		return nil, errors.New("channel has no available key")
	}

	client, err := ChannelHttpClient(&request)
	if err != nil {
		return nil, err
	}

	var fetchModel []string
	var lastErr error
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		attempt := request
		// 每次请求只携带当前 key，确保模型同步和实际中继一样按 key
		// 隔离认证失败；不能让 fetch 函数再次从原渠道随机取回第一个 key。
		attempt.Keys = []model.ChannelKey{key}

		fetchModel, err = fetchModelsWithKey(client, ctx, attempt)
		if err == nil {
			lastErr = nil
			break
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = fmt.Errorf("channel key %d: %w", key.ID, err)
	}
	if lastErr != nil {
		return nil, lastErr
	}

	if request.MatchRegex != nil && *request.MatchRegex != "" {
		matchModel := make([]string, 0)
		re, err := regexp2.Compile(*request.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			return nil, err
		}
		for _, model := range fetchModel {
			matched, err := re.MatchString(model)
			if err != nil {
				return nil, err
			}
			if matched {
				matchModel = append(matchModel, model)
			}
		}
		return matchModel, nil
	}
	return fetchModel, nil
}

func fetchModelsWithKey(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {
	switch request.Type {
	case llm.APIFormatAnthropicMessage:
		return fetchAnthropicModels(client, ctx, request)
	case llm.APIFormatGeminiContents:
		return fetchGeminiModels(client, ctx, request)
	default:
		return fetchOpenAIModels(client, ctx, request)
	}
}

func channelRequestKey(request model.Channel) string {
	if len(request.Keys) > 0 {
		return request.Keys[0].ChannelKey
	}
	return request.GetChannelKey().ChannelKey
}

func ensureModelListResponse(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if readErr != nil {
		return fmt.Errorf("model list request failed with status %s: %w", resp.Status, readErr)
	}
	if detail := strings.TrimSpace(string(body)); detail != "" {
		return fmt.Errorf("model list request failed with status %s: %s", resp.Status, detail)
	}
	return fmt.Errorf("model list request failed with status %s", resp.Status)
}

// refer: https://platform.openai.com/docs/api-reference/models/list
func fetchOpenAIModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {
	baseURL := transformer.NormalizeBaseURL(request.GetBaseUrl(), "v1")
	if request.Type == model.ChannelTypeDoubao {
		baseURL = transformer.NormalizeBaseURL(request.GetBaseUrl(), "v3")
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/models",
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+channelRequestKey(request))
	applyCustomHeaders(req, request)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := ensureModelListResponse(resp); err != nil {
		return nil, err
	}

	var result model.OpenAIModelList

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

// refer: https://ai.google.dev/api/models
func fetchGeminiModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {
	var allModels []string
	pageToken := ""
	baseURL := transformer.NormalizeBaseURL(request.GetBaseUrl(), "v1beta")
	// Gemini transformer 会保留用户显式填写的 /v1；这里同样处理，避免把 /v1 拼成 /v1/v1beta。
	if strings.HasSuffix(strings.TrimRight(request.GetBaseUrl(), "/"), "/v1") {
		baseURL = transformer.NormalizeBaseURL(request.GetBaseUrl(), "")
	}

	for {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			baseURL+"/models",
			nil,
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Goog-Api-Key", channelRequestKey(request))
		applyCustomHeaders(req, request)
		if pageToken != "" {
			q := req.URL.Query()
			q.Add("pageToken", pageToken)
			req.URL.RawQuery = q.Encode()
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if err := ensureModelListResponse(resp); err != nil {
			resp.Body.Close()
			return nil, err
		}

		var result model.GeminiModelList

		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}

		for _, m := range result.Models {
			name := strings.TrimPrefix(m.Name, "models/")
			allModels = append(allModels, name)
		}

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}
	if len(allModels) == 0 {
		return fetchOpenAIModels(client, ctx, request)
	}
	return allModels, nil
}

// refer: https://platform.claude.com/docs
func fetchAnthropicModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {

	var allModels []string
	var afterID string
	baseURL := transformer.NormalizeBaseURL(request.GetBaseUrl(), "v1")
	for {

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			baseURL+"/models",
			nil,
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Api-Key", channelRequestKey(request))
		req.Header.Set("Anthropic-Version", "2023-06-01")
		applyCustomHeaders(req, request)
		// 设置多页参数
		q := req.URL.Query()

		if afterID != "" {
			q.Set("after_id", afterID)
		}
		req.URL.RawQuery = q.Encode()

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if err := ensureModelListResponse(resp); err != nil {
			resp.Body.Close()
			return nil, err
		}

		var result model.AnthropicModelList

		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}

		for _, m := range result.Data {
			allModels = append(allModels, m.ID)
		}

		if !result.HasMore {
			break
		}

		afterID = result.LastID
	}
	if len(allModels) == 0 {
		return fetchOpenAIModels(client, ctx, request)
	}
	return allModels, nil
}

func applyCustomHeaders(req *http.Request, channel model.Channel) {
	for _, header := range channel.CustomHeader {
		if header.HeaderKey != "" {
			// 渠道配置中的自定义头不能覆盖本次选中的 key；否则一个固定
			// Authorization/X-Api-Key 会让多 key 兜底实际仍使用错误凭据。
			if req.Header.Get(header.HeaderKey) != "" && llmhttpclient.IsSensitiveHeader(header.HeaderKey) {
				continue
			}
			req.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}
}
