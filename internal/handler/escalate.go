// Package handler 提供转人工处理逻辑。
package handler

import (
	"context"
	"log"

	"github.com/even/feishu-bot/internal/feishu"
	"github.com/even/feishu-bot/pkg/models"
)

// EscalationHandler 处理转人工服务。
type EscalationHandler struct {
	feishuClient      *feishu.Client
	escalationGroupID string
}

// NewEscalationHandler 创建新的转人工处理器。
func NewEscalationHandler(client *feishu.Client, escalationGroupID string) *EscalationHandler {
	return &EscalationHandler{
		feishuClient:      client,
		escalationGroupID: escalationGroupID,
	}
}

// HandleEscalation 处理转人工请求：发送摘要到群组，转发文件，通知用户。
func (h *EscalationHandler) HandleEscalation(ctx context.Context, conv *models.Conversation) error {
	log.Printf("[Escalation] Processing for chat %s, user %s", conv.ChatID, conv.SenderID)

	// 1. 构建并发送摘要到群组
	summary := conv.GetInfoSummary()
	log.Printf("[Escalation] Sending summary to group %s", h.escalationGroupID)

	if err := h.feishuClient.SendPostMessage(ctx, h.escalationGroupID, "用户支持请求", summary); err != nil {
		log.Printf("[Escalation] Failed to send summary: %v", err)
		return err
	}
	log.Printf("[Escalation] Summary sent successfully")

	// 2. 转发文件消息到群组
	fileForwarded := false
	for _, msgID := range conv.FileMessageIDs {
		log.Printf("[Escalation] Forwarding file message %s to group", msgID)
		if err := h.feishuClient.ForwardMessage(ctx, msgID, h.escalationGroupID); err != nil {
			log.Printf("[Escalation] Forward failed for %s: %v", msgID, err)
			// 转发失败不影响整体流程
		} else {
			log.Printf("[Escalation] File message %s forwarded successfully", msgID)
			fileForwarded = true
		}
	}

	// 如果没有成功转发任何消息但有 fileKey，尝试直接发送文件
	if !fileForwarded && conv.LogFileKey != "" {
		log.Printf("[Escalation] Trying to send file directly with fileKey: %s", conv.LogFileKey)
		if err := h.feishuClient.SendFileMessage(ctx, h.escalationGroupID, conv.LogFileKey); err != nil {
			log.Printf("[Escalation] Direct file send also failed: %v", err)
		} else {
			log.Printf("[Escalation] File sent directly via fileKey")
		}
	}

	// 3. 通知用户
	userMsg := "✅ 您的问题已提交给技术支持团队，我们会尽快处理！"
	if err := h.feishuClient.SendTextMessage(ctx, conv.ChatID, userMsg); err != nil {
		log.Printf("[Escalation] Failed to notify user: %v", err)
	}

	log.Printf("[Escalation] Completed for chat %s", conv.ChatID)
	return nil
}

// GetEscalationGroupID 返回转人工群组 ID。
func (h *EscalationHandler) GetEscalationGroupID() string {
	return h.escalationGroupID
}

// SetFeishuClient 设置飞书客户端（初始化时使用）。
func (h *EscalationHandler) SetFeishuClient(client *feishu.Client) {
	h.feishuClient = client
}
