# AIP Quickstart: Legal Copilot 飞书 Demo

> 30 分钟从零开始，在本地 kind 集群上部署 AI Platform 并运行 Legal Copilot 飞书机器人示例。

## 前置条件

| 工具 | 最低版本 | 安装参考 |
|------|----------|----------|
| Docker | 24+ | https://docs.docker.com/get-docker/ |
| kind | 0.20+ | https://kind.sigs.k8s.io/docs/user/quick-start/#installation |
| kubectl | 1.28+ | https://kubernetes.io/docs/tasks/tools/ |
| Helm | 3.13+ | https://helm.sh/docs/intro/install/ |

确认工具就绪：

```bash
docker version
kind version
kubectl version --client
helm version
```

## 步骤 1：创建 kind 集群

```bash
make kind-up
```

这会创建一个 `aik-dev` kind 集群并启动本地镜像 registry（`localhost:5001`）。

> 如果 `make kind-up` 不可用，可以手动执行：
> ```bash
> kind create cluster --name aik-dev --config hack/kind/kind-config.yaml
> ```

验证集群就绪：

```bash
kubectl cluster-info --context kind-aik-dev
```

## 步骤 2：构建并推送镜像

```bash
make images
```

该命令会构建所有组件镜像并推送到本地 registry。

## 步骤 3：安装 CRDs

```bash
make install-crds
```

## 步骤 4：部署存储层

存储层（PostgreSQL、Redis、NATS、ClickHouse、MinIO）作为 AIP 的依赖，优先部署：

```bash
helm upgrade --install aip deploy/helm/ai-keeper \
  --namespace aik-system --create-namespace \
  --set manager.enabled=false \
  --set gateway.enabled=false \
  --set pdp.enabled=false \
  --set pep.enabled=false \
  --set audit.enabled=false \
  --set router.enabled=false \
  --set runtime.enabled=false \
  --set channels.enabled=false \
  --wait --timeout 5m
```

等待存储 Pod 就绪：

```bash
kubectl -n aik-system wait --for=condition=Ready pod -l app.kubernetes.io/component=storage --timeout=120s
```

## 步骤 5：部署 AIP 平台

完整安装所有组件：

```bash
helm upgrade --install aip deploy/helm/ai-keeper \
  --namespace aik-system --create-namespace \
  --wait --timeout 5m
```

验证所有 Pod 运行正常：

```bash
kubectl -n aik-system get pods
```

预期输出中应看到所有组件（manager、gateway、pdp、pep、audit、router、runtime、channels）均为 `Running` 状态。

## 步骤 6：应用 Legal Copilot Pack

Legal Copilot 是一个预置的 Agent Pack，包含法律知识库、合规 Policy 和飞书频道配置。

```bash
kubectl apply -f examples/packs/legal-copilot/
```

该目录包含以下资源：
- `Tenant` — 法务部租户
- `Agent` — Legal Copilot Agent 定义
- `KnowledgeBase` — 法律文档知识库
- `Policy` — 合规审计策略
- `ModelEndpoint` — LLM 接入配置
- `Channel` — 飞书频道绑定

验证资源已就绪：

```bash
kubectl get agents,knowledgebases,policies -n legal-copilot
```

## 步骤 7：配置飞书机器人

1. 在[飞书开放平台](https://open.feishu.cn/)创建企业自建应用
2. 获取 `App ID`、`App Secret`、`Verification Token`、`Encrypt Key`
3. 更新 channels 配置：

```bash
kubectl -n aik-system create secret generic feishu-credentials \
  --from-literal=app-id=YOUR_APP_ID \
  --from-literal=app-secret=YOUR_APP_SECRET \
  --from-literal=verification-token=YOUR_TOKEN \
  --from-literal=encrypt-key=YOUR_KEY
```

4. 配置飞书事件订阅 webhook 指向 channels 服务：
   - URL: `http://<EXTERNAL_IP>:8080/webhooks/feishu`
   - 本地开发可用 `kubectl port-forward` 配合内网穿透工具

```bash
kubectl -n aik-system port-forward svc/aip-channels 8080:8080
```

## 步骤 8：发送测试消息

在飞书中向 Legal Copilot 机器人发送测试消息：

```
请帮我审查这份合同中的免责条款是否合规
```

你应该能看到机器人返回合规审查结果。

## 步骤 9：验证审计记录

确认审计事件已正确记录：

```bash
kubectl -n aik-system exec -it deploy/aik-audit-storage-clickhouse -- \
  clickhouse-client --query "SELECT * FROM aip_audit.events ORDER BY timestamp DESC LIMIT 5"
```

你应该能看到包含以下字段的审计记录：
- `tenant_id` — 租户标识
- `agent_name` — Legal Copilot
- `action` — 调用动作
- `timestamp` — 事件时间
- `policy_decision` — 策略决策结果

## 清理

```bash
# 删除 Helm release
helm uninstall aip -n aik-system

# 删除 kind 集群
make kind-down
# 或: kind delete cluster --name aik-dev
```

## 故障排查

| 问题 | 解决方案 |
|------|----------|
| Pod CrashLoopBackOff | `kubectl -n aik-system logs <pod-name>` 查看日志 |
| 镜像拉取失败 | 确认 `make images` 成功且 registry 可达 |
| CRD 未找到 | 重新执行 `make install-crds` |
| 飞书 webhook 不通 | 检查 port-forward 和内网穿透配置 |

## 下一步

- 查看 [架构文档](architecture.md) 了解系统设计
- 阅读 `examples/` 目录中的更多示例 Pack
- 参考 `deploy/helm/ai-keeper/values.yaml` 自定义部署配置
