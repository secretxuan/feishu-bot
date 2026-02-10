// Package handler 提供转人工处理逻辑。
package handler

import (
	"bytes"
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

// HandleEscalation 处理转人工请求：邀请用户入群 → 发送摘要（@用户）→ 话题内回复文件 → 通知用户。
func (h *EscalationHandler) HandleEscalation(ctx context.Context, conv *models.Conversation) error {
	log.Printf("[Escalation] Processing for chat %s, user %s", conv.ChatID, conv.SenderID)

	// 1. 邀请用户到技术支持群
	if conv.SenderID != "" {
		log.Printf("[Escalation] Inviting user %s to group %s", conv.SenderID, h.escalationGroupID)
		if err := h.feishuClient.InviteUserToChat(ctx, h.escalationGroupID, conv.SenderID); err != nil {
			log.Printf("[Escalation] Failed to invite user (may already be in group): %v", err)
			// 邀请失败不阻塞流程（用户可能已在群中）
		}
	}

	// 2. 发送摘要到群组（创建话题根消息），并 @用户
	summary := conv.GetInfoSummary()
	log.Printf("[Escalation] Sending summary to group %s with @user %s", h.escalationGroupID, conv.SenderID)

	// 根据模式选择标题
	title := "用户问题反馈 / User Issue Report"
	if conv.Mode == models.ModeSuggestion {
		title = "用户建议反馈 / User Suggestion"
	}

	rootMsgID, err := h.feishuClient.SendPostMessage(ctx, h.escalationGroupID, title, summary, conv.SenderID)
	if err != nil {
		log.Printf("[Escalation] Failed to send summary: %v", err)
		return err
	}
	log.Printf("[Escalation] Summary sent, rootMsgID=%s", rootMsgID)

	// 3. 在同一话题内回复文件（下载 → 重新上传 → 话题内回复）
	if conv.HasFiles() && rootMsgID != "" {
		for _, f := range conv.Files {
			if err := h.forwardFileInThread(ctx, rootMsgID, f); err != nil {
				log.Printf("[Escalation] File thread reply failed for %s: %v", f.FileName, err)
				// 单个文件失败不影响整体流程，继续处理下一个文件
			}
		}
	}

	// 4. 通知用户
	userMsg := "✅ 您的问题已提交给技术支持团队，我们会尽快处理！\nYour issue has been submitted to the support team. We'll handle it ASAP!\n\n您已被邀请到技术支持群，可以在群里直接跟进问题。\nYou've been invited to the support group where you can follow up directly."
	if err := h.feishuClient.SendTextMessage(ctx, conv.ChatID, userMsg); err != nil {
		log.Printf("[Escalation] Failed to notify user: %v", err)
	}

	log.Printf("[Escalation] Completed for chat %s", conv.ChatID)
	return nil
}

// forwardFileInThread 将用户上传的文件下载后重新上传，然后在话题内回复。
func (h *EscalationHandler) forwardFileInThread(ctx context.Context, rootMsgID string, f models.FileInfo) error {
	log.Printf("[Escalation] Forwarding file %s in thread (parentMsg=%s)", f.FileName, rootMsgID)

	// Step 1: 从原始消息下载文件
	data, downloadedName, err := h.feishuClient.DownloadMessageResource(ctx, f.MessageID, f.FileKey, "file")
	if err != nil {
		return err
	}

	// 使用下载到的文件名，如果为空则用记录的文件名
	fileName := downloadedName
	if fileName == "" {
		fileName = f.FileName
	}
	if fileName == "" {
		fileName = "attachment"
	}

	log.Printf("[Escalation] Downloaded file: %s (%d bytes)", fileName, len(data))

	// Step 2: 重新上传文件获取新的 fileKey
	newFileKey, err := h.feishuClient.UploadFile(ctx, fileName, bytes.NewReader(data))
	if err != nil {
		return err
	}
	log.Printf("[Escalation] Re-uploaded file, new fileKey=%s", newFileKey)

	// Step 3: 在话题内回复文件
	if err := h.feishuClient.ReplyFileInThread(ctx, rootMsgID, newFileKey); err != nil {
		return err
	}

	log.Printf("[Escalation] File %s replied in thread successfully", fileName)
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
