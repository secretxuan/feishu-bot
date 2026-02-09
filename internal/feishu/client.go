// Package feishu 提供飞书（Lark）API 客户端。
package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// Client 封装飞书 SDK 客户端。
type Client struct {
	wsClient  *larkws.Client
	larkCli   *lark.Client
	appID     string
	appSecret string
}

// NewClient 创建新的飞书客户端。
func NewClient(appID, appSecret string, eventHandler *dispatcher.EventDispatcher) *Client {
	wsClient := larkws.NewClient(
		appID,
		appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	larkCli := lark.NewClient(appID, appSecret)

	return &Client{
		wsClient:  wsClient,
		larkCli:   larkCli,
		appID:     appID,
		appSecret: appSecret,
	}
}

// Start 启动 WebSocket 连接。
func (c *Client) Start(ctx context.Context) error {
	return c.wsClient.Start(ctx)
}

// SendTextMessage 发送文本消息到指定聊天。
func (c *Client) SendTextMessage(ctx context.Context, chatID, text string) error {
	log.Printf("[Feishu] SendTextMessage: chatID=%s, text=%q", chatID, truncate(text, 100))

	content := fmt.Sprintf(`{"text":"%s"}`, escapeJSON(text))

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(content).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Create(ctx, req)
	if err != nil {
		log.Printf("[ERROR] SendTextMessage failed: %v", err)
		return fmt.Errorf("failed to send message: %w", err)
	}

	if !resp.Success() {
		log.Printf("[ERROR] SendTextMessage API error: code=%d, msg=%s", resp.Code, resp.Msg)
		return fmt.Errorf("send message failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	log.Printf("[Feishu] Message sent successfully")
	return nil
}

// escapeJSON 转义 JSON 字符串中的特殊字符。
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// ReplyMessage 回复消息。
func (c *Client) ReplyMessage(ctx context.Context, messageID, text string) error {
	content := larkim.NewMessageTextBuilder().
		Text(text).
		Build()

	req := larkim.NewReplyMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			Content(content).
			MsgType(larkim.MsgTypeText).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Reply(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to reply message: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("reply failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// UploadFile 上传文件到飞书。
func (c *Client) UploadFile(ctx context.Context, filename string, file io.Reader) (string, error) {
	req := larkim.NewCreateFileReqBuilder().
		Body(larkim.NewCreateFileReqBodyBuilder().
			FileType(larkim.FileTypeStream).
			FileName(filename).
			File(file).
			Build()).
		Build()

	resp, err := c.larkCli.Im.File.Create(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("file upload failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return *resp.Data.FileKey, nil
}

// SendFileMessage 发送文件消息到指定聊天。
func (c *Client) SendFileMessage(ctx context.Context, chatID, fileKey string) error {
	log.Printf("[Feishu] SendFileMessage: chatID=%s, fileKey=%s", chatID, fileKey)

	content := larkim.MessageFile{
		FileKey: fileKey,
	}
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("failed to marshal file content: %w", err)
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeFile).
			Content(string(contentBytes)).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send file message: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("send file failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	log.Printf("[Feishu] File message sent successfully")
	return nil
}

// ForwardMessage 转发消息到目标聊天（用于转发文件到群组）。
func (c *Client) ForwardMessage(ctx context.Context, messageID, targetChatID string) error {
	log.Printf("[Feishu] ForwardMessage: msgID=%s, target=%s", messageID, targetChatID)

	req := larkim.NewForwardMessageReqBuilder().
		MessageId(messageID).
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewForwardMessageReqBodyBuilder().
			ReceiveId(targetChatID).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Forward(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to forward message: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("forward message failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	log.Printf("[Feishu] Message forwarded successfully")
	return nil
}

// SendPostMessage 发送富文本（post）消息到指定聊天，返回消息ID（用于话题内回复）。
func (c *Client) SendPostMessage(ctx context.Context, chatID, title, textContent string) (string, error) {
	log.Printf("[Feishu] SendPostMessage: chatID=%s, title=%s", chatID, title)

	postContent := map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"title": title,
			"content": [][]map[string]interface{}{
				{
					{
						"tag":  "text",
						"text": textContent,
					},
				},
			},
		},
	}

	contentBytes, err := json.Marshal(postContent)
	if err != nil {
		return "", fmt.Errorf("failed to marshal post content: %w", err)
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypePost).
			Content(string(contentBytes)).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Create(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to send post message: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("send post failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	// 提取消息ID，用于后续在同一话题内回复文件
	msgID := ""
	if resp.Data != nil && resp.Data.MessageId != nil {
		msgID = *resp.Data.MessageId
	}

	log.Printf("[Feishu] Post message sent successfully, msgID=%s", msgID)
	return msgID, nil
}

// ReplyFileInThread 在话题内回复文件（将文件放入与摘要同一话题中）。
func (c *Client) ReplyFileInThread(ctx context.Context, parentMsgID, fileKey string) error {
	log.Printf("[Feishu] ReplyFileInThread: parentMsg=%s, fileKey=%s", parentMsgID, fileKey)

	content := fmt.Sprintf(`{"file_key":"%s"}`, fileKey)

	req := larkim.NewReplyMessageReqBuilder().
		MessageId(parentMsgID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			Content(content).
			MsgType(larkim.MsgTypeFile).
			ReplyInThread(true).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Reply(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to reply file in thread: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("reply file in thread failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	log.Printf("[Feishu] File replied in thread successfully")
	return nil
}

// DownloadMessageResource 下载消息中的文件资源（返回文件数据和文件名）。
func (c *Client) DownloadMessageResource(ctx context.Context, messageID, fileKey, resourceType string) ([]byte, string, error) {
	log.Printf("[Feishu] DownloadMessageResource: msgID=%s, fileKey=%s, type=%s", messageID, fileKey, resourceType)

	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(fileKey).
		Type(resourceType).
		Build()

	resp, err := c.larkCli.Im.MessageResource.Get(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download resource: %w", err)
	}

	if !resp.Success() {
		return nil, "", fmt.Errorf("download resource failed: code=%d", resp.Code)
	}

	// 读取文件内容
	data, err := io.ReadAll(resp.File)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read resource data: %w", err)
	}

	fileName := resp.FileName
	log.Printf("[Feishu] Downloaded resource: %d bytes, fileName=%s", len(data), fileName)
	return data, fileName, nil
}

// GetMessage 获取消息详情。
func (c *Client) GetMessage(ctx context.Context, messageID string) (*larkim.Message, error) {
	req := larkim.NewGetMessageReqBuilder().
		MessageId(messageID).
		Build()

	resp, err := c.larkCli.Im.Message.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("get message failed: code=%d", resp.Code)
	}

	if len(resp.Data.Items) == 0 {
		return nil, fmt.Errorf("no message found in response")
	}

	return resp.Data.Items[0], nil
}

// GetWSClient 返回底层的 WebSocket 客户端。
func (c *Client) GetWSClient() *larkws.Client {
	return c.wsClient
}

// GetAppID 返回应用 ID。
func (c *Client) GetAppID() string {
	return c.appID
}

// GetAppSecret 返回应用密钥。
func (c *Client) GetAppSecret() string {
	return c.appSecret
}

// truncate 截断字符串用于日志输出。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
