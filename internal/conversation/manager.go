// Package conversation 提供会话管理功能。
package conversation

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/even/feishu-bot/internal/llm"
	"github.com/even/feishu-bot/pkg/models"
)

// EscalatePrefix 是触发自动转人工的响应前缀。
const EscalatePrefix = "ESCALATE:"

// Manager 管理会话和信息收集。
type Manager struct {
	store   *Store
	llm     llm.Client
	prompts *PromptManager
}

// NewManager 创建新的会话管理器。
func NewManager(store *Store, llmClient llm.Client, prompts *PromptManager) *Manager {
	return &Manager{
		store:   store,
		llm:     llmClient,
		prompts: prompts,
	}
}

// ProcessMessage 处理用户消息，返回回复内容。
// 如果返回值以 EscalatePrefix 开头，表示需要自动转人工。
func (m *Manager) ProcessMessage(ctx context.Context, chatID, senderID, senderName, content, msgType, fileKey, messageID string) (string, error) {
	log.Printf("[Manager] ProcessMessage: chatID=%s, content=%q, msgType=%s, fileKey=%s", chatID, content, msgType, fileKey)

	// 获取或创建会话
	conv, err := m.store.GetOrCreateConversation(ctx, chatID, senderID, senderName)
	if err != nil {
		return "", fmt.Errorf("failed to get conversation: %w", err)
	}

	// 处理非文本消息（文件、图片等）
	if msgType != "text" {
		return m.handleFileMessage(ctx, conv, content, fileKey, messageID)
	}

	// 处理空文本
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", nil
	}

	// ====== 建议/反馈 模式检测 ======
	if conv.Mode == models.ModeUnknown {
		if isSuggestion(trimmed) {
			return m.handleSuggestion(ctx, conv, trimmed)
		}
	}
	// 如果已经是建议模式（不应发生，因为建议模式会立即提交），跳过
	if conv.Mode == models.ModeSuggestion {
		return m.handleSuggestion(ctx, conv, trimmed)
	}

	// ====== 问题反馈模式 ======
	conv.Mode = models.ModeIssue

	// 添加用户消息
	conv.AddMessage("user", content)

	// 获取当前已收集的信息快照
	collectedInfo := m.getCollectedInfoSnapshot(conv)

	// 使用 LLM 从当前这条消息中提取信息
	var result *llm.ExtractionResult
	if m.llm != nil {
		result, err = m.llm.ExtractInfo(ctx, content, collectedInfo)
		if err != nil {
			log.Printf("[Manager] LLM extraction failed: %v", err)
			result = &llm.ExtractionResult{} // 使用空结果，不影响流程
		}
	} else {
		result = &llm.ExtractionResult{}
	}

	// 合并新提取的信息到会话
	newInfoParts := m.mergeExtractedInfo(conv, result, collectedInfo)

	// 检查信息是否已完整
	if conv.IsInfoComplete() {
		return m.buildEscalateResponse(ctx, conv)
	}

	// 构建智能回复
	response := m.buildSmartResponse(newInfoParts, conv)
	conv.AddMessage("assistant", response)

	// 保存会话
	if err := m.store.SaveConversation(ctx, conv); err != nil {
		return "", fmt.Errorf("failed to save conversation: %w", err)
	}

	return response, nil
}

