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
	Version string `json:"version"`
	Device  string `json:"device"`
	User    string `json:"user"`
	Issue   string `json:"issue"`
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

const systemPrompt = `你是一个技术支持信息收集助手。你的唯一任务是从用户的【当前这一条消息】中提取以下四项信息。

需要收集的信息：
1. version - 软件版本号（如 v1.2.3、2.0.1、g1 等）
2. device - 设备型号和操作系统（如 iPhone 15/iOS 17、Windows 11、华为 Mate60 等）
3. user - 用户姓名或工号（如 张三、工号12345 等）
4. issue - 遇到的具体问题描述（如 无法登录、页面崩溃 等）

严格规则：
- 只从用户当前这一条消息中提取新信息
- 如果用户这条消息没有明确提到某项信息，该字段必须返回空字符串 ""
- 不要从"当前已收集信息"中复制任何内容到结果中
- 不要把问候语（如"你好"、"请问"）当作任何信息
- 不要猜测或编造信息
- 如果用户纠正了之前的信息（如"版本不对，应该是v3.0"），返回新值
- 信息要简洁准确

返回严格的 JSON 格式，不要有其他任何文字：
{"version": "", "device": "", "user": "", "issue": ""}`

// ExtractInfo 从用户的单条消息中提取信息。
func (c *OpenAICompatibleClient) ExtractInfo(ctx context.Context, userMessage string, collectedInfo map[string]string) (*ExtractionResult, error) {
	var messages []openai.ChatCompletionMessage

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})

	// 构建上下文：已收集的信息 + 当前消息
	var userPrompt strings.Builder
	userPrompt.WriteString("当前已收集的信息（仅供参考，不要复制到结果中）：\n")
	fieldNames := map[string]string{
		"version": "版本信息",
		"device":  "设备信息",
		"user":    "用户信息",
		"issue":   "问题描述",
	}
	for _, key := range []string{"version", "device", "user", "issue"} {
		if val, ok := collectedInfo[key]; ok && val != "" {
			userPrompt.WriteString(fmt.Sprintf("- %s: %s（已收集）\n", fieldNames[key], val))
		} else {
			userPrompt.WriteString(fmt.Sprintf("- %s: 未收集\n", fieldNames[key]))
		}
	}
	userPrompt.WriteString(fmt.Sprintf("\n用户当前消息：%s\n\n请从这条消息中提取信息，返回 JSON。", userMessage))

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
	result.Version = cleanExtractedValue(result.Version)
	result.Device = cleanExtractedValue(result.Device)
	result.User = cleanExtractedValue(result.User)
	result.Issue = cleanExtractedValue(result.Issue)

	log.Printf("[LLM] Extracted: version=%q, device=%q, user=%q, issue=%q",
		result.Version, result.Device, result.User, result.Issue)

	return &result, nil
}

// cleanExtractedValue 清理提取的值。
func cleanExtractedValue(val string) string {
	val = strings.TrimSpace(val)
	// 过滤掉无意义的占位值
	lower := strings.ToLower(val)
	if lower == "未提供" || lower == "无" || lower == "未知" || lower == "null" || lower == "none" || lower == "n/a" {
		return ""
	}
	return val
}
