# FileTree — 项目文件索引

> 本文件记录项目中每个文件的职责说明。修改或新增文件后请同步更新此文件。

```
feishu-bot/
├── cmd/
│   └── bot/
│       └── main.go                    # 程序入口：初始化各模块、注册事件处理器、启动 WebSocket；
│                                      # 包含 wrappedMessageHandler 实现消息路由（关键词判断、
│                                      # ProcessMessage 调用、自动/手动转人工分发）
│
├── internal/
│   ├── config/
│   │   └── config.go                  # 配置管理：从 config.yaml 加载配置，支持环境变量覆盖；
│   │                                  # 定义 FeishuConfig / LLMConfig / RedisConfig / BotConfig 结构；
│   │                                  # 提供 IsEscalationKeyword / IsClearContextKeyword 关键词匹配
│   │
│   ├── conversation/
│   │   ├── manager.go                 # 会话管理核心：ProcessMessage 处理用户消息，调用 LLM 提取信息，
│   │                                  # 合并到 CollectedInfo，判断完整性，构建中英双语智能回复；
│   │                                  # 支持中英文建议/反馈触发词；定义 EscalatePrefix 常量
│   │   ├── store.go                   # Redis 存储层：会话的 CRUD 操作、TryMarkMessageProcessed
│   │                                  # (SETNX 原子去重)、会话过期管理
│   │   ├── collector.go               # 信息收集器（备用）：基于规则的本地信息提取，定义 InfoType
│   │                                  # 枚举和 RequiredInfos / OptionalInfos 配置；
│   │                                  # 当前未在主流程中使用，由 LLM 提取替代
│   │   └── prompt.go                  # 提示词管理：从 YAML 文件加载 Prompt 模板，支持占位符替换；
│   │                                  # 当前未在主流程中使用，保留供后续扩展
│   │
│   ├── feishu/
│   │   ├── client.go                  # 飞书 API 客户端：封装 lark SDK，提供 SendTextMessage /
│   │                                  # SendPostMessage（返回 msgID，支持 @用户）/ SendFileMessage /
│   │                                  # ForwardMessage / ReplyMessage / ReplyFileInThread /
│   │                                  # UploadFile / DownloadMessageResource / GetMessage /
│   │                                  # InviteUserToChat（邀请用户入群）等方法
│   │   ├── event_handler.go           # 飞书事件处理器：WebSocket 事件入口，实现原子去重
│   │                                  # (SETNX) + 会话级互斥锁 (sync.Map)，提取消息内容，
│   │                                  # 分发到 MessageHandler
│   │   └── message.go                 # 消息工具（备用）：MessageBuilder 构建转人工消息、
│   │                                  # CreateLogContent 生成对话日志、UploadLogContent 上传日志文件；
│   │                                  # 当前未在主流程中使用，保留供后续扩展
│   │
│   ├── handler/
│   │   └── escalate.go                # 转人工处理器：邀请用户入群 → 发送信息摘要到群组话题
│   │                                  # （@用户）→ 下载文件后重新上传并在话题内回复（ReplyInThread）
│   │                                  # → 通知用户；确保摘要和附件在同一个话题中
│   │
│   └── llm/
│       └── client.go                  # LLM 客户端：定义 Client 接口 (ExtractInfo)，实现
│                                      # OpenAI 兼容的信息提取；英文 System Prompt 支持中英文用户输入；
│                                      # 从用户单条消息中提取 11 个字段，中英双语字段名
│
├── pkg/
│   └── models/
│       └── types.go                   # 公共数据结构：Message / FileInfo / Conversation / ConversationMode /
│                                      # RequiredFields(11项，中英双语名称) 定义；支持问题反馈和建议反馈
│                                      # 两种模式；GetInfoSummary / GetUserSummary 输出中英双语摘要
│
├── configs/
│   └── config.yaml                    # 应用配置文件：飞书凭证、LLM 配置、Redis 连接、
│                                      # 中英文转人工关键词、中英文清除上下文关键词
│
├── Dockerfile                         # 多阶段 Docker 构建：golang:alpine 编译 → alpine 运行
├── docker-compose.yml                 # Docker Compose 编排：bot + Redis 服务，健康检查依赖
├── go.mod                             # Go 模块定义和依赖声明
├── go.sum                             # Go 依赖校验和
├── README.md                          # 项目文档：架构说明、链路详解、部署指南
└── FileTree.md                        # 本文件：项目文件索引和职责说明
```
