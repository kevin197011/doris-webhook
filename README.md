# doris-webhook

一个轻量级的 HTTP Webhook 服务，用于将 HTTP POST 数据实时写入 Apache Doris 数据库。服务直接连接 Doris BE 节点进行 Stream Load，无需经过 FE。

## 特性

- 🚀 高性能：基于 Go 语言和 Gin 框架，支持高并发写入
  - HTTP 连接池：最大 100 个连接，每个主机 50 个连接
  - 连接复用：启用 Keep-Alive，减少连接建立开销
  - 优雅关闭：支持优雅关闭（graceful shutdown），确保请求处理完成后再关闭
  - 结构化日志：使用 `slog` 结构化日志，支持 JSON 格式，便于日志收集和分析
  - 日志级别控制：支持 `debug`, `info`, `warn`, `error` 级别，生产环境默认减少日志输出
- 📦 容器化：支持 Docker 和 Docker Compose 部署
- 🔧 易配置：通过环境变量灵活配置
- ✅ 数据验证：自动验证 JSON 格式和请求内容
- 🔄 连接管理：内置 HTTP 连接池和超时控制
- 📊 流式加载：直接连接 BE 节点，使用 Doris Stream Load API 实现高效数据写入
- 🎯 简单直接：无需配置 FE，直接连接 BE 负载均衡地址
- 🌐 跨域支持：内置 CORS 支持，可通过环境变量配置

## 快速开始

### Docker Compose（推荐用于本地开发）

1. **配置环境变量**

```bash
# 复制环境变量模板
cp env.template .env

# 编辑 .env 文件，修改 Doris 配置
vim .env
```

2. **启动服务**

```bash
# 构建并启动
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

3. **测试接口**

```bash
curl -X POST http://localhost:8080/video \
  -H "Content-Type: application/json" \
  -d '{
    "project": "test",
    "event": "play",
    "userAgent": "Mozilla/5.0"
  }'
```

### 本地运行

```bash
# 设置环境变量
export DORIS_BE_HTTP="10.170.2.56:8040"
export DORIS_PASSWORD="your_password"
export DORIS_DATABASE="video"
export DORIS_USER="devops"

# 运行
go run main.go
```

### 使用 Makefile 构建

```bash
# 查看所有可用命令
make help

# 构建 Docker 镜像
make build

# 构建并推送镜像
make all

# 运行容器
make run
```

### Kubernetes 部署（使用 Helm）

项目包含 Helm Chart，可以快速部署到 Kubernetes 集群。

**安装：**

```bash
# 创建命名空间（如果不存在）
kubectl create namespace devops

# 使用默认配置（部署到 devops 命名空间）
helm install doris-webhook ./helm/doris-webhook -n devops

# 使用自定义配置
helm install doris-webhook ./helm/doris-webhook -n devops \
  --set doris.beHttp="10.170.2.56:8040" \
  --set secret.create=true \
  --set secret.password="your_password"
```

**配置示例：**

```bash
# 创建自定义 values 文件
cat > my-values.yaml <<EOF
namespace: devops
doris:
  beHttp: "10.170.2.56:8040"
  database: "video"
  user: "devops"

secret:
  create: true
  password: "your_password"

replicaCount: 3
EOF

# 使用自定义配置安装到 devops 命名空间
helm install doris-webhook ./helm/doris-webhook -f my-values.yaml -n devops
```

**验证部署：**

```bash
# 查看 Pod 状态
kubectl get pods -l app.kubernetes.io/name=doris-webhook -n devops

# 测试服务
kubectl port-forward svc/doris-webhook 8080:8080 -n devops
curl http://localhost:8080/health
```

更多 Helm 配置说明请参考 [helm/doris-webhook/README.md](./helm/doris-webhook/README.md)

### Istio 配置（可选）

如果集群中启用了 Istio，可以使用 Istio VirtualService 和 Gateway 来管理流量。

**使用 Helm 启用 Istio：**

```bash
helm install doris-webhook ./helm/doris-webhook -n devops \
  --set istio.enabled=true \
  --set istio.hosts[0]=doris-webhook.example.com \
  --set istio.gateways[0]=istio-system/infra-istio-ingressgateway-extra
```

**或直接使用示例文件：**

```bash
# 应用 VirtualService
kubectl apply -f helm/doris-webhook/examples/istio-virtualservice.yaml

