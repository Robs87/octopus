package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/task"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	llmhttpclient "github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/transformer"
)

func init() {
	router.NewGroupRouter("/api/v1/channel").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(listChannel),
		).
		AddRoute(
			router.NewRoute("/create", http.MethodPost).
				Handle(createChannel),
		).
		AddRoute(
			router.NewRoute("/update", http.MethodPost).
				Handle(updateChannel),
		).
		AddRoute(
			router.NewRoute("/enable", http.MethodPost).
				Handle(enableChannel),
		).
		AddRoute(
			router.NewRoute("/delete/:id", http.MethodDelete).
				Handle(deleteChannel),
		).
		AddRoute(
			router.NewRoute("/fetch-model", http.MethodPost).
				Handle(fetchModel),
		).
		AddRoute(
			router.NewRoute("/test-model", http.MethodPost).
				Handle(testModel),
		)
	router.NewGroupRouter("/api/v1/channel").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/sync", http.MethodPost).
				Handle(syncChannel),
		).
		AddRoute(
			router.NewRoute("/last-sync-time", http.MethodGet).
				Handle(getLastSyncTime),
		)
}

func listChannel(c *gin.Context) {
	channels, err := op.ChannelList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	for i, channel := range channels {
		stats := op.StatsChannelGet(channel.ID)
		channels[i].Stats = &stats
	}
	resp.Success(c, channels)
}

func createChannel(c *gin.Context) {
	var channel model.Channel
	if err := c.ShouldBindJSON(&channel); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if err := op.ChannelCreate(&channel, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	stats := op.StatsChannelGet(channel.ID)
	channel.Stats = &stats
	go func(channel *model.Channel) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		modelStr := channel.Model + "," + channel.CustomModel
		modelArray := strings.Split(modelStr, ",")
		helper.LLMPriceAddToDB(modelArray, ctx)
		helper.ChannelBaseUrlDelayUpdate(channel, ctx)
		helper.ChannelAutoGroup(channel, ctx)
	}(&channel)
	resp.Success(c, channel)
}

func updateChannel(c *gin.Context) {
	var req model.ChannelUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	channel, err := op.ChannelUpdate(&req, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	stats := op.StatsChannelGet(channel.ID)
	channel.Stats = &stats
	go func(channel *model.Channel) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		modelStr := channel.Model + "," + channel.CustomModel
		modelArray := strings.Split(modelStr, ",")
		helper.LLMPriceAddToDB(modelArray, ctx)
		helper.ChannelBaseUrlDelayUpdate(channel, ctx)
		helper.ChannelAutoGroup(channel, ctx)
	}(channel)
	resp.Success(c, channel)
}

func enableChannel(c *gin.Context) {
	var request struct {
		ID      int  `json:"id"`
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if err := op.ChannelEnabled(request.ID, request.Enabled, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}

func deleteChannel(c *gin.Context) {
	id := c.Param("id")
	idNum, err := strconv.Atoi(id)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidParam)
		return
	}
	if err := op.ChannelDel(idNum, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}
func fetchModel(c *gin.Context) {
	var request model.Channel
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	models, err := helper.FetchModels(c.Request.Context(), request)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, models)
}

// testModelRequest is the request body for POST /api/v1/channel/test-model.
type testModelRequest struct {
	ChannelID   int    `json:"channel_id" binding:"required"`
	Model       string `json:"model" binding:"required"`
	TestAllKeys bool   `json:"test_all_keys"` // true 时逐个测试渠道下所有启用的 key
}

// testModelKeyResult 单个 key 的测试结果（all-keys 模式）。
type testModelKeyResult struct {
	KeyID      int    `json:"key_id"`
	KeyName    string `json:"key_name"`
	Success    bool   `json:"success"`
	LatencyMS  int64  `json:"latency_ms"`
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message"`
}

// testModelResponse is the result of a channel model connectivity test.
// all-keys 模式下 Total/Passed/Results 有值，Success 表示所有 key 均通过。
type testModelResponse struct {
	Success    bool                 `json:"success"`
	LatencyMS  int64                `json:"latency_ms,omitempty"`
	StatusCode int                  `json:"status_code,omitempty"`
	Message    string               `json:"message"`
	Total      int                  `json:"total,omitempty"`
	Passed     int                  `json:"passed,omitempty"`
	Results    []testModelKeyResult `json:"results,omitempty"`
}

