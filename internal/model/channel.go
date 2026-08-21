package model

import (
	"net/http"
	"sort"
	"time"

	"github.com/looplj/axonhub/llm"
)

type AutoGroupType int

const (
	AutoGroupTypeNone  AutoGroupType = 0 //不自动分组
	AutoGroupTypeFuzzy AutoGroupType = 1 //模糊匹配
	AutoGroupTypeExact AutoGroupType = 2 //准确匹配
	AutoGroupTypeRegex AutoGroupType = 3 //正则匹配
)

const ChannelTypeDoubao llm.APIFormat = "doubao"

type Channel struct {
	ID            int            `json:"id" gorm:"primaryKey"`
	Name          string         `json:"name" gorm:"unique;not null"`
	Type          llm.APIFormat  `json:"type"`
	Enabled       bool           `json:"enabled" gorm:"default:true"`
	BaseUrls      []BaseUrl      `json:"base_urls" gorm:"serializer:json"`
	Keys          []ChannelKey   `json:"keys" gorm:"foreignKey:ChannelID"`
	Model         string         `json:"model"`
	CustomModel   string         `json:"custom_model"`
	Proxy         bool           `json:"proxy" gorm:"default:false"`
	AutoSync      bool           `json:"auto_sync" gorm:"default:false"`
	AutoGroup     AutoGroupType  `json:"auto_group" gorm:"default:0"`
	CustomHeader  []CustomHeader `json:"custom_header" gorm:"serializer:json"`
	ParamOverride *string        `json:"param_override"`
	ChannelProxy  *string        `json:"channel_proxy"`
	Stats         *StatsChannel  `json:"stats,omitempty" gorm:"foreignKey:ChannelID"`
	MatchRegex    *string        `json:"match_regex"`
}

type BaseUrl struct {
	URL   string `json:"url"`
	Delay int    `json:"delay"`
}

type CustomHeader struct {
	HeaderKey   string `json:"header_key"`
	HeaderValue string `json:"header_value"`
}

type ChannelKey struct {
	ID               int     `json:"id" gorm:"primaryKey"`
	ChannelID        int     `json:"channel_id"`
	Enabled          bool    `json:"enabled" gorm:"default:true"`
	ChannelKey       string  `json:"channel_key"`
	StatusCode       int     `json:"status_code"`
	LastUseTimeStamp int64   `json:"last_use_time_stamp"`
	TotalCost        float64 `json:"total_cost"`
	Quota            float64 `json:"quota"` // 额度（$），0 表示不限制
	Remark           string  `json:"remark"`
	LastMessage      string  `json:"last_message,omitempty" gorm:"column:last_message;type:text"` // 最近一次返回详情/错误信息，供前端悬停展示
}

// DisplayName 返回渠道 key 的展示名：备注优先，否则返回 key 本体（前端统一脱敏展示）。
func (k ChannelKey) DisplayName() string {
	if k.Remark != "" {
		return k.Remark
	}
	return k.ChannelKey
}

// ChannelUpdateRequest 渠道更新请求 - 仅包含变更的数据
type ChannelUpdateRequest struct {
	ID            int             `json:"id" binding:"required"`
	Name          *string         `json:"name,omitempty"`
	Type          *llm.APIFormat  `json:"type,omitempty"`
	Enabled       *bool           `json:"enabled,omitempty"`
	BaseUrls      *[]BaseUrl      `json:"base_urls,omitempty"`
	Model         *string         `json:"model,omitempty"`
	CustomModel   *string         `json:"custom_model,omitempty"`
	Proxy         *bool           `json:"proxy,omitempty"`
	AutoSync      *bool           `json:"auto_sync,omitempty"`
	AutoGroup     *AutoGroupType  `json:"auto_group,omitempty"`
	CustomHeader  *[]CustomHeader `json:"custom_header,omitempty"`
	ChannelProxy  *string         `json:"channel_proxy,omitempty"`
	ParamOverride *string         `json:"param_override,omitempty"`
	MatchRegex    *string         `json:"match_regex,omitempty"`

	KeysToAdd    []ChannelKeyAddRequest    `json:"keys_to_add,omitempty"`
	KeysToUpdate []ChannelKeyUpdateRequest `json:"keys_to_update,omitempty"`
	KeysToDelete []int                     `json:"keys_to_delete,omitempty"`
}

type ChannelKeyAddRequest struct {
	Enabled    bool    `json:"enabled"`
	ChannelKey string  `json:"channel_key" binding:"required"`
	Quota      float64 `json:"quota"`
	Remark     string  `json:"remark"`
}