# 或使用完整配置
kubectl apply -f helm/doris-webhook/examples/istio-complete.yaml
```

**注意**：VirtualService 需要配合已有的 Gateway 使用。确保集群中已有 Gateway（如 `istio-system/istio-gateway`），或根据实际情况修改示例文件中的 `gateways` 配置。

更多 Istio 配置说明请参考 [helm/doris-webhook/README.md](./helm/doris-webhook/README.md#istio-配置)

### CI/CD 自动构建

项目包含 GitHub Actions workflow，可以自动构建 Docker 镜像并推送到 GitHub Packages。

**触发条件：**
- 推送到 `main` 或 `master` 分支
- 创建版本标签（如 `v1.0.0`）
- 手动触发（workflow_dispatch）
- Pull Request（仅构建，不推送）

**使用 GitHub Packages 镜像：**

1. **创建 GitHub Personal Access Token (PAT)**
   - 访问 GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
   - 创建新 token，勾选 `read:packages` 权限
   - 保存 token（只显示一次）

2. **登录到 GitHub Container Registry**

```bash
# 使用 PAT 登录（推荐）
echo $GITHUB_TOKEN | docker login ghcr.io -u YOUR_USERNAME --password-stdin

# 或使用交互式登录
docker login ghcr.io -u YOUR_USERNAME
# 输入 PAT 作为密码
```

3. **拉取镜像**

```bash
# 拉取最新镜像
docker pull ghcr.io/OWNER/doris-webhook:latest

# 或使用特定版本
docker pull ghcr.io/OWNER/doris-webhook:v1.0.0

# 查看可用标签（需要先登录）
curl -H "Authorization: Bearer $(echo $GITHUB_TOKEN | base64)" \
  https://ghcr.io/v2/OWNER/doris-webhook/tags/list
```

**故障排除：**

如果遇到连接超时错误（`Client.Timeout exceeded`），可以尝试：

```bash
# 1. 检查网络连接
ping ghcr.io

# 2. 配置 Docker 镜像加速器（如果在中国大陆）
# 编辑 /etc/docker/daemon.json，添加：
{
  "registry-mirrors": ["https://docker.mirrors.ustc.edu.cn"]
}

# 3. 增加 Docker 客户端超时时间
export DOCKER_CLIENT_TIMEOUT=300
export COMPOSE_HTTP_TIMEOUT=300

# 4. 使用代理（如果需要）
export HTTP_PROXY=http://proxy.example.com:8080
export HTTPS_PROXY=http://proxy.example.com:8080
docker pull ghcr.io/OWNER/doris-webhook:latest
```

**镜像标签规则：**
- `latest`: 默认分支的最新构建
- `v1.0.0`: 语义化版本标签
- `v1.0`: 主版本号
- `v1`: 大版本号
- `main-<sha>`: 分支名和提交 SHA

## 环境变量配置

### 必需环境变量

- `DORIS_BE_HTTP`: BE HTTP 地址，格式为 `host:port` 或 `http://host:port`
  - 示例：`10.170.2.56:8040` 或 `http://10.170.2.56:8040`
- `DORIS_PASSWORD`: Doris 用户密码

**注意**：本服务直接连接 BE 节点进行 Stream Load，不经过 FE。

### 可选环境变量

- `DORIS_DATABASE`: 数据库名（默认: `video`）
- `DORIS_USER`: 用户名（默认: `devops`）
- `CORS_ALLOWED_ORIGIN`: 允许的跨域源（默认: `*`，允许所有）
- `CORS_ALLOWED_METHODS`: 允许的 HTTP 方法（默认: `GET, POST, OPTIONS`）
- `CORS_ALLOWED_HEADERS`: 允许的请求头（默认: `Content-Type, Authorization`）
- `CORS_ALLOW_CREDENTIALS`: 是否允许携带凭证（默认: `false`）
- `CORS_MAX_AGE`: 预检请求缓存时间，单位秒（默认: `3600`）
- `LOG_LEVEL`: 日志级别（默认: `info`），可选值：`debug`, `info`, `warn`, `error`
- `LOG_FORMAT`: 日志格式（默认: `text`），可选值：`text`, `json`（JSON 格式更适合日志收集系统）
- `GIN_MODE`: Gin 框架模式（默认: `release`），可选值：`debug`, `release`, `test`
- `DEBUG`: 调试模式（默认: `false`），设置为 `true` 时输出详细调试日志

### 配置说明

- 必须设置 `DORIS_BE_HTTP`，指向 BE 的 HTTP 地址（通常是负载均衡地址）
- BE HTTP 端口通常是 8040
- 如果地址没有协议前缀，会自动添加 `http://`

