// Package llm 提供 LLM 客户端（OpenAI 兼容），用于从用户消息中提取信息。
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// Client LLM 客户端接口。
type Client interface {
	// ExtractInfo 从用户的单条消息中提取信息字段。
	ExtractInfo(ctx context.Context, userMessage string, collectedInfo map[string]string) (*ExtractionResult, error)
}

// ExtractionResult 表示从单条消息中提取的信息。
type ExtractionResult struct {
	Issue          string `json:"issue"`           // 问题描述
	OccurTime      string `json:"occur_time"`      // 发生时间
	Reproducible   string `json:"reproducible"`    // 是否必现
	VPN            string `json:"vpn"`             // 是否使用VPN
	AppVersion     string `json:"app_version"`     // 应用版本
	GlassesVersion string `json:"glasses_version"` // 眼镜版本
	GlassesSN      string `json:"glasses_sn"`      // 眼镜SN号
	RingVersion    string `json:"ring_version"`    // 戒指版本
	RingSN         string `json:"ring_sn"`         // 戒指SN号
	PhoneModel     string `json:"phone_model"`     // 手机型号
	PhoneOS        string `json:"phone_os"`        // 手机系统版本
}

// AllFieldKeys 返回所有字段的 key 列表（与 JSON tag 一致）。
var AllFieldKeys = []string{
	"issue", "occur_time", "reproducible", "vpn",
	"app_version", "glasses_version", "glasses_sn",
	"ring_version", "ring_sn", "phone_model", "phone_os",
}

// FieldDisplayNames 字段 key → 中英双语显示名称。
var FieldDisplayNames = map[string]string{
	"issue":           "问题描述 / Issue Description",
	"occur_time":      "发生时间 / Time of Occurrence",
	"reproducible":    "是否必现 / Reproducible?",
	"vpn":             "是否使用VPN / Using VPN?",
	"app_version":     "应用版本 / App Version",
	"glasses_version": "眼镜版本 / Glasses Firmware",
	"glasses_sn":      "眼镜SN号 / Glasses SN",
	"ring_version":    "戒指版本 / Ring Firmware",
	"ring_sn":         "戒指SN号 / Ring SN",
	"phone_model":     "手机型号 / Phone Model",
	"phone_os":        "手机系统版本 / Phone OS Version",
}

// ProviderConfig LLM 提供商配置。
type ProviderConfig struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
}

// NewClient 创建 LLM 客户端的便捷函数。
func NewClient(cfg *ProviderConfig) (Client, error) {
	return NewOpenAICompatibleClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
}

// OpenAICompatibleClient OpenAI 兼容客户端实现。
type OpenAICompatibleClient struct {
	client *openai.Client
	model  string
}

// NewOpenAICompatibleClient 创建新的 OpenAI 兼容客户端。
func NewOpenAICompatibleClient(baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	return &OpenAICompatibleClient{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}, nil
}

const systemPrompt = `You are a tech support information collector. Your ONLY task is to extract the following 11 fields from the user's CURRENT message. The user may write in Chinese or English — handle both languages.

Fields to collect:
1. issue - Problem description (what happened, what the user was doing before the issue)
2. occur_time - Time of occurrence (e.g. "today 3pm", "2026-02-09 14:00", "今天下午3点")
3. reproducible - Is it reproducible? (e.g. "yes", "no", "sometimes", "是", "否", "偶现")
4. vpn - Using VPN? If yes, specify region/node (e.g. "no", "yes, US node", "否", "是，香港节点")
5. app_version - App version number (e.g. 2.0.6, v1.2.3)
6. glasses_version - Glasses firmware version (e.g. 1.2.0, v2.1)
7. glasses_sn - Glasses serial number (e.g. G2xxxxxxx, SN12345)
8. ring_version - Ring firmware version (e.g. 1.0, v2.1)
9. ring_sn - Ring serial number (e.g. R1xxxxxxx, SN67890)
10. phone_model - Phone model (e.g. iPhone 15 Pro, Xiaomi 14, Samsung Galaxy S24)
11. phone_os - Phone OS version (e.g. Android 15, iOS 18.3.2)

Strict rules:
- Extract ONLY from the user's current message
- If a field is not mentioned, return empty string ""
- Do NOT copy from "already collected info"
- Do NOT treat greetings ("hi", "hello", "你好") as any info
- Do NOT guess or fabricate info
- If the user corrects previous info (e.g. "wrong version, it's v3.0"), return the new value
- Distinguish between app version vs glasses/ring firmware version
- Distinguish between glasses SN vs ring SN
- Distinguish between phone model (hardware) vs phone OS (software)
- Keep extracted values concise and accurate
- Preserve the user's original language in the extracted values

Return strict JSON only, no other text:
{"issue": "", "occur_time": "", "reproducible": "", "vpn": "", "app_version": "", "glasses_version": "", "glasses_sn": "", "ring_version": "", "ring_sn": "", "phone_model": "", "phone_os": ""}`

