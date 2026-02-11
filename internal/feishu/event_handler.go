// Package feishu 提供飞书事件处理器。
package feishu

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/even/feishu-bot/internal/conversation"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// MessageHandler 处理接收到的消息。
type MessageHandler interface {
	HandleMessage(ctx context.Context, chatID, senderID, messageID, content, msgType, fileKey string) error
	HandleEscalation(ctx context.Context, chatID, senderID, content string) error
}

// EventHandlers 保存事件处理器。
type EventHandlers struct {
	messageHandler MessageHandler
	feishuClient   *Client
	store          *conversation.Store
	chatLocks      sync.Map // map[chatID]*sync.Mutex — 防止同一会话并发处理
}

// NewEventHandlers 创建新的事件处理器实例。
func NewEventHandlers(handler MessageHandler, client *Client, store *conversation.Store) *EventHandlers {
	return &EventHandlers{
		messageHandler: handler,
		feishuClient:   client,
		store:          store,
	}
}

// SetFeishuClient 设置飞书客户端（用于解决循环依赖问题）。
func (e *EventHandlers) SetFeishuClient(client *Client) {
	e.feishuClient = client
}

// RegisterHandlers 注册所有事件处理器。
func (e *EventHandlers) RegisterHandlers() *dispatcher.EventDispatcher {
	return dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(e.handlePrivateMessage).
		OnP1P2PChatCreatedV1(e.handleP2PChatCreated)
}

// handleP2PChatCreated 处理用户首次打开机器人对话事件。
func (e *EventHandlers) handleP2PChatCreated(ctx context.Context, event *larkim.P1P2PChatCreatedV1) error {
	chatID := ""
	userID := ""
	if event.Event != nil {
		chatID = event.Event.ChatID
		if event.Event.User != nil {
			userID = event.Event.User.OpenId
		}
	}
	log.Printf("[Event] New p2p chat created: chatID=%s, userID=%s", chatID, userID)

	// 发送欢迎消息
	if chatID != "" && e.feishuClient != nil {
		welcome := "您好，我是技术支持助手。/ Hi, I'm the tech support assistant.\n\n" +
			"📋 反馈问题，请提供以下信息 / To report an issue, please provide:\n" +
			"  - 问题描述 / Issue Description\n" +
			"  - 发生时间 / Time of Occurrence\n" +
			"  - 是否必现 / Reproducible?\n" +
			"  - 应用版本 / App Version\n" +
			"  - 眼镜版本 / Glasses Firmware\n" +
			"  - 眼镜SN号 / Glasses SN\n" +
			"  - 戒指版本 / Ring Firmware\n" +
			"  - 戒指SN号 / Ring SN\n" +
			"  - 手机型号 / Phone Model\n" +
			"  - 手机系统版本 / Phone OS Version\n" +
			"  - 是否使用VPN / Using VPN?（可选 / optional）\n\n" +
			"💡 反馈建议，请直接发送 / To submit a suggestion, send:\n" +
			"  反馈：您的内容 / feedback: your content\n" +
			"  建议：您的内容 / suggestion: your content\n\n" +
			"您可以一次性告诉我，也可以分多次发送。\nYou can provide all info at once or send it in multiple messages.\n" +
			"如有日志文件，可直接发送附件。\nIf you have log files, feel free to send them as attachments."
		if err := e.feishuClient.SendTextMessage(ctx, chatID, welcome); err != nil {
			log.Printf("[Event] Failed to send welcome message: %v", err)
		}
	}

	return nil
}

