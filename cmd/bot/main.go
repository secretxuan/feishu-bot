// Package main 是飞书机器人的入口。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/even/feishu-bot/internal/config"
	"github.com/even/feishu-bot/internal/conversation"
	"github.com/even/feishu-bot/internal/feishu"
	"github.com/even/feishu-bot/internal/handler"
	"github.com/even/feishu-bot/internal/llm"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	log.Printf("Starting Feishu Bot v%s (built at %s)", version, buildTime)

	// 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 初始化 Redis 存储
	store, err := conversation.NewStore(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.Expiration,
	)
	if err != nil {
		log.Fatalf("Failed to initialize Redis store: %v", err)
	}
	defer store.Close()

	promptMgr, err := conversation.NewPromptManager("configs/prompts.yaml")
	if err != nil {
		log.Printf("Warning: failed to load prompts from file, using defaults: %v", err)
		promptMgr, _ = conversation.NewPromptManager("")
	}

	llmClient, err := llm.NewOpenAICompatibleClient(
		cfg.LLM.BaseURL,
		cfg.LLM.APIKey,
		cfg.LLM.Model,
	)
	if err != nil {
		log.Printf("Warning: failed to initialize LLM client, running without LLM: %v", err)
		llmClient = nil
	}

	convMgr := conversation.NewManager(store, llmClient, promptMgr)

	// 初始化转人工处理器
	escalationHandler := handler.NewEscalationHandler(
		nil, // 稍后设置
		cfg.Feishu.EscalationGroupID,
	)

	// 创建包装器连接转人工处理器和事件处理器
	wrappedHandler := &wrappedMessageHandler{
		conversationManager: convMgr,
		escalationHandler:   escalationHandler,
		feishuClient:        nil, // 稍后设置
		cfg:                 cfg,
	}

	// 初始化飞书事件处理器
	feishuHandlers := feishu.NewEventHandlers(wrappedHandler, nil, store)

	// 初始化飞书客户端
	feishuClient := feishu.NewClient(
		cfg.Feishu.AppID,
		cfg.Feishu.AppSecret,
		feishuHandlers.RegisterHandlers(),
	)

	// 更新处理器的飞书客户端
	wrappedHandler.feishuClient = feishuClient
	escalationHandler.SetFeishuClient(feishuClient)
	feishuHandlers.SetFeishuClient(feishuClient)

	// 创建关闭上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理关闭信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在 goroutine 中启动飞书 WebSocket 连接
	errChan := make(chan error, 1)
	go func() {
		log.Println("Starting Feishu WebSocket connection...")
		if err := feishuClient.Start(ctx); err != nil {
			errChan <- fmt.Errorf("Feishu client error: %w", err)
		}
	}()

	log.Println("Feishu Bot is running. Press Ctrl+C to stop.")

	// 等待关闭信号或错误
	select {
	case <-sigChan:
		log.Println("Shutting down...")
		cancel()
	case err := <-errChan:
		log.Printf("Error: %v", err)
		cancel()
	}

	log.Println("Feishu Bot stopped.")
}

// wrappedMessageHandler 包装消息处理逻辑。
type wrappedMessageHandler struct {
	conversationManager *conversation.Manager
	escalationHandler   *handler.EscalationHandler
	feishuClient        *feishu.Client
	cfg                 *config.Config
}

// HandleMessage 处理用户消息。
func (h *wrappedMessageHandler) HandleMessage(ctx context.Context, chatID, senderID, messageID, content, msgType, fileKey string) error {
	// 检查是否需要清除上下文
	if msgType == "text" && h.cfg.IsClearContextKeyword(content) {
		return h.handleClearContext(ctx, chatID)
	}

	// 检查是否需要转人工（用户主动触发）
	if msgType == "text" && h.cfg.IsEscalationKeyword(content) {
		return h.HandleEscalation(ctx, chatID, senderID, content)
	}

	// 处理消息并获取回复
	response, err := h.conversationManager.ProcessMessage(ctx, chatID, senderID, "", content, msgType, fileKey, messageID)
	if err != nil {
		log.Printf("[Handler] ProcessMessage failed: %v", err)
		_ = h.feishuClient.ReplyMessage(ctx, messageID, "抱歉，处理您的消息时出错了，请稍后重试。")
		return err
	}

	// 检查是否需要自动转人工（信息收集完毕）
	if strings.HasPrefix(response, conversation.EscalatePrefix) {
		userMsg := strings.TrimPrefix(response, conversation.EscalatePrefix)
		if userMsg != "" {
			_ = h.feishuClient.SendTextMessage(ctx, chatID, userMsg)
		}
		return h.doEscalation(ctx, chatID, senderID)
	}

	// 发送普通回复
	if response != "" {
		if err := h.feishuClient.SendTextMessage(ctx, chatID, response); err != nil {
			log.Printf("[Handler] Failed to send response: %v", err)
			return err
		}
	}
	return nil
}

// HandleEscalation 处理用户主动转人工请求。
func (h *wrappedMessageHandler) HandleEscalation(ctx context.Context, chatID, senderID, content string) error {
	log.Printf("[Escalation] User %s requested escalation in chat %s", senderID, chatID)
	return h.doEscalation(ctx, chatID, senderID)
}

// doEscalation 执行转人工操作。
func (h *wrappedMessageHandler) doEscalation(ctx context.Context, chatID, senderID string) error {
	conv, err := h.conversationManager.GetConversation(ctx, chatID)
	if err != nil {
		log.Printf("[Escalation] Failed to get conversation: %v", err)
		_ = h.feishuClient.SendTextMessage(ctx, chatID, "抱歉，获取会话信息失败，请稍后重试。")
		return err
	}

	if conv == nil {
		_ = h.feishuClient.SendTextMessage(ctx, chatID, "请先描述您的问题，我会帮您收集必要信息。")
		return nil
	}

	// 直接执行转人工，不再重新检查 LLM
	if err := h.escalationHandler.HandleEscalation(ctx, conv); err != nil {
		log.Printf("[Escalation] HandleEscalation failed: %v", err)
		_ = h.feishuClient.SendTextMessage(ctx, chatID, "提交失败，请稍后重试。")
		return err
	}

	// 转人工成功后清除会话
	_ = h.conversationManager.ClearConversation(ctx, chatID)
	log.Printf("[Escalation] Completed and conversation cleared for chat %s", chatID)

	return nil
}

// handleClearContext 清除会话上下文。
func (h *wrappedMessageHandler) handleClearContext(ctx context.Context, chatID string) error {
	log.Printf("[Clear] Clearing context for chat %s", chatID)

	if err := h.conversationManager.ClearConversation(ctx, chatID); err != nil {
		log.Printf("[Clear] Failed: %v", err)
		_ = h.feishuClient.SendTextMessage(ctx, chatID, "抱歉，清除上下文时出错了。")
		return err
	}

	return h.feishuClient.SendTextMessage(ctx, chatID, "上下文已清除，请重新开始描述您的问题。")
}