**配置示例：**

```bash
# 直接连接 BE 负载均衡地址
export DORIS_BE_HTTP="10.170.2.56:8040"
export DORIS_PASSWORD="your_password"
export DORIS_DATABASE="video"
export DORIS_USER="devops"
```

## API 接口

### POST /video

写入视频指标数据到 Doris。

**请求头：**
- `Content-Type: application/json`（必需）

**请求体：**

```json
{
  "project": "my-project",
  "event": "play",
  "userAgent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"
}
```

**字段说明：**
- `project` (string): 项目名称
- `event` (string): 事件类型
- `userAgent` (string): 用户代理字符串

**请求示例：**

```bash
curl -X POST http://localhost:8080/video \
  -H "Content-Type: application/json" \
  -d '{
    "project": "my-project",
    "event": "play",
    "userAgent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"
  }'
```

**响应状态码：**
- `200 OK`: 数据写入成功
- `400 Bad Request`: 请求格式错误（JSON 格式无效或字段缺失）
- `405 Method Not Allowed`: 请求方法不正确（仅支持 POST）
- `415 Unsupported Media Type`: Content-Type 不正确
- `502 Bad Gateway`: Doris 连接失败或写入失败

**成功响应：**

```
Data processed successfully.
```

### GET /health

健康检查端点，用于检查服务是否正常运行。

**请求示例：**

```bash
curl http://localhost:8080/health
```

**成功响应：**

```json
{
  "status": "ok",
  "service": "doris-webhook"
}
```

## 数据库表结构

表名：`video_metrics`

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `project` | VARCHAR | 项目名称 |
| `event` | VARCHAR | 事件类型 |
| `user_agent` | VARCHAR | 用户代理 |

**创建表 SQL 示例：**

```sql
CREATE TABLE IF NOT EXISTS video_metrics (
    project VARCHAR(255),
    event VARCHAR(255),
    user_agent VARCHAR(500)
) ENGINE=OLAP
DUPLICATE KEY(project, event)
DISTRIBUTED BY HASH(project) BUCKETS 10
PROPERTIES (
    "replication_num" = "1"
);
```

## 开发

### 依赖要求

- Go 1.23.3 或更高版本
- Docker（可选，用于容器化部署）

### 构建

```bash
# 下载依赖
go mod download

# 构建
go build -o doris-webhook main.go

# 运行
./doris-webhook
```

### 项目结构

```
.
├── main.go              # 主程序文件
├── go.mod              # Go 模块定义
├── go.sum              # 依赖校验和
├── Dockerfile          # Docker 镜像构建文件
├── docker-compose.yml  # Docker Compose 配置
├── Makefile            # 构建脚本
├── env.template        # 环境变量模板
└── README.md           # 项目文档
```

## 技术细节

- **HTTP 服务器**: 使用 Go 标准库 `net/http`
- **数据写入**: 直接连接 Doris BE 节点，通过 Stream Load API 实现流式写入
- **连接管理**: 使用 HTTP 连接池，支持最大 20 个空闲连接
- **超时控制**: 请求超时时间 10 秒，读写超时分别设置
- **数据格式**: 使用 JSON 格式，支持按行读取（`read_json_by_line=true`）
- **重定向处理**: 自动跟随 HTTP 重定向（最多 10 次）

## 直接使用 curl 连接 BE

如果需要直接使用 curl 命令连接 BE 插入数据，可以参考以下示例：

```bash
# 生成 UUID label
LABEL=$(uuidgen)

# Base64 编码认证信息
AUTH=$(echo -n "devops:your_password" | base64)

# 执行 Stream Load
curl -X PUT \
  "http://10.170.2.56:8040/api/video/video_metrics/_stream_load" \
  -H "Authorization: Basic ${AUTH}" \
  -H "Content-Type: application/json" \
  -H "Expect: 100-continue" \
  -H "label: ${LABEL}" \
  -H "format: json" \
  -H "read_json_by_line: true" \
  -H "columns: project,event,user_agent" \
  --data-binary '{"project":"my-project","event":"play","user_agent":"Mozilla/5.0"}
' \
  -v
```

**重要说明：**
- BE HTTP 端口通常是 8040（不是 FE 的 8030）
- 数据格式：每行一个 JSON 对象，末尾需要换行符（`\n`）
- label 必须唯一，重复的 label 会被忽略
- 成功响应中 `Status` 字段应为 `"Success"`

## 许可证

详见 [LICENSE](./LICENSE) 文件。
