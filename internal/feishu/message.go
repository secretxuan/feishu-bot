// Package feishu 提供飞书消息相关工具。
package feishu

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/even/feishu-bot/pkg/models"
)

// MessageBuilder 帮助构建不同类型的飞书消息。
type MessageBuilder struct{}

// NewMessageBuilder 创建新的消息构建器。
func NewMessageBuilder() *MessageBuilder {
	return &MessageBuilder{}
}

// BuildEscalationMessage 构建转人工消息内容。
func (b *MessageBuilder) BuildEscalationMessage(conv *models.Conversation) string {
	var sb strings.Builder

	sb.WriteString("**用户请求转人工**\n\n")
	sb.WriteString(fmt.Sprintf("用户ID: %s\n", conv.SenderID))
	if conv.SenderName != "" {
		sb.WriteString(fmt.Sprintf("用户名称: %s\n", conv.SenderName))
	}

	// 从会话中获取问题摘要
	summary := b.getTopicSummary(conv)
	sb.WriteString(fmt.Sprintf("问题摘要: %s\n", summary))

	sb.WriteString(fmt.Sprintf("\n对话共 %d 轮，详细日志已上传。", len(conv.Messages)))

	return sb.String()
}

// getTopicSummary 从会话中提取问题摘要。
func (b *MessageBuilder) getTopicSummary(conv *models.Conversation) string {
	if len(conv.Messages) == 0 {
		return "无对话记录"
	}

	// 获取第一条用户消息作为主题
	for _, msg := range conv.Messages {
		if msg.Role == "user" {
			// 如果太长则截断
			maxLen := 100
			if len(msg.Content) <= maxLen {
				return msg.Content
			}
			return msg.Content[:maxLen] + "..."
		}
	}

	return "用户咨询"
}

// CreateLogContent 从会话创建日志内容字符串。
func CreateLogContent(conv *models.Conversation) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("会话ID: %s\n", conv.ChatID))
	sb.WriteString(fmt.Sprintf("用户ID: %s\n", conv.SenderID))
	sb.WriteString(fmt.Sprintf("用户名称: %s\n", conv.SenderName))
	sb.WriteString(fmt.Sprintf("开始时间: %s\n", conv.CreatedAt.Format("2006-01-02 15:04:05")))

	// 获取消息数量
	msgCount := len(conv.Messages)
	sb.WriteString(fmt.Sprintf("消息数量: %d\n", msgCount))

	sb.WriteString("\n===== 对话记录 =====\n\n")

	for i, msg := range conv.Messages {
		role := "用户"
		if msg.Role == "assistant" {
			role = "助手"
		}
		sb.WriteString(fmt.Sprintf("[%d] [%s] %s\n", i+1, role, msg.Content))
		sb.WriteString("\n")
	}

	return sb.String()
}

// UploadLogContent 上传日志内容作为文件。
func UploadLogContent(ctx context.Context, client *Client, conv *models.Conversation) (string, error) {
	content := CreateLogContent(conv)
	return createAndUploadLogWithClient(ctx, client, content, conv.ChatID)
}

// createAndUploadLogWithClient 使用客户端创建并上传日志文件。
func createAndUploadLogWithClient(ctx context.Context, client *Client, content string, chatID string) (string, error) {
	// 直接上传文本文件，不压缩
	filename := fmt.Sprintf("conversation_%s.log", chatID)

	// 使用 bytes.Reader 从内存中读取
	reader := NewBytesReader([]byte(content))

	return client.UploadFile(ctx, filename, reader)
}

// BytesReader 从字节切片创建 io.Reader。
type BytesReader struct {
	*bytes.Reader
}

// NewBytesReader 创建新的 BytesReader。
func NewBytesReader(data []byte) *BytesReader {
	return &BytesReader{
		Reader: bytes.NewReader(data),
	}
}