type ChannelKeyUpdateRequest struct {
	ID         int      `json:"id" binding:"required"`
	Enabled    *bool    `json:"enabled,omitempty"`
	ChannelKey *string  `json:"channel_key,omitempty"`
	Quota      *float64 `json:"quota,omitempty"`
	Remark     *string  `json:"remark,omitempty"`
}

func (c *Channel) GetBaseUrl() string {
	if c == nil || len(c.BaseUrls) == 0 {
		return ""
	}

	bestURL := ""
	bestDelay := 0
	bestSet := false

	for _, bu := range c.BaseUrls {
		if bu.URL == "" {
			continue
		}
		if !bestSet || bu.Delay < bestDelay {
			bestURL = bu.URL
			bestDelay = bu.Delay
			bestSet = true
		}
	}

	return bestURL
}

// isSuccessStatus 判断 key 最近一次调用是否为成功状态（HTTP 2xx/3xx）。
func isSuccessStatus(k ChannelKey) bool {
	return k.StatusCode > 0 && k.StatusCode < 400
}

// IsCircuitFailureStatus 判断一次 HTTP 结果是否足以影响该 key+model 的熔断状态。
// 普通请求错误不应因为重复提交而把正常 key 熔断；404 则保留为模型不可用信号，
// 让当前渠道对该模型进入短暂熔断并转到其他渠道。
func IsCircuitFailureStatus(statusCode int) bool {
	switch statusCode {
	case 0, http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden,
		http.StatusNotFound, http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	default:
		return statusCode >= http.StatusInternalServerError
	}
}

// isKeyFailureStatus 判断失败是否更可能由 key 本身导致。
// 普通 400/404/422 多数是请求或模型问题，不应让 key 进入全局冷却；
// 认证、配额、限流、网络和服务端故障才需要暂时避开该 key。
func isKeyFailureStatus(k ChannelKey) bool {
	return k.StatusCode != http.StatusNotFound && IsCircuitFailureStatus(k.StatusCode)
}

// GetChannelKeys 按以下规则返回渠道所有可用 key：
//  1. 过滤禁用、空 key；剩余额度耗尽（Quota > 0 且 Quota-TotalCost <= 0）的 key 视为不可用；
//  2. 最近 5 分钟内 key 相关调用失败（认证/配额/限流/服务端或网络错误）的 key 进入冷却，跳过；
//  3. 可用 key 中优先返回最近成功过的 key（LastUseTimeStamp 最大且 StatusCode 为成功），
//     让同一个 key 持续使用，避免多个健康 key 之间轮询；
//  4. 没有成功记录时按 key ID 升序返回，保证行为稳定。
//
// 返回的切片用于同渠道同模型故障时按顺序尝试下一个 key。
func (c *Channel) GetChannelKeys() []ChannelKey {
	if c == nil || len(c.Keys) == 0 {
		return nil
	}

	nowSec := time.Now().Unix()
	const cooldownSec = int64(5 * 60) // 故障冷却 5 分钟

	var available []ChannelKey
	for _, k := range c.Keys {
		if !k.Enabled || k.ChannelKey == "" {
			continue
		}
		// 剩余额度耗尽的 key 视为不可用
		if k.Quota > 0 && k.Quota-k.TotalCost <= 0 {
			continue
		}
		// 故障冷却：最近 5 分钟内 key 相关调用失败（认证/配额/限流/服务端或网络错误）
		if k.LastUseTimeStamp > 0 && nowSec-k.LastUseTimeStamp < cooldownSec {
			if isKeyFailureStatus(k) {
				continue
			}
		}
		available = append(available, k)
	}

	sort.Slice(available, func(i, j int) bool {
		iOK := isSuccessStatus(available[i])
		jOK := isSuccessStatus(available[j])
		if iOK != jOK {
			return iOK // 成功过的 key 排前面
		}
		if iOK {
			// 都是成功过的 key：最近成功者优先，保持“一直用同一个 key”
			return available[i].LastUseTimeStamp > available[j].LastUseTimeStamp
		}
		// 都没有成功记录：按 key ID 稳定排序
		return available[i].ID < available[j].ID
	})
	return available
}

// GetChannelKey 按以下规则选择渠道 key：
//  1. 过滤禁用、空 key；剩余额度耗尽（Quota > 0 且 Quota-TotalCost <= 0）的 key 视为不可用；
//  2. 最近 5 分钟内 key 相关调用失败（认证/配额/限流/服务端或网络错误）的 key 进入冷却，跳过；
//  3. 优先选择最近成功过的 key，避免健康 key 之间轮询；
//  4. 没有成功记录时选择第一个可用 key。
func (c *Channel) GetChannelKey() ChannelKey {
	keys := c.GetChannelKeys()
	if len(keys) == 0 {
		return ChannelKey{}
	}
	return keys[0]
}
