// Package conversation 提供使用 Redis 的会话存储。
package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/even/feishu-bot/pkg/models"
	"github.com/redis/go-redis/v9"
)

const (
	// ConversationKeyPrefix 是 Redis 中会话键的前缀。
	ConversationKeyPrefix = "feishu:conv:"
	// ProcessedMessagesKeyPrefix 是 Redis 中已处理消息ID键的前缀。
	ProcessedMessagesKeyPrefix = "feishu:processed:"
)

// Store 使用 Redis 处理会话持久化。
type Store struct {
	client     *redis.Client
	expiration time.Duration
}

// NewStore 创建新的 Redis 支持的会话存储。
func NewStore(addr, password string, db int, expiration int) (*Store, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Store{
		client:     rdb,
		expiration: time.Duration(expiration) * time.Second,
	}, nil
}

// SaveConversation 将会话保存到 Redis。
func (s *Store) SaveConversation(ctx context.Context, conv *models.Conversation) error {
	key := s.conversationKey(conv.ChatID)

	data, err := json.Marshal(conv)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation: %w", err)
	}

	if err := s.client.Set(ctx, key, data, s.expiration).Err(); err != nil {
		return fmt.Errorf("failed to save conversation: %w", err)
	}

	return nil
}

// GetConversation 从 Redis 获取会话。
func (s *Store) GetConversation(ctx context.Context, chatID string) (*models.Conversation, error) {
	key := s.conversationKey(chatID)

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	var conv models.Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, fmt.Errorf("failed to unmarshal conversation: %w", err)
	}

	return &conv, nil
}

// GetOrCreateConversation 获取现有会话或创建新会话。
func (s *Store) GetOrCreateConversation(ctx context.Context, chatID, senderID, senderName string) (*models.Conversation, error) {
	conv, err := s.GetConversation(ctx, chatID)
	if err != nil {
		return nil, err
	}

	if conv != nil {
		if conv.SenderID != senderID {
			conv.SenderID = senderID
		}
		if senderName != "" && conv.SenderName != senderName {
			conv.SenderName = senderName
		}
		return conv, nil
	}

	// 创建新会话
	now := time.Now()
	conv = &models.Conversation{
		ChatID:        chatID,
		SenderID:      senderID,
		SenderName:    senderName,
		Messages:      []models.Message{},
		CollectedInfo: make(map[string]string),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	return conv, nil
}

// ClearConversation 从 Redis 中删除会话。
func (s *Store) ClearConversation(ctx context.Context, chatID string) error {
	key := s.conversationKey(chatID)

	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to clear conversation: %w", err)
	}

	return nil
}

// TryMarkMessageProcessed 原子性地检查并标记消息为已处理。
// 使用 SETNX（SetNX）实现原子操作，避免竞态条件。
// 返回 true 表示消息是新的（首次标记成功），false 表示消息已处理过。
func (s *Store) TryMarkMessageProcessed(ctx context.Context, messageID string) (bool, error) {
	key := s.processedMessageKey(messageID)
	// SETNX：只在 key 不存在时设置成功，是原子操作
	ok, err := s.client.SetNX(ctx, key, "1", 24*time.Hour).Result()
	if err != nil {
		return false, fmt.Errorf("failed to mark message as processed: %w", err)
	}
	return ok, nil
}

// Close 关闭 Redis 连接。
func (s *Store) Close() error {
	return s.client.Close()
}

// conversationKey 返回会话的 Redis 键。
func (s *Store) conversationKey(chatID string) string {
	return ConversationKeyPrefix + chatID
}

// Client 返回底层的 Redis 客户端。
func (s *Store) Client() *redis.Client {
	return s.client
}

// processedMessageKey 返回已处理消息的 Redis 键。
func (s *Store) processedMessageKey(messageID string) string {
	return ProcessedMessagesKeyPrefix + messageID
}
