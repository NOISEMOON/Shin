# 使用官方 Go 镜像作为构建阶段
FROM golang:1.23.2 AS builder

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum 文件
COPY go.mod ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY shin.go .

# 构建 Go 应用程序为静态链接的可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -o shin .

# 使用 scratch 作为基础镜像
FROM scratch

ENV POLL_INTERVAL_SECONDS=1800
ENV FRESHRSS_AUTH_URL=
ENV FRESHRSS_LIST_SUBSCRIPTION_URL=
ENV FRESHRSS_CONTENT_URL_PREFIX=
ENV FRESHRSS_FILTERED_LABEL=
ENV SENDER_EMAIL=
ENV SENDER_AUTH_TOKEN=
ENV SMTP_SERVER=
ENV SMTP_PORT=
ENV RECEIVER_EMAIL=
ENV DEFAULT_OT=
ENV OT_MAP_JSON=

# 复制构建好的可执行文件
COPY --from=builder /app/shin .

# 启动应用程序
CMD ["./shin"]