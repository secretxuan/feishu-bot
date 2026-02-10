// Package models 提供公共数据结构。
package models

import (
	"fmt"
	"strings"
	"time"
)

// ConversationMode 表示会话模式。
type ConversationMode string

const (
	ModeUnknown    ConversationMode = ""           // 未确定模式
	ModeIssue      ConversationMode = "issue"      // 问题反馈模式
	ModeSuggestion ConversationMode = "suggestion" // 建议反馈模式
)

// Message 表示聊天消息。
type Message struct {
	Role      string `json:"role"` // "user" or "assistant"
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// FileInfo 表示用户上传的文件信息。
type FileInfo struct {
	MessageID string `json:"message_id"` // 原始消息ID（用于下载文件）
	FileKey   string `json:"file_key"`   // 文件的 fileKey
	FileName  string `json:"file_name"`  // 文件名
}

// RequiredFields 定义问题反馈模式需要收集的 11 项必填信息（中英双语）。
var RequiredFields = []struct {
	Key  string
	Name string
}{
	{"issue", "问题描述 / Issue Description"},
	{"occur_time", "发生时间 / Time of Occurrence"},
	{"reproducible", "是否必现 / Reproducible?"},
	{"vpn", "是否使用VPN / Using VPN?"},
	{"app_version", "应用版本 / App Version"},
	{"glasses_version", "眼镜版本 / Glasses Firmware"},
	{"glasses_sn", "眼镜SN号 / Glasses SN"},
	{"ring_version", "戒指版本 / Ring Firmware"},
	{"ring_sn", "戒指SN号 / Ring SN"},
	{"phone_model", "手机型号 / Phone Model"},
	{"phone_os", "手机系统版本 / Phone OS Version"},
}

// Conversation 表示用户会话。
type Conversation struct {
	ChatID     string    `json:"chat_id"`
	SenderID   string    `json:"sender_id"`
	SenderName string    `json:"sender_name"`
	Messages   []Message `json:"messages"`

	// 会话模式
	Mode ConversationMode `json:"mode,omitempty"` // issue / suggestion

	// 信息收集状态
	CollectedInfo map[string]string `json:"collected_info,omitempty"` // 已收集的信息
	Files         []FileInfo        `json:"files,omitempty"`          // 用户上传的文件列表

	// 建议内容（建议模式下使用）
	SuggestionText string `json:"suggestion_text,omitempty"`

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

// AddFile 添加用户上传的文件信息。
func (c *Conversation) AddFile(f FileInfo) {
	c.Files = append(c.Files, f)
	c.UpdatedAt = time.Now()
}

// HasFiles 检查是否有用户上传的文件。
func (c *Conversation) HasFiles() bool {
	return len(c.Files) > 0
}

// IsInfoComplete 检查是否所有必填信息都已收集（仅在问题反馈模式下使用）。
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
// 注意：用户ID 通过消息中的 @用户 实现，不再写在文本中。
func (c *Conversation) GetInfoSummary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("提交时间 / Submitted: %s\n", c.UpdatedAt.Format("2006-01-02 15:04:05")))

	if c.Mode == ModeSuggestion {
		sb.WriteString("类型 / Type: 建议反馈 / Suggestion\n\n")
		sb.WriteString(fmt.Sprintf("内容 / Content: %s\n", c.SuggestionText))
	} else {
		sb.WriteString("类型 / Type: 问题反馈 / Issue Report\n\n")
		for _, field := range RequiredFields {
			if val, ok := c.CollectedInfo[field.Key]; ok && val != "" {
				sb.WriteString(fmt.Sprintf("%s: %s\n", field.Name, val))
			} else {
				sb.WriteString(fmt.Sprintf("%s: 未提供 / Not provided\n", field.Name))
			}
		}
	}

	if c.HasFiles() {
		sb.WriteString("\n日志文件 / Log files: 已上传（见话题内附件）/ Uploaded (see attachments in thread)\n")
	}

	return sb.String()
}

// GetUserSummary 获取用于展示给用户的信息摘要。
func (c *Conversation) GetUserSummary() string {
	var sb strings.Builder

	if c.Mode == ModeSuggestion {
		sb.WriteString(fmt.Sprintf("- 建议内容 / Suggestion: %s\n", c.SuggestionText))
	} else {
		for _, field := range RequiredFields {
			if val, ok := c.CollectedInfo[field.Key]; ok && val != "" {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", field.Name, val))
			}
		}
	}
	if c.HasFiles() {
		sb.WriteString("- 日志文件 / Log files: 已上传 / Uploaded\n")
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
