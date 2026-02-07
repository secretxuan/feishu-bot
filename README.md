# 飞书信息收集机器人

基于 Go 开发的飞书企业自建应用机器人。通过多轮对话收集用户的**版本信息、设备信息、用户信息、问题描述**，收集完毕后自动提交到技术支持群，同时转发用户上传的日志文件。

## 功能特性

- **多轮信息收集** — LLM 从每条消息中提取信息，逐步累积，不重复追问
- **自动提交** — 4 项必填信息收集齐后自动发送到技术支持群
- **文件转发** — 用户上传的日志文件通过飞书消息转发 API 原样转发到群
- **消息去重** — Redis SETNX 原子操作 + 会话级互斥锁，杜绝重复回复
- **手动转人工** — 用户可随时发送「转人工」强制提交，无论信息是否完整
- **WebSocket 长连接** — 实时接收飞书消息事件
- **Redis 会话存储** — 会话状态持久化，支持过期自动清理

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.21+ |
| 飞书 SDK | [larksuite/oapi-sdk-go/v3](https://github.com/larksuite/oapi-sdk-go) |
| LLM | OpenAI 兼容 API（智谱 GLM-4-Flash） |
| 存储 | Redis |
| 部署 | Docker Compose |

## 系统架构

```
飞书用户（私聊）
    │
    │ 发送文本 / 文件消息
    ▼
┌──────────────────────────────────────────────────────────┐
│                    飞书开放平台                            │
│        WebSocket 长连接 (im.message.receive_v1)          │
└──────────────────────┬───────────────────────────────────┘
                       │ P2MessageReceiveV1 事件
                       ▼
┌──────────────────────────────────────────────────────────┐
│                   feishu-bot (Go)                         │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │           EventHandlers (event_handler.go)         │  │
│  │                                                    │  │
│  │  1. 提取 chatID / senderID / messageID / content   │  │
│  │  2. SETNX 原子去重 ── 重复消息直接丢弃             │  │
│  │  3. 过滤非 p2p 消息                                │  │
│  │  4. 获取 per-chat Mutex 加锁（串行化同一会话）      │  │
│  │  5. 提取消息内容 + 文件信息                         │  │
│  │  6. 调用 MessageHandler                            │  │
│  └───────────────────────┬────────────────────────────┘  │
│                          │                               │
│  ┌───────────────────────▼────────────────────────────┐  │
│  │        wrappedMessageHandler (main.go)              │  │
│  │                                                    │  │
│  │  路由判断：                                         │  │
│  │  ├─ "清空" 类关键词 → 清除会话上下文                │  │
│  │  ├─ "转人工" 类关键词 → 强制转人工                  │  │
│  │  └─ 其他消息 → ProcessMessage                      │  │
│  │                                                    │  │
│  │  处理 ProcessMessage 返回值：                       │  │
│  │  ├─ "ESCALATE:..." → 发摘要给用户 + 执行转人工      │  │
│  │  └─ 普通文本 → 发送给用户                           │  │
│  └───────────────────────┬────────────────────────────┘  │
│                          │                               │
│        ┌─────────────────┴──────────────────┐            │
│        ▼                                    ▼            │
│  ┌──────────────┐                    ┌──────────────┐    │
│  │ 文本消息处理  │                    │ 文件消息处理  │    │
│  │              │                    │              │    │
│  │ 1.添加到对话  │                    │ 1.存 fileKey  │    │
│  │   历史       │                    │ 2.存 msgID    │    │
│  │ 2.LLM 提取   │                    │   (转发用)    │    │
│  │   当前消息    │                    │ 3.添加到对话   │    │
│  │   的信息     │                    │   历史        │    │
│  │ 3.合并到     │                    │ 4.检查完整性   │    │
│  │   CollectedInfo                   │              │    │
│  │ 4.检查完整性  │                    │              │    │
│  └──────┬───────┘                    └──────┬───────┘    │
│         │                                   │            │
│         └─────────────┬─────────────────────┘            │
│                       │                                  │
│                       ▼                                  │
│            ┌─────────────────────┐                       │
│            │  信息完整？          │                       │
│            │                     │                       │
│            │  是 → 自动转人工     │                       │
│            │  否 → 回复缺失项     │                       │
│            └──────────┬──────────┘                       │
│                       │ (转人工时)                        │
│                       ▼                                  │
│  ┌────────────────────────────────────────────────────┐  │
│  │         EscalationHandler (escalate.go)             │  │
│  │                                                    │  │
│  │  1. conv.GetInfoSummary() 构建摘要                  │  │
│  │  2. SendPostMessage → 发送到技术支持群              │  │
│  │  3. ForwardMessage → 逐条转发文件消息到群            │  │
│  │  4. SendTextMessage → 通知用户「已提交」             │  │
│  │  5. ClearConversation → 清除会话上下文               │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
└──────────────────────────────────────────────────────────┘
         │                           │
         ▼                           ▼
┌──────────────────┐      ┌──────────────────┐
│     Redis        │      │   LLM (智谱)      │
│                  │      │                  │
│ - 会话状态       │      │ - 从单条消息中    │
│ - CollectedInfo  │      │   提取信息字段    │
│ - 消息去重标记   │      │ - 低温度精准提取  │
│ - 会话过期清理   │      │                  │
└──────────────────┘      └──────────────────┘
```

## 详细链路说明

### 1. 消息接收链路

```
WebSocket 事件 → EventHandlers.handlePrivateMessage()
```

| 步骤 | 操作 | 说明 |
|------|------|------|
| 1 | 提取基本字段 | 从事件中读取 chatID、senderID、messageID、chatType、msgType |
| 2 | 原子去重 | `store.TryMarkMessageProcessed(messageID)` — Redis SETNX，返回 false 则丢弃 |
| 3 | 过滤非私聊 | chatType != "p2p" 直接忽略 |
| 4 | 提取内容 | `extractMessageInfo()` — 解析 JSON 获取文本或文件信息 |
| 5 | 会话加锁 | `getChatLock(chatID).Lock()` — 同一用户消息串行处理 |
| 6 | 分发处理 | 调用 `MessageHandler.HandleMessage()` |

### 2. 信息收集链路

```
HandleMessage() → Manager.ProcessMessage()
```

| 步骤 | 操作 | 说明 |
|------|------|------|
| 1 | 获取会话 | `store.GetOrCreateConversation()` — 从 Redis 读取或创建新会话 |
| 2 | 分类处理 | 文本消息 → LLM 提取；文件消息 → 存储 fileKey + messageID |
| 3 | LLM 提取 | `llm.ExtractInfo(当前消息, 已收集信息)` — 只从当前这一条消息中提取 |
| 4 | 合并信息 | 新提取的字段合并到 `conv.CollectedInfo`，跳过已存在的相同值 |
| 5 | 完整性判断 | `conv.IsInfoComplete()` — 4 项都有则触发自动转人工 |
| 6 | 构建回复 | 新信息 → "已记录：xxx"；缺失项 → "还需要：xxx" |

### 3. LLM 提取链路

```
Manager → llm.ExtractInfo()
```

| 步骤 | 操作 | 说明 |
|------|------|------|
| 1 | 构建 Prompt | System Prompt 定义 4 个字段 + 严格提取规则 |
| 2 | 注入上下文 | 告知 LLM 已收集的信息（防止重复提取） |
| 3 | 发送请求 | 只传当前用户消息，不传对话历史，Temperature=0.1 |
| 4 | 解析响应 | JSON 提取，自动清理 markdown 代码块、无意义值 |

### 4. 转人工链路

```
HandleEscalation() / 自动触发 → EscalationHandler.HandleEscalation()
```

| 步骤 | 操作 | 说明 |
|------|------|------|
| 1 | 构建摘要 | `conv.GetInfoSummary()` — 代码直接生成，不依赖 LLM |
| 2 | 发送摘要 | `SendPostMessage()` → 富文本消息发到技术支持群 |
| 3 | 转发文件 | `ForwardMessage(msgID, groupID)` — 逐条转发用户上传的文件 |
| 4 | 降级兜底 | 如果转发失败，尝试 `SendFileMessage(fileKey)` 直接发送 |
| 5 | 通知用户 | 发送「已提交」确认消息 |
| 6 | 清除会话 | `ClearConversation()` — 防止重复提交 |

### 5. 消息去重机制

```
SETNX feishu:processed:{messageID} "1" EX 86400
```

- **原子性**：SETNX 是 Redis 原子操作，check-and-set 在一条命令完成
- **防并发**：WebSocket 重连后可能批量推送旧事件，SETNX 确保每条消息只处理一次
- **自动清理**：24 小时 TTL，防止 Redis 膨胀

### 6. 会话锁机制

```go
sync.Map[chatID] → *sync.Mutex
```

- **目的**：同一用户的多条消息必须串行处理，否则 CollectedInfo 会状态冲突
- **范围**：不同用户使用不同锁，互不影响
- **实现**：`sync.Map` 懒初始化 + `sync.Mutex` 阻塞等待

## 交互示例

```
用户: 你好，我遇到个问题
机器人: 您好，我是技术支持助手。

      为了帮您处理问题，请提供以下信息：
      - 版本信息
      - 设备信息
      - 用户信息
      - 问题描述

      您可以一次性告诉我，也可以分多次发送。
      如有日志文件，可直接发送附件。

用户: 版本是v2.1，我用的iPhone 15
机器人: 已记录：版本信息: v2.1、设备信息: iPhone 15

      还需要以下信息：
      - 用户信息
      - 问题描述

      回复「转人工」可直接提交当前信息。

用户: 我是张三，登录的时候一直报错
机器人: 信息收集完毕！

      - 版本信息: v2.1
      - 设备信息: iPhone 15
      - 用户信息: 张三
      - 问题描述: 登录的时候一直报错

      正在为您提交到技术支持团队...

机器人: ✅ 您的问题已提交给技术支持团队，我们会尽快处理！
```

## 快速开始

### 1. 前置要求

- Go 1.21+
- Redis
- 飞书企业自建应用（[创建指南](https://open.feishu.cn/app)）

### 2. 飞书开放平台配置

#### 创建应用

1. 访问 [飞书开放平台](https://open.feishu.cn/app) → 创建"企业自建应用"
2. 记录 `App ID` 和 `App Secret`
3. 应用能力 → 添加能力 → **机器人**
4. 发布版本

#### 配置权限

| 权限名称 | 权限代码 |
|---------|----------|
| 接收私聊消息 | `im:message.p2p_msg:readonly` |
| 发送私聊消息 | `im:message.p2p_msg` |
| 以机器人身份发送 | `im:message:send_as_bot` |
| 获取群组消息 | `im:message:readonly` |
| 获取与上传文件 | `im:resource` |

#### 配置事件订阅

- 事件与回调 → 事件订阅 → **使用长连接接收事件**
- 添加事件：`im.message.receive_v1`

### 3. 配置

编辑 `configs/config.yaml`：

```yaml
feishu:
  app_id: "cli_xxxxx"
  app_secret: "xxxxxxxxxxxx"
  escalation_group_id: "oc_xxxxx"    # 技术支持群的 chat_id

llm:
  provider: "zhipu"
  api_key: "your_api_key"
  base_url: "https://open.bigmodel.cn/api/paas/v4"
  model: "glm-4-flash"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0
  expiration: 600

bot:
  escalation_keywords: ["转人工", "人工", "客服", "提交"]
  clear_context_keywords: ["清空", "清除上下文", "重新开始"]
```

支持环境变量覆盖：`FEISHU_APP_ID`、`FEISHU_APP_SECRET`、`FEISHU_ESCALATION_GROUP_ID`、`LLM_API_KEY`、`REDIS_ADDR` 等。

### 4. 运行

```bash
# 开发模式
go run cmd/bot/main.go

# 编译运行
go build -o feishu-bot cmd/bot/main.go && ./feishu-bot
```

### 5. Docker Compose 部署（推荐）

```bash
docker compose up -d          # 构建并启动
docker compose logs -f bot    # 查看日志
docker compose down           # 停止
```

## 故障排除

| 问题 | 排查方向 |
|------|---------|
| 机器人不回复 | 检查事件订阅是否为"长连接"；检查权限是否全部开通；检查机器人是否已发布 |
| 连接失败 | 检查 Redis 是否运行；检查飞书凭证是否正确 |
| 转人工失败 | 检查 `escalation_group_id` 是否正确；检查机器人是否在该群中 |
| 文件转发失败 | 检查机器人是否有 `im:resource` 权限；检查机器人是否在目标群中 |
| 重复回复 | 检查 Redis 连接是否正常（SETNX 去重依赖 Redis） |

## 许可证

MIT License
