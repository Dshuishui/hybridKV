# 第一阶段：构建阶段
FROM golang:1.22-alpine AS builder

# 安装必要的工具
RUN apk --no-cache add ca-certificates git

# 设置Go代理（解决网络问题）
ENV GOPROXY=https://goproxy.cn,direct
ENV GO111MODULE=on

# 设置工作目录
WORKDIR /app

# 复制go.mod和go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建主应用（KV服务器）
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o kvserver ./kvstore/kvserver/kvserver.go

# 构建测试客户端
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o test-client ./benchmark/client/client.go

# 第二阶段：运行阶段（轻量级，不包含Go环境）
FROM alpine:latest

# 安装ca-certificates
RUN apk --no-cache add ca-certificates

# 创建非root用户
RUN adduser -D -s /bin/sh kvuser

# 设置工作目录
WORKDIR /app

# 从构建阶段复制预编译的二进制文件
COPY --from=builder /app/kvserver .
COPY --from=builder /app/test-client .

# 复制配置文件
COPY --from=builder /app/config ./config

# 创建data目录并设置权限
RUN mkdir -p data && chown -R kvuser:kvuser /app

# 给二进制文件执行权限
RUN chmod +x kvserver test-client

# 检查文件是否复制成功
RUN ls -la /app

# 切换到非root用户
USER kvuser

# 暴露端口
EXPOSE 3088 30881

# 设置启动命令
ENTRYPOINT ["./kvserver"]