// testModel sends a minimal non-streaming chat request through the channel to
// verify that the channel's base URL, API key and the given model are usable.
func testModel(c *gin.Context) {
	var request testModelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	channel, err := op.ChannelGet(request.ChannelID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	if request.TestAllKeys {
		// all-keys：每个 key 独立超时，不用单一 30s 包裹整体（key 多时避免整体超时）
		resp.Success(c, runChannelAllKeysTest(c.Request.Context(), channel, request.Model, model.LogTypeTest))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	resp.Success(c, runChannelTest(ctx, channel, request.Model, model.LogTypeTest))
}

// runChannelAllKeysTest 逐个测试渠道下所有启用的 key，返回每个 key 的结果。
// 每个 key 独立 30s 超时；余额不足的 key 会被自动停用（与单 key 测试同规则）。
func runChannelAllKeysTest(ctx context.Context, channel *model.Channel, modelName string, logType string) testModelResponse {
	baseURL := channel.GetBaseUrl()
	if baseURL == "" {
		return testModelResponse{Success: false, Message: "channel has no available base url"}
	}

	var results []testModelKeyResult
	passed, tested := 0, 0
	for _, key := range channel.Keys {
		if !key.Enabled || key.ChannelKey == "" {
			continue
		}
		tested++
		keyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		res := runChannelKeyTest(keyCtx, channel, key, modelName, logType, "")
		cancel()
		results = append(results, testModelKeyResult{
			KeyID:      key.ID,
			KeyName:    key.DisplayName(),
			Success:    res.Success,
			LatencyMS:  res.LatencyMS,
			StatusCode: res.StatusCode,
			Message:    res.Message,
		})
		if res.Success {
			passed++
		}
	}

	if tested == 0 {
		return testModelResponse{Success: false, Message: "channel has no enabled key"}
	}

	return testModelResponse{
		Success: passed == tested,
		Message: fmt.Sprintf("%d/%d keys passed", passed, tested),
		Total:   tested,
		Passed:  passed,
		Results: results,
	}
}

// runChannelTest 对单个渠道发起一次最小聊天请求并写测试日志（默认规则选 key）。
// logType 指定日志类型：渠道测试（/channel/test-model）传 LogTypeTest，
// 分组测试（/group/test）传 LogTypeGroupTest。
func runChannelTest(ctx context.Context, channel *model.Channel, modelName string, logType string) testModelResponse {
	usedKey := channel.GetChannelKey()
	if usedKey.ChannelKey == "" {
		return testModelResponse{Success: false, Message: "channel has no available key"}
	}
	return runChannelKeyTest(ctx, channel, usedKey, modelName, logType, "")
}

// runChannelKeyTest 用指定 key 对渠道发起一次最小聊天请求并写测试日志。
// 与转发链路同规则：余额不足时自动停用该 key；若渠道所有 key 均已停用，则停用渠道。
func runChannelKeyTest(ctx context.Context, channel *model.Channel, usedKey model.ChannelKey, modelName string, logType string, requestModelName string) testModelResponse {
	if err := ctx.Err(); err != nil {
		return testModelResponse{Success: false, Message: err.Error()}
	}
	baseURL := channel.GetBaseUrl()
	if baseURL == "" {
		return testModelResponse{Success: false, Message: "channel has no available base url"}
	}
	if usedKey.ChannelKey == "" {
		return testModelResponse{Success: false, Message: "key is empty"}
	}

	testReq, requestContent, err := buildModelTestRequest(ctx, channel, baseURL, usedKey.ChannelKey, modelName)
	if err != nil {
		return testModelResponse{Success: false, Message: err.Error()}
	}

	client, err := helper.ChannelHttpClient(channel)
	if err != nil {
		return testModelResponse{Success: false, Message: err.Error()}
	}

	start := time.Now()
	upstreamResp, err := client.Do(testReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		if ctx.Err() != nil {
			// 请求取消不是 key 故障；不要把后续 key 写成网络错误或触发熔断。
			return testModelResponse{Success: false, LatencyMS: latency, Message: ctx.Err().Error()}
		}
		// 网络错误也记录到 key 状态，避免渠道详情里该 key 一直显示空白。
		usedKey.StatusCode = 0
		usedKey.LastUseTimeStamp = time.Now().Unix()
		usedKey.LastMessage = "网络错误"
		updateChannelKeyAfterTest(usedKey, 0)
		if model.IsCircuitFailureStatus(0) {
			balancer.RecordFailure(channel.ID, usedKey.ID, modelName)
		}
		saveTestLog(channel, usedKey, modelName, start, int(latency), 0, 0, 0, false, err.Error(), requestContent, "", logType, requestModelName)
		return testModelResponse{Success: false, LatencyMS: latency, Message: err.Error()}
	}
	defer upstreamResp.Body.Close()

	body, _ := io.ReadAll(upstreamResp.Body)
	success := upstreamResp.StatusCode >= http.StatusOK && upstreamResp.StatusCode < http.StatusMultipleChoices
	msg := "ok"
	if !success {
		msg = extractErrorMessage(body)
	}
	// 无论成功/失败，都把本次测试的 HTTP 状态和最近使用时间写回 key，
	// 与转发链路一致，渠道详情才能显示正确的 key 状态。
	usedKey.StatusCode = upstreamResp.StatusCode
	usedKey.LastUseTimeStamp = time.Now().Unix()
	if success {
		usedKey.LastMessage = "正常"
	} else {
		usedKey.LastMessage = msg
	}
	// 尽力解析 usage（OpenAI 兼容格式），用于测试日志的输入/输出长度与金额
	inputTokens, outputTokens := parseTestUsage(body)
	var cost float64
	if inputTokens > 0 || outputTokens > 0 {
		cost = calcTestCost(channel, modelName, inputTokens, outputTokens)
	}
	// 测试成功产生的费用累加到该 key 的 total_cost（与转发链路同口径），
	// 让卡片/编辑界面的消耗金额与额度扣除逻辑在测试后也能及时反映。
	// 先回写 key 状态，再执行余额不足停用；
	// 顺序不能反，否则后面回写会用旧的 enabled=true 覆盖停用结果。
	updateChannelKeyAfterTest(usedKey, cost)
	if success {
		balancer.RecordSuccess(channel.ID, usedKey.ID, modelName)
	} else if model.IsCircuitFailureStatus(upstreamResp.StatusCode) {
		balancer.RecordFailure(channel.ID, usedKey.ID, modelName)
	}
	if !success {
		// 余额不足：自动停用该 key；若渠道所有 key 均已停用，则停用渠道（与转发链路同规则）。
		op.DisableInsufficientKey(channel, usedKey, upstreamResp.StatusCode, body)
	}
	saveTestLog(channel, usedKey, modelName, start, int(latency), inputTokens, outputTokens, cost, success, msg, requestContent, string(body), logType, requestModelName)
	return testModelResponse{
		Success:    success,
		LatencyMS:  latency,
		StatusCode: upstreamResp.StatusCode,
		Message:    msg,
	}
}

// updateChannelKeyAfterTest 将测试后的 key 状态更新到缓存并立即落库，
// 保证渠道详情页能立刻显示正确的 status_code / 最近使用时间 / 消耗金额。
func updateChannelKeyAfterTest(key model.ChannelKey, cost float64) {
	if err := op.ChannelKeyRecordUsage(key, cost); err != nil {
		log.Warnf("failed to update channel key after test: %v", err)
		return
	}
	if err := op.ChannelKeySaveDB(context.Background()); err != nil {
		log.Warnf("failed to persist channel key after test: %v", err)
	}
}

// parseTestUsage 从测试响应体中尽力解析 usage（OpenAI 兼容格式：{"usage":{"prompt_tokens":N,"completion_tokens":M}}）。
// Anthropic/Gemini 等格式无法解析时返回 0，不影响日志写入。
func parseTestUsage(body []byte) (inputTokens, outputTokens int) {
	var resp struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, 0
	}
	return resp.Usage.PromptTokens, resp.Usage.CompletionTokens
}

// calcTestCost 按模型价格表计算测试请求费用（与转发链路同口径：单价 × tokens × 1e-6）。
func calcTestCost(channel *model.Channel, modelName string, inputTokens, outputTokens int) float64 {
	modelPrice := price.GetLLMPrice(modelName)
	if modelPrice == nil {
		return 0
	}
	return (float64(inputTokens)*modelPrice.Input + float64(outputTokens)*modelPrice.Output) * 1e-6
}

// saveTestLog 将一次测试写入测试日志，不阻塞测试响应。
// logType 区分渠道测试（LogTypeTest）与分组测试（LogTypeGroupTest）。
// requestContent / responseContent 保存测试请求与响应原文，供日志详情展开查看。
func saveTestLog(channel *model.Channel, usedKey model.ChannelKey, modelName string, startTime time.Time, latency, inputTokens, outputTokens int, cost float64, success bool, msg string, requestContent, responseContent string, logType string, requestModelName string) {
	// 分组测试时 requestModelName 传分组名称，和正常 API 调用分组日志的展示保持一致；
	// 渠道测试时 requestModelName 为空，继续用 modelName 作为请求模型名。
	logRequestName := requestModelName
	if logRequestName == "" {
		logRequestName = modelName
	}
	logEntry := model.RelayLog{
		Time:              startTime.Unix(),
		RequestModelName:  logRequestName,
		RequestAPIKeyName: usedKey.DisplayName(),
		ChannelId:         channel.ID,
		ChannelName:       channel.Name,
		ActualModelName:   modelName,
		InputTokens:       inputTokens,
		OutputTokens:      outputTokens,
		UseTime:           latency,
		Cost:              cost,
		Username:          op.UserGet().Username,
		LogType:           logType,
		RequestContent:    requestContent,
		ResponseContent:   responseContent,
	}
	if !success {
		logEntry.Error = msg
	}
	if err := op.RelayLogAdd(context.Background(), logEntry); err != nil {
		log.Warnf("failed to save test log: %v", err)
	}
}

// extractErrorMessage 从上游错误响应体中提取人类可读的错误信息。
// 兼容两种常见格式：
//  1. OpenAI / Anthropic / Gemini 风格：{"error":{"message":"..."}}
//  2. One API / new-api 等网关扁平风格：{"code":"INSUFFICIENT_BALANCE","message":"余额不足"}
//
// 无法解析时退回原始文本（截断到 200 字符）。
func extractErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "unknown error"
	}
	// 格式 1：嵌套 error.message
	var nested struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &nested); err == nil && strings.TrimSpace(nested.Error.Message) != "" {
		return strings.TrimSpace(nested.Error.Message)
	}
	// 格式 2：扁平 message 字段
	var flat struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &flat); err == nil && strings.TrimSpace(flat.Message) != "" {
		return strings.TrimSpace(flat.Message)
	}
	if len(trimmed) > 200 {
		trimmed = trimmed[:200] + "..."
	}
	return trimmed
}