// isSuggestion 检测消息是否为建议/反馈格式（支持中英文）。
func isSuggestion(text string) bool {
	lower := strings.ToLower(text)
	prefixes := []string{
		"反馈：", "反馈:", "建议：", "建议:",
		"feedback：", "feedback:", "suggestion：", "suggestion:",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// handleSuggestion 处理建议/反馈消息，直接提交。
func (m *Manager) handleSuggestion(ctx context.Context, conv *models.Conversation, content string) (string, error) {
	conv.Mode = models.ModeSuggestion
	conv.SuggestionText = content
	conv.AddMessage("user", content)

	// 建议模式直接提交
	var sb strings.Builder
	sb.WriteString("已收到您的反馈/建议 / Your feedback has been received:\n\n")
	sb.WriteString(content)
	sb.WriteString("\n\n正在为您提交到技术支持团队... / Submitting to the support team...")

	userMsg := sb.String()
	conv.AddMessage("assistant", userMsg)

	if err := m.store.SaveConversation(ctx, conv); err != nil {
		return "", fmt.Errorf("failed to save conversation: %w", err)
	}

	return EscalatePrefix + userMsg, nil
}

// handleFileMessage 处理文件类消息。
func (m *Manager) handleFileMessage(ctx context.Context, conv *models.Conversation, content, fileKey, messageID string) (string, error) {
	// 记录文件信息（messageID + fileKey + 文件名）
	if fileKey != "" && messageID != "" {
		// 从 content 中提取文件名（格式: "上传了文件: xxx.zip"）
		fileName := "attachment"
		if strings.HasPrefix(content, "上传了文件: ") {
			fileName = strings.TrimPrefix(content, "上传了文件: ")
		}
		conv.AddFile(models.FileInfo{
			MessageID: messageID,
			FileKey:   fileKey,
			FileName:  fileName,
		})
	}

	conv.AddMessage("user", content)

	// 设置为问题模式（如果还未确定）
	if conv.Mode == models.ModeUnknown {
		conv.Mode = models.ModeIssue
	}

	// 检查信息是否已完整
	if conv.IsInfoComplete() {
		conv.AddMessage("assistant", "收到文件。信息已完整，正在为您提交...\nFile received. All info collected, submitting...")
		if err := m.store.SaveConversation(ctx, conv); err != nil {
			return "", err
		}
		return m.buildEscalateResponse(ctx, conv)
	}

	// 信息不完整，提示用户
	missing := conv.GetMissingFields()
	var sb strings.Builder
	sb.WriteString("收到文件，已记录。/ File received.\n\n")
	sb.WriteString("还需要以下信息 / Still need the following info:\n")
	for _, name := range missing {
		sb.WriteString(fmt.Sprintf("- %s\n", name))
	}
	sb.WriteString("\n回复「转人工」或 \"submit\" 可直接提交当前信息。\nReply \"submit\" to submit current info directly.")

	response := sb.String()
	conv.AddMessage("assistant", response)

	if err := m.store.SaveConversation(ctx, conv); err != nil {
		return "", err
	}

	return response, nil
}

// getCollectedInfoSnapshot 获取当前已收集信息的快照。
func (m *Manager) getCollectedInfoSnapshot(conv *models.Conversation) map[string]string {
	snapshot := make(map[string]string)
	if conv.CollectedInfo != nil {
		for k, v := range conv.CollectedInfo {
			snapshot[k] = v
		}
	}
	return snapshot
}

// mergeExtractedInfo 将 LLM 提取的新信息合并到会话中，返回新收集/更新的信息描述。
func (m *Manager) mergeExtractedInfo(conv *models.Conversation, result *llm.ExtractionResult, oldInfo map[string]string) []string {
	var newParts []string

	fieldMap := result.ToFieldMap()

	for _, key := range llm.AllFieldKeys {
		newValue := fieldMap[key]
		if newValue == "" {
			continue // LLM 没有从当前消息中提取到此字段
		}
		oldVal := oldInfo[key]
		if oldVal == newValue {
			continue // 值没有变化，跳过
		}
		name := llm.FieldDisplayNames[key]
		conv.SetCollectedInfo(key, newValue)
		if oldVal == "" {
			newParts = append(newParts, fmt.Sprintf("%s: %s", name, newValue))
		} else {
			newParts = append(newParts, fmt.Sprintf("%s: %s (updated)", name, newValue))
		}
		log.Printf("[Manager] Collected %s = %q (was %q)", key, newValue, oldVal)
	}

	return newParts
}

// buildSmartResponse 根据新收集的信息和缺失信息构建回复。
func (m *Manager) buildSmartResponse(newInfoParts []string, conv *models.Conversation) string {
	var sb strings.Builder
	missing := conv.GetMissingFields()

	// 第一次对话（没有提取到任何信息），发送欢迎消息
	if len(newInfoParts) == 0 && len(conv.Messages) <= 2 {
		sb.WriteString("您好，我是技术支持助手。/ Hi, I'm the tech support assistant.\n\n")
		sb.WriteString("📋 反馈问题，请提供以下信息 / To report an issue, please provide:\n")
		for _, name := range missing {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		// 展示可选字段提示
		for _, field := range models.OptionalFields {
			sb.WriteString(fmt.Sprintf("  - %s（可选 / optional）\n", field.Name))
		}
		sb.WriteString("\n💡 反馈建议，请直接发送 / To submit a suggestion, send:\n")
		sb.WriteString("  反馈：您的内容 / feedback: your content\n")
		sb.WriteString("  建议：您的内容 / suggestion: your content\n")
		sb.WriteString("\n您可以一次性告诉我，也可以分多次发送。\nYou can provide all info at once or send it in multiple messages.\n")
		sb.WriteString("如有日志文件，可直接发送附件。\nIf you have log files, feel free to send them as attachments.")
		return sb.String()
	}

	// 有新收集的信息
	if len(newInfoParts) > 0 {
		sb.WriteString("已记录 / Noted:\n")
		for _, part := range newInfoParts {
			sb.WriteString(fmt.Sprintf("  ✅ %s\n", part))
		}
		sb.WriteString("\n")
	}

	// 还有缺失信息
	if len(missing) > 0 {
		if len(newInfoParts) == 0 {
			// 用户发了消息但没有提取到新信息
			sb.WriteString("请继续提供以下信息 / Please provide the following info:\n")
		} else {
			sb.WriteString("还需要以下信息 / Still need:\n")
		}
		for _, name := range missing {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		sb.WriteString("\n回复「提交」或 \"submit\" 可直接提交当前信息。\nReply \"submit\" to submit current info directly.")
	}

	return sb.String()
}

// buildEscalateResponse 构建自动转人工的响应。
func (m *Manager) buildEscalateResponse(ctx context.Context, conv *models.Conversation) (string, error) {
	var sb strings.Builder
	if conv.Mode == models.ModeSuggestion {
		sb.WriteString("已收到您的反馈/建议，正在提交...\nYour feedback has been received, submitting...\n\n")
	} else {
		sb.WriteString("信息收集完毕！/ All info collected!\n\n")
	}
	sb.WriteString(conv.GetUserSummary())
	sb.WriteString("\n正在为您提交到技术支持团队... / Submitting to the support team...")

	userMsg := sb.String()
	conv.AddMessage("assistant", userMsg)

	if err := m.store.SaveConversation(ctx, conv); err != nil {
		return "", fmt.Errorf("failed to save conversation: %w", err)
	}

	return EscalatePrefix + userMsg, nil
}

// GetConversation 根据 chat ID 获取会话。
func (m *Manager) GetConversation(ctx context.Context, chatID string) (*models.Conversation, error) {
	return m.store.GetConversation(ctx, chatID)
}

// ClearConversation 从存储中清除会话。
func (m *Manager) ClearConversation(ctx context.Context, chatID string) error {
	return m.store.ClearConversation(ctx, chatID)
}

// Close 关闭管理器及其资源。
func (m *Manager) Close() error {
	return m.store.Close()
}
