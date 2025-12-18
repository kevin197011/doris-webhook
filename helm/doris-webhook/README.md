# doris-webhook Helm Chart

用于在 Kubernetes 集群中部署 doris-webhook 服务的 Helm Chart。

## 安装

### 使用部署脚本（推荐）

```bash
# 进入 Helm chart 目录
cd helm/doris-webhook

# 使用默认配置安装（devops 命名空间）
./deploy.sh

# 安装到指定命名空间
./deploy.sh -n production

# 使用自定义发布名称
./deploy.sh -r my-doris-webhook -n production

# 升级现有发布
./deploy.sh -u

# 查看帮助
./deploy.sh -h
```

### 使用默认配置（部署到 devops 命名空间）

```bash
# 创建命名空间（如果不存在）
kubectl create namespace devops

# 安装到 devops 命名空间
helm install doris-webhook ./helm/doris-webhook -n devops
```

### 使用自定义配置

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

### 使用 Secret 存储密码（推荐）

```bash
# 方式 1: 使用 Helm values 中的 secret.password
helm install doris-webhook ./helm/doris-webhook -n devops \
  --set secret.create=true \
  --set secret.password="your_password"

# 方式 2: 使用已有的 Secret
# 先创建 Secret（在 devops 命名空间）
kubectl create secret generic doris-webhook-secret \
  --from-literal=password="your_password" \
  -n devops

# 安装时指定使用已有 Secret
helm install doris-webhook ./helm/doris-webhook -n devops \
  --set secret.create=false \
  --set secret.name="doris-webhook-secret"
```

## 配置说明

### 必需配置

- `doris.beHttp`: BE HTTP 地址（格式：`host:port` 或 `http://host:port`）
- `secret.password` 或 `doris.password`: Doris 用户密码

### 主要配置项

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `namespace` | 部署命名空间 | `devops` |
| `replicaCount` | 副本数 | `2` |
| `image.repository` | 镜像仓库 | `ghcr.io/kevin197011/doris-webhook` |
| `image.tag` | 镜像标签 | `latest` |
| `service.type` | Service 类型 | `ClusterIP` |
| `service.port` | Service 端口 | `8080` |
| `doris.beHttp` | BE HTTP 地址 | `10.170.2.56:8040` |
| `doris.database` | 数据库名 | `video` |
| `doris.user` | 用户名 | `devops` |
| `secret.create` | 是否创建 Secret | `true` |
| `resources.limits.cpu` | CPU 限制 | `500m` |
| `resources.limits.memory` | 内存限制 | `256Mi` |

### 完整配置示例

```yaml
namespace: devops

replicaCount: 3

image:
  repository: ghcr.io/kevin197011/doris-webhook
  tag: "v1.0.0"
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

doris:
  beHttp: "10.170.2.56:8040"
  database: "video"
  user: "devops"

secret:
  create: true
  password: "your_password"

resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80

livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 10
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 5
  periodSeconds: 10
```

## 升级

```bash
# 升级到新版本
helm upgrade doris-webhook ./helm/doris-webhook -n devops

# 使用新的配置升级
helm upgrade doris-webhook ./helm/doris-webhook -f my-values.yaml -n devops
```

## 卸载

```bash
helm uninstall doris-webhook -n devops
```

## 验证部署

```bash
# 查看 Pod 状态
kubectl get pods -l app.kubernetes.io/name=doris-webhook -n devops

# 查看 Service
kubectl get svc -l app.kubernetes.io/name=doris-webhook -n devops

# 查看日志
kubectl logs -l app.kubernetes.io/name=doris-webhook -n devops

# 测试健康检查
kubectl port-forward svc/doris-webhook 8080:8080 -n devops
curl http://localhost:8080/health
```

## 故障排除

### Pod 无法启动

```bash
# 查看 Pod 事件
kubectl describe pod <pod-name> -n devops

# 查看日志
kubectl logs <pod-name> -n devops
```

### 连接 Doris 失败

检查环境变量配置：

```bash
kubectl exec <pod-name> -n devops -- env | grep DORIS
```

### Secret 问题

```bash
# 查看 Secret
kubectl get secret <secret-name> -n devops -o yaml

# 验证密码
kubectl get secret <secret-name> -n devops -o jsonpath='{.data.password}' | base64 -d
```

## Istio 配置

如果集群中启用了 Istio，可以配置 VirtualService 来管理流量路由。VirtualService 需要配合已有的 Gateway 使用。

### 启用 Istio

```bash
# 方式 1: 使用 values 文件
cat > istio-values.yaml <<EOF
istio:
  enabled: true
  hosts:
    - doris-webhook.example.com
  gateways:
    - istio-system/infra-istio-ingressgateway-extra
EOF

helm install doris-webhook ./helm/doris-webhook -n devops -f istio-values.yaml

# 方式 2: 使用命令行参数
helm install doris-webhook ./helm/doris-webhook -n devops \
  --set istio.enabled=true \
  --set istio.hosts[0]=doris-webhook.example.com \
  --set istio.gateways[0]=istio-system/istio-gateway
```

### Istio 配置说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `istio.enabled` | 是否启用 Istio | `false` |
| `istio.hosts` | VirtualService 的 hosts | `["doris-webhook.local"]` |
| `istio.gateways` | Gateways 列表 | `[]` |
| `istio.timeout` | 请求超时 | `"30s"` |
| `istio.retries.attempts` | 重试次数 | `3` |
| `istio.gateway.create` | 是否创建 Gateway | `false` |

### 完整 Istio 配置示例

```yaml
istio:
  enabled: true
  hosts:
    - doris-webhook.example.com
    - doris-webhook.internal
  gateways:
    - istio-system/infra-istio-ingressgateway-extra  # 使用已有的 Gateway
  timeout: "30s"
  retries:
    attempts: 3
    perTryTimeout: "10s"
    retryOn: "5xx,reset,connect-failure"
  corsPolicy:
    allowOrigins:
      - exact: "*"
    allowMethods:
      - GET
      - POST
    allowHeaders:
      - content-type
    maxAge: "24h"
```

**注意**：VirtualService 需要配合已有的 Gateway 使用。如果集群中没有 Gateway，需要先创建 Gateway 或使用集群中已有的 Gateway。

### 验证 Istio 配置

```bash
# 查看 VirtualService
kubectl get virtualservice -n devops

# 查看 VirtualService 详情
kubectl describe virtualservice doris-webhook -n devops

# 查看 Gateway（如果使用已有的）
kubectl get gateway -n istio-system
```

