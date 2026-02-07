// Package models 提供公共数据结构。
package models

import (
	"fmt"
	"strings"
	"time"
)

// Message 表示聊天消息。
type Message struct {
	Role      string `json:"role"` // "user" or "assistant"
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// RequiredFields 定义需要收集的 4 项必填信息。
var RequiredFields = []struct {
	Key  string
	Name string
}{
	{"version", "版本信息"},
	{"device", "设备信息"},
	{"user", "用户信息"},
	{"issue", "问题描述"},
}

// Conversation 表示用户会话。
type Conversation struct {
	ChatID     string    `json:"chat_id"`
	SenderID   string    `json:"sender_id"`
	SenderName string    `json:"sender_name"`
	Messages   []Message `json:"messages"`

	// 信息收集状态
	CollectedInfo  map[string]string `json:"collected_info,omitempty"`   // 已收集的信息
	LogFileKey     string            `json:"log_file_key,omitempty"`     // 日志文件 fileKey
	FileMessageIDs []string          `json:"file_message_ids,omitempty"` // 文件消息ID列表，用于转发

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AddMessage 添加消息到会话。
func (c *Conversation) AddMessage(role, content string) {
	c.Messages = append(c.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Unix(),
	})
	c.UpdatedAt = time.Now()
}

// SetCollectedInfo 设置已收集的信息。
func (c *Conversation) SetCollectedInfo(key, value string) {
	if c.CollectedInfo == nil {
		c.CollectedInfo = make(map[string]string)
	}
	c.CollectedInfo[key] = value
	c.UpdatedAt = time.Now()
}

// GetCollectedInfo 获取已收集的信息。
func (c *Conversation) GetCollectedInfo(key string) (string, bool) {
	if c.CollectedInfo == nil {
		return "", false
	}
	val, ok := c.CollectedInfo[key]
	return val, ok
}

// SetLogFileKey 设置日志文件的 fileKey。
func (c *Conversation) SetLogFileKey(fileKey string) {
	c.LogFileKey = fileKey
	c.UpdatedAt = time.Now()
}

// HasLogFile 检查是否有日志文件。
func (c *Conversation) HasLogFile() bool {
	return c.LogFileKey != ""
}

// AddFileMessageID 添加文件消息ID（用于后续转发到群）。
func (c *Conversation) AddFileMessageID(messageID string) {
	c.FileMessageIDs = append(c.FileMessageIDs, messageID)
	c.UpdatedAt = time.Now()
}

// HasFileMessages 检查是否有文件消息。
func (c *Conversation) HasFileMessages() bool {
	return len(c.FileMessageIDs) > 0
}

// IsInfoComplete 检查是否所有必填信息都已收集。
func (c *Conversation) IsInfoComplete() bool {
	if c.CollectedInfo == nil {
		return false
	}
	for _, field := range RequiredFields {
		if val, ok := c.CollectedInfo[field.Key]; !ok || val == "" {
			return false
		}
	}
	return true
}

// GetMissingFields 获取缺失的必填信息列表（返回显示名称）。
func (c *Conversation) GetMissingFields() []string {
	var missing []string
	for _, field := range RequiredFields {
		if c.CollectedInfo == nil {
			missing = append(missing, field.Name)
			continue
		}
		if val, ok := c.CollectedInfo[field.Key]; !ok || val == "" {
			missing = append(missing, field.Name)
		}
	}
	return missing
}

// GetInfoSummary 获取已收集信息的总结（用于发送到群组）。
func (c *Conversation) GetInfoSummary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("用户ID: %s\n", c.SenderID))
	if c.SenderName != "" {
		sb.WriteString(fmt.Sprintf("用户名称: %s\n", c.SenderName))
	}
	sb.WriteString(fmt.Sprintf("提交时间: %s\n\n", c.UpdatedAt.Format("2006-01-02 15:04:05")))

	for _, field := range RequiredFields {
		if val, ok := c.CollectedInfo[field.Key]; ok && val != "" {
			sb.WriteString(fmt.Sprintf("%s: %s\n", field.Name, val))
		} else {
			sb.WriteString(fmt.Sprintf("%s: 未提供\n", field.Name))
		}
	}

	if c.HasLogFile() || c.HasFileMessages() {
		sb.WriteString("\n日志文件: 已上传（见附件）\n")
	}

	return sb.String()
}

// GetUserSummary 获取用于展示给用户的信息摘要。
func (c *Conversation) GetUserSummary() string {
	var sb strings.Builder

	for _, field := range RequiredFields {
		if val, ok := c.CollectedInfo[field.Key]; ok && val != "" {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", field.Name, val))
		}
	}
	if c.HasLogFile() || c.HasFileMessages() {
		sb.WriteString("- 日志文件: 已上传\n")
	}

	return sb.String()
}

// GetMessagesForLLM 返回最后 N 条消息。
func (c *Conversation) GetMessagesForLLM(maxCount int) []Message {
	if len(c.Messages) <= maxCount {
		return c.Messages
	}
	return c.Messages[len(c.Messages)-maxCount:]
}

// EscalationRequest 表示转人工请求。
type EscalationRequest struct {
	Conversation *Conversation
	Reason       string
	FileKey      string
	Timestamp    time.Time
}

// FeishuMessage 表示飞书消息内容。
type FeishuMessage struct {
	ChatID    string
	MessageID string
	Content   string
	SenderID  string
	ChatType  string // "p2p", "group", etc.
	MsgType   string // "text", "file", "image", etc.
	FileKey   string // 文件消息的 fileKey
}

// BotStatus 表示机器人当前状态。
type BotStatus struct {
	IsConnected     bool
	StartTime       time.Time
	MessageCount    int64
	EscalationCount int64
}
