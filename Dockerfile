# 多阶段构建
FROM golang:1.25.6-alpine AS builder

# 安装必要的工具
RUN apk add --no-cache git

# 设置工作目录
WORKDIR /app

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 编译
RUN CGO_ENABLED=0 go build -o feishu-bot cmd/bot/main.go

# 最终镜像
FROM alpine:latest

# 安装 ca 证书（用于 HTTPS 请求）
RUN apk --no-cache add ca-certificates

WORKDIR /app

# 从构建阶段复制可执行文件
COPY --from=builder /app/feishu-bot .

# 复制配置文件
COPY --from=builder /app/configs ./configs

# 运行
CMD ["./feishu-bot"]