// getChatLock 获取指定 chatID 的互斥锁（懒初始化）。
func (e *EventHandlers) getChatLock(chatID string) *sync.Mutex {
	val, _ := e.chatLocks.LoadOrStore(chatID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// handlePrivateMessage 处理私聊（P2P）消息事件。
func (e *EventHandlers) handlePrivateMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	// 提取基本信息
	chatID := ""
	senderID := ""
	messageID := ""
	chatType := ""
	msgType := ""

	if event.Event.Message.ChatId != nil {
		chatID = *event.Event.Message.ChatId
	}
	if event.Event.Sender.SenderId.OpenId != nil {
		senderID = *event.Event.Sender.SenderId.OpenId
	}
	if event.Event.Message.MessageId != nil {
		messageID = *event.Event.Message.MessageId
	}
	if event.Event.Message.ChatType != nil {
		chatType = *event.Event.Message.ChatType
	}
	if event.Event.Message.MessageType != nil {
		msgType = *event.Event.Message.MessageType
	}

	// ====== 原子性消息去重（关键修复） ======
	// 使用 Redis SETNX 实现原子的 check-and-set，
	// 彻底解决 WebSocket 重发导致的重复处理问题。
	if messageID != "" && e.store != nil {
		isNew, err := e.store.TryMarkMessageProcessed(ctx, messageID)
		if err != nil {
			log.Printf("[ERROR] Failed to check/mark message %s: %v", messageID, err)
			// 出错时保守处理：跳过此消息
			return nil
		}
		if !isNew {
			log.Printf("[DEDUP] Message %s already processed, skipping", messageID)
			return nil
		}
		log.Printf("[DEDUP] Message %s is new, processing", messageID)
	}

	// 只处理私聊消息
	if chatType != "p2p" {
		log.Printf("[Event] Ignoring non-p2p message (chatType=%s)", chatType)
		return nil
	}

	// 提取消息内容和文件信息
	content, fileKey := e.extractMessageInfo(event)

	log.Printf("[Event] ChatID=%s, Sender=%s, MsgID=%s, Type=%s, Content=%q, FileKey=%s",
		chatID, senderID, messageID, msgType, content, fileKey)

	// ====== 每个会话加锁，防止并发处理 ======
	// 确保同一个用户的消息串行处理，避免状态冲突
	mu := e.getChatLock(chatID)
	mu.Lock()
	defer mu.Unlock()

	// 委托给消息处理器
	if err := e.messageHandler.HandleMessage(ctx, chatID, senderID, messageID, content, msgType, fileKey); err != nil {
		log.Printf("[ERROR] HandleMessage failed: %v", err)
		return err
	}

	return nil
}

// extractMessageInfo 从消息中提取文本内容和文件信息。
func (e *EventHandlers) extractMessageInfo(event *larkim.P2MessageReceiveV1) (content string, fileKey string) {
	msg := event.Event.Message
	if msg == nil {
		return "", ""
	}

	msgType := ""
	if msg.MessageType != nil {
		msgType = *msg.MessageType
	}

	// 从 Content 字段提取
	if msg.Content != nil {
		contentStr := *msg.Content
		if contentStr != "" {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(contentStr), &data); err != nil {
				log.Printf("[Event] Failed to parse content JSON: %v", err)
			} else {
				switch msgType {
				case "text":
					if text, ok := data["text"].(string); ok {
						return text, ""
					}
				case "file":
					if fileKeyVal, ok := data["file_key"].(string); ok {
						if fileName, ok := data["file_name"].(string); ok {
							return "上传了文件: " + fileName, fileKeyVal
						}
						return "上传了文件", fileKeyVal
					}
				case "image":
					if imageKey, ok := data["image_key"].(string); ok {
						return "[图片]", imageKey
					}
				case "audio":
					if fileKeyVal, ok := data["file_key"].(string); ok {
						return "[语音]", fileKeyVal
					}
				case "media":
					if fileKeyVal, ok := data["file_key"].(string); ok {
						return "[视频]", fileKeyVal
					}
				case "sticker":
					return "[表情包]", ""
				default:
					if text, ok := data["text"].(string); ok {
						return text, ""
					}
				}
			}
		}
	}

	// 如果 Content 为空，尝试通过 API 获取
	if msg.MessageId != nil && e.feishuClient != nil {
		messageID := *msg.MessageId
		log.Printf("[Event] Content empty, fetching via API: %s", messageID)

		fullMsg, err := e.feishuClient.GetMessage(context.Background(), messageID)
		if err != nil {
			log.Printf("[Event] Failed to get message from API: %v", err)
			return "", ""
		}

		if fullMsg != nil && fullMsg.Body != nil && fullMsg.Body.Content != nil {
			contentStr := *fullMsg.Body.Content
			if contentStr != "" {
				var data map[string]interface{}
				if err := json.Unmarshal([]byte(contentStr), &data); err == nil {
					if fullMsg.MsgType != nil {
						msgType = *fullMsg.MsgType
					}
					switch msgType {
					case "text":
						if text, ok := data["text"].(string); ok {
							return text, ""
						}
					case "file":
						if fileKeyVal, ok := data["file_key"].(string); ok {
							if fileName, ok := data["file_name"].(string); ok {
								return "上传了文件: " + fileName, fileKeyVal
							}
							return "上传了文件", fileKeyVal
						}
					}
				}
			}
		}
	}

	return "", ""
}
