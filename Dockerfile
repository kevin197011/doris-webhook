# Build stage
FROM golang:1.23 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o doris-webhook .

# Runtime stage
FROM alpine:latest

# 安装时区数据并设置为香港时间（东八区）
RUN apk add --no-cache tzdata && \
    cp /usr/share/zoneinfo/Asia/Hong_Kong /etc/localtime && \
    echo "Asia/Hong_Kong" > /etc/timezone && \
    apk del tzdata

WORKDIR /app
COPY --from=builder /app/doris-webhook .

# 设置时区环境变量
ENV TZ=Asia/Hong_Kong

ARG APP_PORT=8080
EXPOSE ${APP_PORT}
CMD ["/app/doris-webhook"]
