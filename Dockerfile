# 使用官方 Go 镜像作为构建阶段
FROM golang:1.23.2 AS builder

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -o shin .
# RUN CGO_ENABLED=1 GOOS=linux go build -o shin .

# 使用小的基础镜像来运行应用
FROM alpine:latest

# ENV DB_PATH=/data/shin.db
# ENV POLL_INTERVAL_SECONDS=1800
# ENV FRESHRSS_AUTH_URL=
# ENV FRESHRSS_LIST_SUBSCRIPTION_URL=
# ENV FRESHRSS_CONTENT_URL_PREFIX=
# ENV FRESHRSS_FILTERED_LABEL=
# ENV SENDER_EMAIL=
# ENV SENDER_AUTH_TOKEN=
# ENV SMTP_SERVER=
# ENV SMTP_PORT=
# ENV RECEIVER_EMAIL=
# ENV DEFAULT_OT=
# ENV OT_MAP_JSON=

# 复制构建好的可执行文件
COPY --from=builder /app/shin .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# 创建数据库存储的目录
RUN mkdir -p /data

EXPOSE 8777

# 启动应用程序
CMD ["./shin"]