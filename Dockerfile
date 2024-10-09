# 使用官方 Go 镜像作为构建阶段
FROM golang:1.23.2 AS builder

# 设置工作目录
WORKDIR /app

COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o shin .

FROM alpine:latest

# 复制构建好的可执行文件
COPY --from=builder /app/shin .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# 创建数据库存储的目录
RUN mkdir -p /data

EXPOSE 8777

# 启动应用程序
CMD ["./shin"]