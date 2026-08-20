package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/dlclark/regexp2"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
)

func init() {
	router.NewGroupRouter("/api/v1/group").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(getGroupList),
		).
		AddRoute(
			router.NewRoute("/create", http.MethodPost).
				Handle(createGroup),
		).
		AddRoute(
			router.NewRoute("/update", http.MethodPost).
				Handle(updateGroup),
		).
		AddRoute(
			router.NewRoute("/delete/:id", http.MethodDelete).
				Handle(deleteGroup),
		).
		AddRoute(
			router.NewRoute("/test", http.MethodPost).
				Handle(testGroup),
		)
	// AddRoute(
	// 	router.NewRoute("/auto-add-item", http.MethodPost).
	// 		Handle(autoAddGroupItem),
	// )
}

func getGroupList(c *gin.Context) {
	groups, err := op.GroupList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, groups)
}

func createGroup(c *gin.Context) {
	var group model.Group
	if err := c.ShouldBindJSON(&group); err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if group.MatchRegex != "" {
		_, err := regexp2.Compile(group.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := op.GroupCreate(&group, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, group)
}

func updateGroup(c *gin.Context) {
	var req model.GroupUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.MatchRegex != nil {
		_, err := regexp2.Compile(*req.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	group, err := op.GroupUpdate(&req, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, group)
}

func deleteGroup(c *gin.Context) {
	id := c.Param("id")
	idNum, err := strconv.Atoi(id)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := op.GroupDel(idNum, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, "group deleted successfully")
}

// testGroupRequest is the request body for POST /api/v1/group/test.
type testGroupRequest struct {
	GroupID int `json:"group_id" binding:"required"`
}

// testGroupResponse is the result of a group connectivity test.
type testGroupResponse struct {
	Success            bool   `json:"success"`
	ChannelName        string `json:"channel_name,omitempty"`
	ModelName          string `json:"model_name,omitempty"`
	LatencyMS          int64  `json:"latency_ms,omitempty"`
	StatusCode         int    `json:"status_code,omitempty"`
	ModelContextLength int    `json:"model_context_length,omitempty"` // 上游模型上下文长度(tokens)；与分组 context_length 不一致时返回
	Message            string `json:"message"`
}

// testGroup 根据分组规则（轮询/随机/加权/故障转移）依次测试候选渠道+模型是否可调用。
// 模拟真实转发：跳过禁用/无可用 key/熔断中的候选；同一 channel+model 候选内先逐个 key 测试，
// 当前候选所有 key 都失败后才降级到下一个候选，任一成功即返回成功。
func testGroup(c *gin.Context) {
	var request testGroupRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	group, err := op.GroupGet(request.GroupID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	if len(group.Items) == 0 {
		resp.Success(c, testGroupResponse{Success: false, Message: "group has no channel items"})
		return
	}

	// 按分组模式生成候选顺序（apiKeyID=0：测试场景无粘性会话）
	iter := balancer.NewIterator(*group, 0, "")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	var lastErr string
	for iter.Next() {
		item := iter.Item()
		channel, err := op.ChannelGet(item.ChannelID, ctx)
		if err != nil {
			iter.Skip(item.ChannelID, 0, fmt.Sprintf("channel_%d", item.ChannelID), err.Error())
			continue
		}
		if !channel.Enabled {
			iter.Skip(channel.ID, 0, channel.Name, "channel disabled")
			continue
		}
		keys := channel.GetChannelKeys()
		if len(keys) == 0 {
			iter.Skip(channel.ID, 0, channel.Name, "no available key")
			continue
		}

		for _, usedKey := range keys {
			if iter.SkipCircuitBreak(channel.ID, usedKey.ID, channel.Name) {
				continue
			}

			result := runChannelKeyTest(ctx, channel, usedKey, item.ModelName, model.LogTypeGroupTest, group.Name)
			if result.Success {
				// 测试成功后，从上游获取模型上下文长度，用于与分组设置比对
				ctxLookup, cancelLookup := context.WithTimeout(c.Request.Context(), 15*time.Second)
				modelCtxLen := getModelContextLength(ctxLookup, channel, item.ModelName)
				cancelLookup()

				resp.Success(c, testGroupResponse{
					Success:            true,
					ChannelName:        channel.Name,
					ModelName:          item.ModelName,
					LatencyMS:          result.LatencyMS,
					StatusCode:         result.StatusCode,
					ModelContextLength: modelCtxLen,
					Message:            result.Message,
				})
				return
			}
			lastErr = fmt.Sprintf("channel %s model %s key %s: %s", channel.Name, item.ModelName, usedKey.DisplayName(), result.Message)
		}
	}
	if lastErr == "" {
		lastErr = "no available channel"
	}
	resp.Success(c, testGroupResponse{Success: false, Message: lastErr})
}

// getModelContextLength 从上游渠道获取指定模型的上下文长度(tokens)。
// 仅对 OpenAI 兼容接口有效；其他类型或获取失败时返回 0。
func getModelContextLength(ctx context.Context, channel *model.Channel, modelName string) int {
	// 仅处理 OpenAI 兼容格式的渠道
	switch channel.Type {
	case llm.APIFormatAnthropicMessage, llm.APIFormatGeminiContents:
		return 0
	}

	baseURL := transformer.NormalizeBaseURL(channel.GetBaseUrl(), "v1")
	url := baseURL + "/models"

	client, err := helper.ChannelHttpClient(channel)
	if err != nil {
		return 0
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+channel.GetChannelKey().ChannelKey)
	// 应用自定义头
	for _, header := range channel.CustomHeader {
		if header.HeaderKey != "" {
			req.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var result struct {
		Data []struct {
			ID             string `json:"id"`
			MaxTokens      int    `json:"max_tokens,omitempty"`
			MaxInputTokens int    `json:"max_input_tokens,omitempty"`
			ContextLength  int    `json:"context_length,omitempty"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0
	}

	// 查找匹配模型（忽略大小写），提取上下文长度
	modelNameLower := strings.ToLower(modelName)
	for _, m := range result.Data {
		if strings.ToLower(m.ID) == modelNameLower {
			if m.ContextLength > 0 {
				return m.ContextLength
			}
			if m.MaxTokens > 0 {
				return m.MaxTokens
			}
			if m.MaxInputTokens > 0 {
				return m.MaxInputTokens
			}
		}
	}
	return 0
}

// func autoAddGroupItem(c *gin.Context) {
// 	var req struct {
// 		ID int `json:"id"`
// 	}
// 	if err := c.ShouldBindJSON(&req); err != nil {
// 		resp.Error(c, http.StatusBadRequest, err.Error())
// 		return
// 	}
// 	if req.ID <= 0 {
// 		resp.Error(c, http.StatusBadRequest, "invalid id")
// 		return
// 	}
// 	err := worker.AutoAddGroupItem(req.ID, c.Request.Context())
// 	if err != nil {
// 		resp.Error(c, http.StatusInternalServerError, err.Error())
// 		return
// 	}
// 	resp.Success(c, nil)
// }