// buildModelTestRequest constructs a minimal chat request in each provider
// family's native request format. OpenAI/Doubao use /chat/completions,
// Anthropic uses /messages and Gemini uses generateContent with a contents
// payload; URLs follow the same normalization rules as the upstream
// transformers (including the explicit /v1 suffix handling for Gemini).
func buildModelTestRequest(ctx context.Context, channel *model.Channel, baseURL, apiKey, modelName string) (*http.Request, string, error) {
	var (
		url     string
		headers = make(map[string]string)
		body    any
	)

	switch channel.Type {
	case llm.APIFormatOpenAIChatCompletion, llm.APIFormatOpenAIResponse:
		url = transformer.NormalizeBaseURL(baseURL, "v1") + "/chat/completions"
		headers["Authorization"] = "Bearer " + apiKey
		body = map[string]any{
			"model":      modelName,
			"max_tokens": 1,
			"messages": []map[string]string{
				{"role": "user", "content": "ping"},
			},
		}
	case model.ChannelTypeDoubao:
		url = transformer.NormalizeBaseURL(baseURL, "v3") + "/chat/completions"
		headers["Authorization"] = "Bearer " + apiKey
		body = map[string]any{
			"model":      modelName,
			"max_tokens": 1,
			"messages": []map[string]string{
				{"role": "user", "content": "ping"},
			},
		}
	case llm.APIFormatAnthropicMessage:
		url = transformer.NormalizeBaseURL(baseURL, "v1") + "/messages"
		headers["x-api-key"] = apiKey
		headers["Anthropic-Version"] = "2023-06-01"
		body = map[string]any{
			"model":      modelName,
			"max_tokens": 1,
			"messages": []map[string]string{
				{"role": "user", "content": "ping"},
			},
		}
	case llm.APIFormatGeminiContents:
		version := "v1beta"
		// Gemini transformer 保留用户显式填写的 /v1 后缀；这里同样处理，避免拼成 /v1/v1beta。
		if strings.HasSuffix(strings.TrimRight(baseURL, "/"), "/v1") {
			version = ""
		}
		url = transformer.NormalizeBaseURL(baseURL, version) + "/models/" + modelName + ":generateContent"
		headers["x-goog-api-key"] = apiKey
		body = map[string]any{
			"contents": []map[string]any{
				{
					"parts": []map[string]string{{"text": "ping"}},
				},
			},
			"generationConfig": map[string]any{"maxOutputTokens": 1},
		}
	default:
		return nil, "", fmt.Errorf("unsupported channel type: %s", channel.Type)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, h := range channel.CustomHeader {
		if h.HeaderKey != "" {
			if req.Header.Get(h.HeaderKey) != "" && llmhttpclient.IsSensitiveHeader(h.HeaderKey) {
				continue
			}
			req.Header.Set(h.HeaderKey, h.HeaderValue)
		}
	}
	return req, string(bodyBytes), nil
}

func syncChannel(c *gin.Context) {
	task.SyncModelsTask()
	resp.Success(c, nil)
}

func getLastSyncTime(c *gin.Context) {
	time := task.GetLastSyncModelsTime()
	resp.Success(c, time)
}
