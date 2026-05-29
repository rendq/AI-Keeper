# AIP 架构概览

AI Platform (AIP) 采用 Kubernetes-native 的声明式架构，通过 CRD + Controller 模式管理 AI Agent 的全生命周期。

## 核心架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        Control Plane                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │ Manager  │  │   PDP    │  │   PEP    │  │    Audit     │   │
│  │Controller│  │  Policy  │  │Enforcer  │  │  Collector   │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                         Data Plane                                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │ Gateway  │  │  Router  │  │ Runtime  │  │  Channels    │   │
│  │  (API)   │  │  (Model) │  │ (Agent)  │  │  (Feishu/..) │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                        Storage Layer                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │PostgreSQL│  │  Redis   │  │   NATS   │  │ ClickHouse   │   │
│  │  (state) │  │ (cache)  │  │ (events) │  │   (audit)    │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## 关键组件

| 组件 | 职责 |
|------|------|
| **Manager** | CRD Controller，协调 Tenant / Agent / Policy / KnowledgeBase 等资源 |
| **Gateway** | API 入口，认证鉴权、请求路由 |
| **PDP** | Policy Decision Point，OPA-based 策略决策引擎 |
| **PEP** | Policy Enforcement Point，请求级别策略拦截 |
| **Router** | ModelRouter，多模型智能路由（权重/failover/A-B test） |
| **Runtime** | Agent 运行时，执行 Skill / Tool / Chain |
| **Channels** | 渠道适配器（飞书、钉钉、企微、Slack） |
| **Audit** | 审计事件采集与持久化 |
| **Storage** | 基础存储（PostgreSQL + Redis + NATS） |
| **Audit-Storage** | 审计存储（ClickHouse + MinIO） |

## CRD 资源模型

AIP 通过以下 CRD 实现声明式管理：

- `Tenant` — 多租户隔离边界
- `ServiceAccount` — 服务身份与凭证
- `Agent` — AI Agent 定义（Skill、Tool、Prompt）
- `ModelEndpoint` / `ModelRouter` — 模型接入与路由
- `Policy` / `Quota` / `Budget` — 治理策略
- `KnowledgeBase` / `DataSource` — 知识库与数据源
- `AuditEvent` — 审计事件

## 详细设计文档

完整的架构设计、决策记录和接口定义请参考：

📄 **[design.md](../.kiro/specs/ai-platform/design.md)**

该文档包含：
- 组件交互序列图
- API 接口定义
- 数据模型设计
- 部署拓扑
- 安全模型
- 可观测性方案

## 相关资源

- [快速开始](quickstart.md) — 30 分钟本地部署 Demo
- [需求文档](../.kiro/specs/ai-platform/requirements.md) — P0/P1 需求列表
- [Helm Chart](../deploy/helm/ai-keeper/) — 部署配置
- [示例 Packs](../examples/) — Agent Pack 示例