// ExtractInfo 从用户的单条消息中提取信息。
func (c *OpenAICompatibleClient) ExtractInfo(ctx context.Context, userMessage string, collectedInfo map[string]string) (*ExtractionResult, error) {
	var messages []openai.ChatCompletionMessage

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})

	// 构建上下文：已收集的信息 + 当前消息
	var userPrompt strings.Builder
	userPrompt.WriteString("Already collected info (reference only, do NOT copy into result):\n")
	for _, key := range AllFieldKeys {
		name := FieldDisplayNames[key]
		if val, ok := collectedInfo[key]; ok && val != "" {
			userPrompt.WriteString(fmt.Sprintf("- %s: %s (collected)\n", name, val))
		} else {
			userPrompt.WriteString(fmt.Sprintf("- %s: not yet collected\n", name))
		}
	}
	userPrompt.WriteString(fmt.Sprintf("\nUser's current message: %s\n\nExtract info from this message and return JSON.", userMessage))

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userPrompt.String(),
	})

	log.Printf("[LLM] Extracting info from message: %s", userMessage)

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Temperature: 0.1, // 低温度确保提取准确
		},
	)
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLM 没有返回结果")
	}

	content := resp.Choices[0].Message.Content
	log.Printf("[LLM] Raw response: %s", content)

	return parseExtractionResult(content)
}

// parseExtractionResult 解析 LLM 返回的提取结果。
func parseExtractionResult(content string) (*ExtractionResult, error) {
	content = strings.TrimSpace(content)

	// 去除 markdown 代码块标记
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	// 尝试找到 JSON 对象
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	content = strings.TrimSpace(content)

	var result ExtractionResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		log.Printf("[LLM] Failed to parse response as JSON: %v (content: %s)", err, content)
		return &ExtractionResult{}, nil // 解析失败返回空结果
	}

	// 清理提取的值（去除空白和无意义内容）
	result.Issue = cleanExtractedValue(result.Issue)
	result.OccurTime = cleanExtractedValue(result.OccurTime)
	result.Reproducible = cleanExtractedValue(result.Reproducible)
	result.VPN = cleanExtractedValue(result.VPN)
	result.AppVersion = cleanExtractedValue(result.AppVersion)
	result.GlassesVersion = cleanExtractedValue(result.GlassesVersion)
	result.GlassesSN = cleanExtractedValue(result.GlassesSN)
	result.RingVersion = cleanExtractedValue(result.RingVersion)
	result.RingSN = cleanExtractedValue(result.RingSN)
	result.PhoneModel = cleanExtractedValue(result.PhoneModel)
	result.PhoneOS = cleanExtractedValue(result.PhoneOS)

	log.Printf("[LLM] Extracted: issue=%q, occur_time=%q, reproducible=%q, vpn=%q, app_version=%q, glasses_version=%q, glasses_sn=%q, ring_version=%q, ring_sn=%q, phone_model=%q, phone_os=%q",
		result.Issue, result.OccurTime, result.Reproducible, result.VPN,
		result.AppVersion, result.GlassesVersion, result.GlassesSN,
		result.RingVersion, result.RingSN, result.PhoneModel, result.PhoneOS)

	return &result, nil
}

// cleanExtractedValue 清理提取的值。
func cleanExtractedValue(val string) string {
	val = strings.TrimSpace(val)
	// 过滤掉无意义的占位值（中英文）
	lower := strings.ToLower(val)
	meaningless := []string{
		"未提供", "无", "未知", "null", "none", "n/a", "not provided",
		"unknown", "not mentioned", "not specified", "not applicable",
		"未收集", "not yet collected", "not collected",
	}
	for _, m := range meaningless {
		if lower == m {
			return ""
		}
	}
	return val
}

// ToFieldMap 将 ExtractionResult 转为 map[string]string，方便与 RequiredFields 统一处理。
func (r *ExtractionResult) ToFieldMap() map[string]string {
	return map[string]string{
		"issue":           r.Issue,
		"occur_time":      r.OccurTime,
		"reproducible":    r.Reproducible,
		"vpn":             r.VPN,
		"app_version":     r.AppVersion,
		"glasses_version": r.GlassesVersion,
		"glasses_sn":      r.GlassesSN,
		"ring_version":    r.RingVersion,
		"ring_sn":         r.RingSN,
		"phone_model":     r.PhoneModel,
		"phone_os":        r.PhoneOS,
	}
}
