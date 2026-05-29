# AI Platform CRD — OpenAPI 3.0 Schema (v1alpha1)

> 适用范围：本文档定义 AI Platform（`ai-keeper.io`）所有核心资源的 OpenAPI 3.0 schema，
> 可直接用于：
>
> - K8s CustomResourceDefinition 生成（`openAPIV3Schema` 字段）
> - 服务端 / 客户端代码生成（openapi-generator、oapi-codegen、kubebuilder、controller-gen）
> - API 网关参数校验（Kong、Envoy、APISIX）
> - 文档站（Redoc、Swagger UI）
>
> 设计依据：见同目录《AI Platform CRD 设计草案》。本文件为该草案的形式化 schema。

## 目录

- [1. 设计约定](#1-设计约定)
- [2. API Group 划分](#2-api-group-划分)
- [3. 通用 Components（共享类型）](#3-通用-components共享类型)
- [4. core.ai-keeper.io](#4-coreaipio)
  - [4.1 Tenant](#41-tenant)
  - [4.2 ServiceAccount](#42-serviceaccount)
- [5. skill.ai-keeper.io](#5-skillaipio)
  - [5.1 Skill](#51-skill)
  - [5.2 Tool](#52-tool)
- [6. agent.ai-keeper.io](#6-agentaipio)
  - [6.1 Agent](#61-agent)
- [7. policy.ai-keeper.io](#7-policyaipio)
  - [7.1 Policy](#71-policy)
  - [7.2 Budget](#72-budget)
  - [7.3 Quota](#73-quota)
- [8. data.ai-keeper.io](#8-dataaipio)
  - [8.1 DataSource](#81-datasource)
  - [8.2 KnowledgeBase](#82-knowledgebase)
- [9. model.ai-keeper.io](#9-modelaipio)
  - [9.1 ModelEndpoint](#91-modelendpoint)
  - [9.2 ModelRouter](#92-modelrouter)
- [10. audit.ai-keeper.io](#10-auditaipio)
  - [10.1 AuditEvent](#101-auditevent)
- [11. K8s CRD 包装示例](#11-k8s-crd-包装示例)
- [12. 代码生成与校验](#12-代码生成与校验)

---

## 1. 设计约定

| 约定 | 说明 |
|---|---|
| OpenAPI 版本 | 3.0.3（K8s CRD 当前支持最稳的 subset） |
| Schema dialect | JSON Schema Draft 04 兼容（K8s `apiextensions.k8s.io/v1` 限制） |
| 命名风格 | camelCase（与 K8s 规范一致） |
| 必填字段 | 通过 `required` 显式声明；未声明视为可选 |
| 枚举开放性 | 所有 `enum` 字段都允许 `x-` 前缀的扩展值（在文档中说明，schema 不强约束） |
| 引用 URI | 统一格式 `<scheme>://<path>[@<version>]`，正则 `^(skill|agent|tool|model|data|prompt|channel|connector|memory|quota|ref|siem|policy):\/\/[A-Za-z0-9._/\-]+(@[A-Za-z0-9._\-+]+)?$` |
| 时间格式 | RFC 3339（`format: date-time`） |
| 时长格式 | Go `time.Duration` 字符串（`30s`, `5m`, `2h`, `7d`）；正则 `^\d+(ns\|us\|ms\|s\|m\|h\|d\|w)$` |
| 资源标识 | DNS-1123 subdomain（≤253 字符，正则 `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`） |
| 版本字段 | strict semver（正则参考 semver.org 官方） |
| `x-kubernetes-*` | 在需要保留未知字段、列表合并、IntOrString 时使用，已显式标注 |

> **说明**：本文档中各资源的 schema 使用 OpenAPI 3.0 单文件结构，所有共享类型集中在 `#/components/schemas`，
> 各资源单独的 `spec`/`status` schema 通过 `$ref` 引用通用类型。
> 对于 K8s CRD，可用脚本将各资源 schema 提取出来，套上 `apiextensions.k8s.io/v1` CRD 外壳即可（见 §11）。

---

## 2. API Group 划分

| API Group | Scope | Kind |
|---|---|---|
| `core.ai-keeper.io/v1alpha1` | Cluster / Namespaced | Tenant, ServiceAccount |
| `skill.ai-keeper.io/v1alpha1` | Namespaced | Skill, Tool |
| `agent.ai-keeper.io/v1alpha1` | Namespaced | Agent |
| `policy.ai-keeper.io/v1alpha1` | Namespaced | Policy, Budget, Quota |
| `data.ai-keeper.io/v1alpha1` | Namespaced | DataSource, KnowledgeBase |
| `model.ai-keeper.io/v1alpha1` | Cluster / Namespaced | ModelEndpoint, ModelRouter |
| `audit.ai-keeper.io/v1alpha1` | Namespaced (read-only) | AuditEvent |

---

## 3. 通用 Components（共享类型）

```yaml
openapi: 3.0.3
info:
  title: AI Platform Core API
  version: v1alpha1
  description: AI Platform CRD shared components

components:
  schemas:

    # ============================================================
    # 基础类型
    # ============================================================

    ObjectMeta:
      type: object
      description: K8s 风格资源元数据(精简版,完整定义见 K8s 官方)
      required: [name]
      properties:
        name:
          type: string
          maxLength: 253
          pattern: '^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$'
        namespace:
          type: string
          maxLength: 63
          pattern: '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'
        labels:
          type: object
          additionalProperties: { type: string }
        annotations:
          type: object
          additionalProperties: { type: string }
        generation:
          type: integer
          format: int64
          readOnly: true
        creationTimestamp:
          type: string
          format: date-time
          readOnly: true
        uid:
          type: string
          format: uuid
          readOnly: true

    ResourceRef:
      type: string
      description: 引用 URI,格式 <scheme>://<path>[@<version>]
      pattern: '^(skill|agent|tool|model|data|prompt|channel|connector|memory|quota|ref|siem|policy)://[A-Za-z0-9._/\-]+(@[A-Za-z0-9._\-+]+)?$'
      example: "skill://contract-review@1.2.0"

    Duration:
      type: string
      pattern: '^\d+(ns|us|ms|s|m|h|d|w)$'
      example: "30s"

    SemVer:
      type: string
      pattern: '^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$'
      example: "1.2.0"

    VersionConstraint:
      type: string
      description: npm/composer 风格版本约束
      example: ">=1.2.0 <2.0.0"

    Classification:
      type: string
      description: 数据/资源敏感分级
      enum: [public, internal, confidential, restricted, secret]

    Stage:
      type: string
      enum: [experimental, beta, stable, deprecated]

    # ============================================================
    # Status 通用结构
    # ============================================================

    Condition:
      type: object
      required: [type, status, lastTransitionTime]
      properties:
        type: { type: string, maxLength: 316 }
        status:
          type: string
          enum: ["True", "False", "Unknown"]
        reason: { type: string }
        message: { type: string }
        lastTransitionTime: { type: string, format: date-time }
        observedGeneration: { type: integer, format: int64 }

    StatusBase:
      type: object
      properties:
        phase:
          type: string
          enum: [Pending, Active, Running, Degraded, Failed, Deprecated, Terminating]
        observedGeneration: { type: integer, format: int64 }
        conditions:
          type: array
          items: { $ref: '#/components/schemas/Condition' }
          x-kubernetes-list-type: map
          x-kubernetes-list-map-keys: [type]

    # ============================================================
    # Schema/合规/治理通用块
    # ============================================================

    JSONSchema:
      type: object
      description: 嵌套 JSON Schema(Draft 2020-12,K8s CRD 内部不深度校验)
      x-kubernetes-preserve-unknown-fields: true

    GovernanceBlock:
      type: object
      properties:
        classification: { $ref: '#/components/schemas/Classification' }
        dataResidency:
          type: object
          properties:
            allowedRegions:
              type: array
              items: { type: string }
              example: [cn-north, cn-shanghai, eu-west, us-east]
            crossBorder:
              type: string
              enum: [forbidden, allowed_with_approval, allowed]
        pii:
          type: object
          properties:
            onInput:
              type: string
              enum: [ignore, detect, detect_and_mask, detect_and_block]
            onOutput:
              type: string
              enum: [ignore, detect, detect_and_mask, detect_and_block]
            patternsRef: { $ref: '#/components/schemas/ResourceRef' }
        compliance:
          type: object
          properties:
            required:
              type: array
              items:
                type: string
                description: 合规标签,平台预置 + 客户扩展
                example: GDPR
            reportTemplate: { $ref: '#/components/schemas/ResourceRef' }
        humanInTheLoop:
          $ref: '#/components/schemas/HumanInTheLoop'

    HumanInTheLoop:
      type: object
      properties:
        required: { type: boolean, default: false }
        triggerWhen:
          type: array
          description: CEL 表达式,任一为真即触发人审
          items: { type: string }
        approver:
          type: object
          required: [kind, name]
          properties:
            kind:
              type: string
              enum: [User, Role, Group, ServiceAccount]
            name: { type: string }
        timeout: { $ref: '#/components/schemas/Duration' }
        ifTimeout:
          type: string
          enum: [allow, deny, escalate]
          default: deny

    CostBlock:
      type: object
      properties:
        estimator:
          type: object
          properties:
            type:
              type: string
              enum: [static, model_based, historical]
            historicalWindow: { $ref: '#/components/schemas/Duration' }
            tokensPerCall:
              $ref: '#/components/schemas/Percentiles'
            usdPerCall:
              $ref: '#/components/schemas/Percentiles'
        budget:
          type: object
          properties:
            tokensPerCall: { type: integer, minimum: 0 }
            usdPerCall: { type: number, minimum: 0 }

    Percentiles:
      type: object
      properties:
        p50: { type: number }
        p95: { type: number }
        p99: { type: number }

    SLOBlock:
      type: object
      properties:
        p95LatencyMs: { type: integer, minimum: 0 }
        p99LatencyMs: { type: integer, minimum: 0 }
        successRate:  { type: number, minimum: 0, maximum: 1 }
        availability:
          type: string
          example: "99.5%"

    ReliabilityBlock:
      type: object
      properties:
        timeout: { $ref: '#/components/schemas/Duration' }
        retries:
          type: object
          properties:
            max: { type: integer, minimum: 0, maximum: 10 }
            backoff:
              type: string
              enum: [fixed, linear, exponential]
            retryOn:
              type: array
              items:
                type: string
                enum: [timeout, 4xx, 5xx, network, rate_limit]
        fallback:
          type: object
          properties:
            kind:
              type: string
              enum: [Skill, Agent, StaticResponse]
            ref: { $ref: '#/components/schemas/ResourceRef' }
            staticResponse: { type: string }
        circuitBreaker:
          type: object
          properties:
            errorRateThreshold: { type: number, minimum: 0, maximum: 1 }
            window: { $ref: '#/components/schemas/Duration' }
            cooldown: { $ref: '#/components/schemas/Duration' }
```

---

## 4. core.ai-keeper.io

### 4.1 Tenant

集群级资源,描述一个客户/业务线的隔离单元和合规域。

```yaml
components:
  schemas:
    Tenant:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [core.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [Tenant] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/TenantSpec' }
        status:     { $ref: '#/components/schemas/TenantStatus' }

    TenantSpec:
      type: object
      required: [displayName, complianceProfile]
      properties:
        displayName: { type: string, maxLength: 200 }
        description: { type: string, maxLength: 2000 }
        contacts:
          type: array
          items:
            type: object
            required: [role, email]
            properties:
              role:
                type: string
                enum: [admin, security, billing, dpo]
              email: { type: string, format: email }
        complianceProfile:
          type: object
          required: [tier]
          properties:
            tier:
              type: string
              enum: [basic, standard, regulated, classified]
              description: 合规等级,影响默认审计/加密/备份策略
            certifications:
              type: array
              items:
                type: string
                example: SOC2
            dataResidency:
              type: object
              properties:
                primaryRegion: { type: string, example: cn-shanghai }
                allowedRegions:
                  type: array
                  items: { type: string }
                forbidCrossBorder: { type: boolean, default: true }
        modelAllowlist:
          type: array
          description: 该租户允许使用的模型(信创/合规白名单)
          items: { $ref: '#/components/schemas/ResourceRef' }
        defaultBudget:
          type: object
          properties:
            usdPerMonth: { type: number, minimum: 0 }
            tokensPerMonth: { type: integer, minimum: 0 }
        deployment:
          type: object
          properties:
            mode:
              type: string
              enum: [saas_shared, saas_dedicated, vpc, on_premise, airgapped]
            controlPlane:
              type: string
              enum: [hosted, self_managed]
            dataPlane:
              type: string
              enum: [hosted, customer_vpc, on_premise]

    TenantStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            namespaces:
              type: array
              items: { type: string }
            usage:
              type: object
              properties:
                activeAgents: { type: integer }
                activeSkills: { type: integer }
                last30dInvocations: { type: integer, format: int64 }
                last30dCostUsd: { type: number }
            certificationsObtained:
              type: array
              items:
                type: object
                properties:
                  name: { type: string }
                  expiresAt: { type: string, format: date-time }
```

### 4.2 ServiceAccount

Agent / Workload 的非人类身份。

```yaml
components:
  schemas:
    ServiceAccount:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [core.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [ServiceAccount] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/ServiceAccountSpec' }
        status:     { $ref: '#/components/schemas/ServiceAccountStatus' }

    ServiceAccountSpec:
      type: object
      required: [identityProvider]
      properties:
        identityProvider:
          type: string
          description: 引用 IdP 配置名(OIDC/SAML/SPIFFE)
          example: corp-okta
        spiffeId:
          type: string
          description: SPIFFE ID,用于 workload identity
          pattern: '^spiffe://[A-Za-z0-9._\-]+(/[A-Za-z0-9._\-]+)*$'
        attributes:
          type: object
          description: 用于 ABAC 决策的自定义属性
          additionalProperties: { type: string }
        tokenLifetime: { $ref: '#/components/schemas/Duration' }
        allowOnBehalfOf:
          type: boolean
          default: false
          description: 是否允许通过 OBO(RFC 8693)代表用户身份调用下游

    ServiceAccountStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            issuedTokens24h: { type: integer }
            lastUsedAt:      { type: string, format: date-time }
```

---

## 5. skill.ai-keeper.io

### 5.1 Skill

平台最重要的可复用业务能力单元。`interface`(契约)与 `implementation`(实现)严格分离。

```yaml
components:
  schemas:
    Skill:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [skill.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [Skill] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/SkillSpec' }
        status:     { $ref: '#/components/schemas/SkillStatus' }

    SkillSpec:
      type: object
      required: [version, stability, interface, implementation]
      properties:
        # ----- 1. 契约 -----
        version:   { $ref: '#/components/schemas/SemVer' }
        stability: { $ref: '#/components/schemas/Stage' }
        interface:
          $ref: '#/components/schemas/SkillInterface'

        # ----- 2. 实现 -----
        implementation:
          $ref: '#/components/schemas/SkillImplementation'

        # ----- 3. 治理 -----
        governance: { $ref: '#/components/schemas/GovernanceBlock' }

        # ----- 4. 成本/SLO -----
        cost:        { $ref: '#/components/schemas/CostBlock' }
        slo:         { $ref: '#/components/schemas/SLOBlock' }

        # ----- 5. 可靠性 -----
        reliability: { $ref: '#/components/schemas/ReliabilityBlock' }

        # ----- 6. 评测 -----
        evaluation:  { $ref: '#/components/schemas/SkillEvaluation' }

        # ----- 7. 生命周期 -----
        lifecycle:
          type: object
          properties:
            deprecation:
              type: object
              properties:
                successor: { $ref: '#/components/schemas/ResourceRef' }
                sunsetAt:  { type: string, format: date-time }
                migrationGuide: { $ref: '#/components/schemas/ResourceRef' }

    SkillInterface:
      type: object
      required: [input, output]
      properties:
        input:
          type: object
          required: [schema]
          properties:
            schema: { $ref: '#/components/schemas/JSONSchema' }
        output:
          type: object
          required: [schema]
          properties:
            schema: { $ref: '#/components/schemas/JSONSchema' }
        examples:
          type: array
          items:
            type: object
            properties:
              input:  { type: object, x-kubernetes-preserve-unknown-fields: true }
              output: { type: object, x-kubernetes-preserve-unknown-fields: true }
              note:   { type: string }

    SkillImplementation:
      type: object
      required: [type]
      properties:
        type:
          type: string
          enum: [function, workflow, agentic, mcp_tool, external_api]
        runtime:
          type: object
          properties:
            engine:
              type: string
              example: aik-runtime/v2
              description: aik-runtime/v2 | langgraph | temporal | custom
            entrypoint: { type: string, example: 'skills.legal.contract_review:run' }
            image:      { type: string, example: 'registry.ai-keeper.io/skills/contract-review:1.2.0' }
        promptTemplate:
          type: object
          properties:
            ref: { $ref: '#/components/schemas/ResourceRef' }
            inline: { type: string }
        requires:
          type: object
          properties:
            models:
              type: array
              items:
                type: object
                required: [alias, ref]
                properties:
                  alias:    { type: string, example: reasoner }
                  ref:      { $ref: '#/components/schemas/ResourceRef' }
                  purpose:
                    type: string
                    enum: [reasoning, embedding, vision, code, classification, rerank]
                  fallback:
                    type: array
                    items: { $ref: '#/components/schemas/ResourceRef' }
            tools:
              type: array
              items:
                type: object
                required: [ref]
                properties:
                  ref: { $ref: '#/components/schemas/ResourceRef' }
            dataSources:
              type: array
              items:
                type: object
                required: [ref]
                properties:
                  ref: { $ref: '#/components/schemas/ResourceRef' }
            skills:
              type: array
              items:
                type: object
                required: [ref]
                properties:
                  ref: { $ref: '#/components/schemas/ResourceRef' }
                  versionConstraint: { $ref: '#/components/schemas/VersionConstraint' }

    SkillEvaluation:
      type: object
      properties:
        evalSet:    { $ref: '#/components/schemas/ResourceRef' }
        redTeamSet: { $ref: '#/components/schemas/ResourceRef' }
        gates:
          type: object
          additionalProperties:
            type: object
            description: "晋升闸门,key=目标 stage,value=指标=>表达式"
            additionalProperties: { type: string }
        schedule:
          type: string
          description: cron 表达式
          example: "0 2 * * *"

    SkillStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            health:
              type: object
              properties:
                p95LatencyMs:    { type: integer }
                successRate:     { type: number }
                costPerCallUsd:  { type: number }
                last24hInvocations: { type: integer, format: int64 }
            evalResults:
              type: object
              properties:
                lastRunAt: { type: string, format: date-time }
                metrics:
                  type: object
                  additionalProperties: { type: number }
            resolvedDependencies:
              type: object
              description: 控制器解析后的具体版本
              properties:
                models:
                  type: array
                  items:
                    type: object
                    properties:
                      alias: { type: string }
                      resolvedRef: { $ref: '#/components/schemas/ResourceRef' }
                tools:
                  type: array
                  items: { $ref: '#/components/schemas/ResourceRef' }
```

### 5.2 Tool

原子工具(多数为 MCP Server 的具体能力)。

```yaml
components:
  schemas:
    Tool:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [skill.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [Tool] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/ToolSpec' }
        status:     { $ref: '#/components/schemas/ToolStatus' }

    ToolSpec:
      type: object
      required: [protocol, endpoint, schema, governance]
      properties:
        protocol:
          type: string
          enum: [mcp, openapi, grpc, builtin, http]
        endpoint:
          type: string
          description: 协议相关的 endpoint URL
          example: "mcp://docusign-server.legal.svc/get_document"
        authentication:
          type: object
          required: [mode]
          properties:
            mode:
              type: string
              enum: [none, api_key, oauth2_client_credentials, oauth2_obo, mtls, spiffe]
            secretRef:
              type: object
              properties:
                name: { type: string }
                key:  { type: string }
            tokenExchangeRef: { type: string, example: oidc-exchange }
        schema:
          type: object
          required: [input, output]
          properties:
            input:  { $ref: '#/components/schemas/JSONSchema' }
            output: { $ref: '#/components/schemas/JSONSchema' }
        governance:
          allOf:
            - $ref: '#/components/schemas/GovernanceBlock'
            - type: object
              properties:
                sideEffects:
                  type: string
                  enum: [read_only, write, destructive, external]
                requiresApproval: { type: boolean, default: false }
        cost:
          type: object
          properties:
            perCallUsd: { type: number, minimum: 0 }
        rateLimit:
          type: object
          properties:
            perAgentPerMinute: { type: integer, minimum: 0 }
            perTenantPerMinute: { type: integer, minimum: 0 }
            burst: { type: integer, minimum: 0 }
        reliability: { $ref: '#/components/schemas/ReliabilityBlock' }

    ToolStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            reachable: { type: boolean }
            lastProbeAt: { type: string, format: date-time }
            invocations24h: { type: integer, format: int64 }
            errorRate24h: { type: number }
```

---

## 6. agent.ai-keeper.io

### 6.1 Agent

面向用户的执行主体 = Skills 组合 + 身份 + 运行时约束 + 安全护栏 + 接入渠道。

```yaml
components:
  schemas:
    Agent:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [agent.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [Agent] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/AgentSpec' }
        status:     { $ref: '#/components/schemas/AgentStatus' }

    AgentSpec:
      type: object
      required: [displayName, identity, skills, runtime]
      properties:
        displayName: { type: string, maxLength: 200 }
        description: { type: string, maxLength: 2000 }

        # ----- 身份 -----
        identity:
          type: object
          required: [serviceAccount]
          properties:
            serviceAccount: { type: string }
            representation:
              type: object
              properties:
                mode:
                  type: string
                  enum: [self, service_account, on_behalf_of]
                  default: service_account
                requireUserContext: { type: boolean, default: false }
                tokenExchange:      { type: string }

        # ----- 能力 -----
        skills:
          type: array
          minItems: 1
          items:
            type: object
            required: [ref]
            properties:
              ref: { $ref: '#/components/schemas/ResourceRef' }
              versionConstraint: { $ref: '#/components/schemas/VersionConstraint' }
              enabled: { type: boolean, default: true }
              alias:   { type: string }

        # ----- 记忆 -----
        memory:
          type: object
          properties:
            shortTerm:
              type: object
              properties:
                type:
                  type: string
                  enum: [conversation, summary, none]
                window:  { type: integer, minimum: 0 }
                ttl:     { $ref: '#/components/schemas/Duration' }
                storage: { $ref: '#/components/schemas/ResourceRef' }
            longTerm:
              type: object
              properties:
                type:
                  type: string
                  enum: [vector, kv, graph, none]
                ref:        { $ref: '#/components/schemas/ResourceRef' }
                isolation:
                  type: string
                  enum: [shared, per_user, per_session, per_tenant]
                  default: per_user
                writePolicy:
                  type: string
                  enum: [auto, explicit_only, manual_review]
                  default: explicit_only
                retention:  { $ref: '#/components/schemas/Duration' }

        # ----- 运行时 -----
        runtime:
          type: object
          required: [pattern]
          properties:
            pattern:
              type: string
              enum: [react, plan_execute, reflection, workflow, tool_calling, multi_agent]
            maxSteps:     { type: integer, minimum: 1, maximum: 100, default: 15 }
            maxToolCalls: { type: integer, minimum: 1, maximum: 200, default: 30 }
            timeout:      { $ref: '#/components/schemas/Duration' }
            parallelism:  { type: integer, minimum: 1, maximum: 32, default: 1 }
            determinism:
              type: object
              properties:
                temperature: { type: number, minimum: 0, maximum: 2 }
                topP:        { type: number, minimum: 0, maximum: 1 }
                seed:        { type: integer, nullable: true }
            sandbox:
              type: object
              properties:
                enabled: { type: boolean, default: false }
                type:
                  type: string
                  enum: [none, gvisor, firecracker, kata, e2b]
                networkPolicy:
                  type: string
                  enum: [deny_all, allow_list, allow_all]
                  default: deny_all
                egressAllowList:
                  type: array
                  items: { type: string }
                cpuLimit:    { type: string, example: "1000m" }
                memoryLimit: { type: string, example: "1Gi" }
            budget:
              type: object
              properties:
                tokensPerSession: { type: integer, minimum: 0 }
                usdPerSession:    { type: number, minimum: 0 }
                tokensPerStep:    { type: integer, minimum: 0 }
                onExceed:
                  type: string
                  enum: [warn, terminate, request_approval]
                  default: terminate

        # ----- 护栏 -----
        guardrails:
          $ref: '#/components/schemas/GuardrailsBlock'

        # ----- 审计 -----
        audit:
          type: object
          properties:
            level:
              type: string
              enum: [off, basic, high, forensic]
              default: basic
            retention: { $ref: '#/components/schemas/Duration' }
            redactPII: { type: boolean, default: true }
            storeRaw:
              type: object
              properties:
                prompts:
                  type: string
                  enum: [full, hashed, none]
                  default: hashed
                outputs:
                  type: string
                  enum: [full, hashed, none]
                  default: full
                toolIo:
                  type: string
                  enum: [full, hashed, none]
                  default: full
            forwarders:
              type: array
              items:
                type: object
                required: [kind, ref]
                properties:
                  kind:
                    type: string
                    enum: [SIEM, Webhook, Kafka, S3, Elasticsearch]
                  ref: { $ref: '#/components/schemas/ResourceRef' }

        # ----- 部署 -----
        deployment:
          type: object
          properties:
            replicas:  { type: integer, minimum: 0, default: 1 }
            autoscale:
              type: object
              properties:
                min: { type: integer, minimum: 0 }
                max: { type: integer, minimum: 0 }
                targetConcurrency: { type: integer, minimum: 1 }
            placement:
              type: object
              properties:
                zones:
                  type: array
                  items: { type: string }
                compliance:
                  type: array
                  items: { type: string }
                airGapped: { type: boolean, default: false }
                nodeSelector:
                  type: object
                  additionalProperties: { type: string }
            rollout:
              type: object
              properties:
                strategy:
                  type: string
                  enum: [recreate, rolling, canary, blue_green]
                  default: rolling
                steps:
                  type: array
                  items:
                    type: string
                    pattern: '^\d{1,3}%$'
                  example: ["10%", "30%", "100%"]
                analysisInterval: { $ref: '#/components/schemas/Duration' }
                analysisRef:     { $ref: '#/components/schemas/ResourceRef' }

        # ----- 渠道 -----
        channels:
          type: array
          items:
            type: object
            required: [kind]
            properties:
              kind:
                type: string
                enum: [feishu, wecom, dingtalk, slack, teams, web, api, sdk, voice, email]
              ref: { $ref: '#/components/schemas/ResourceRef' }
              auth: { type: string }
              rateLimit:
                type: object
                properties:
                  requestsPerMinute: { type: integer, minimum: 0 }
                  concurrentSessions: { type: integer, minimum: 0 }

    GuardrailsBlock:
      type: object
      properties:
        input:
          type: array
          items: { $ref: '#/components/schemas/GuardrailRule' }
        output:
          type: array
          items: { $ref: '#/components/schemas/GuardrailRule' }
        behavior:
          type: object
          properties:
            systemPrompt: { $ref: '#/components/schemas/ResourceRef' }
            blockedTopics:
              type: array
              items: { type: string }
            allowedTopics:
              type: array
              items: { type: string }
            requiredCitations: { type: boolean, default: false }
            languageLock:
              type: array
              items:
                type: string
                pattern: '^[a-z]{2}(-[A-Z]{2})?$'

    GuardrailRule:
      type: object
      required: [kind]
      properties:
        kind:
          type: string
          enum:
            - PromptInjection
            - Jailbreak
            - PII
            - PIILeak
            - Toxicity
            - Hallucination
            - Grounding
            - ClassificationLeak
            - Bias
            - Profanity
            - Custom
        provider:
          type: string
          example: aik-builtin
          description: aik-builtin | llamaguard-v3 | nemo-guardrails | custom
        action:
          type: string
          enum: [allow, mask, block, warn, escalate]
        threshold: { type: number, minimum: 0, maximum: 1 }
        rule:
          type: string
          description: 自定义规则(CEL/regex)
        config:
          type: object
          x-kubernetes-preserve-unknown-fields: true

    AgentStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            readyReplicas: { type: integer }
            attachedSkills:
              type: array
              items: { $ref: '#/components/schemas/ResourceRef' }
            effectivePolicies:
              type: array
              items: { type: string }
            metrics:
              type: object
              properties:
                activeUsers: { type: integer }
                today:
                  type: object
                  properties:
                    invocations: { type: integer, format: int64 }
                    costUsd: { type: number }
                    p95LatencyMs: { type: integer }
                    errorRate: { type: number }
            rolloutStatus:
              type: object
              properties:
                phase:
                  type: string
                  enum: [Progressing, Promoting, Paused, Aborted, Succeeded]
                currentStep: { type: integer }
                trafficWeight: { type: integer, minimum: 0, maximum: 100 }
```

---

## 7. policy.ai-keeper.io

### 7.1 Policy

ABAC + Obligations 模型的统一授权语言。

```yaml
components:
  schemas:
    Policy:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [policy.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [Policy] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/PolicySpec' }
        status:     { $ref: '#/components/schemas/PolicyStatus' }

    PolicySpec:
      type: object
      required: [effect, subject, action]
      properties:
        # ----- 元属性 -----
        effect:
          type: string
          enum: [allow, deny]
        priority:
          type: integer
          minimum: 0
          maximum: 1000
          default: 100
          description: 高优先级胜出;同优先级 deny 胜
        enabled: { type: boolean, default: true }
        effectiveWindow:
          type: object
          properties:
            notBefore: { type: string, format: date-time }
            notAfter:  { type: string, format: date-time }

        # ----- Subject -----
        subject:
          $ref: '#/components/schemas/SubjectSelector'

        # ----- Action -----
        action:
          type: object
          required: [verbs, resources]
          properties:
            verbs:
              type: array
              minItems: 1
              items:
                type: string
                enum: [invoke, read, write, delete, admin, list, watch, train, evaluate, deploy]
            resources:
              type: object
              required: [anyOf]
              properties:
                anyOf:
                  type: array
                  minItems: 1
                  items: { $ref: '#/components/schemas/ResourceSelector' }

        # ----- Conditions -----
        conditions:
          $ref: '#/components/schemas/ConditionSet'

        # ----- Constraints -----
        constraints:
          type: object
          properties:
            budget:
              type: object
              properties:
                tokensPerUserPerDay:    { type: integer, minimum: 0 }
                tokensPerUserPerMonth:  { type: integer, minimum: 0 }
                usdPerUserPerMonth:     { type: number, minimum: 0 }
                tokensPerRequest:       { type: integer, minimum: 0 }
            rateLimit:
              type: object
              properties:
                requestsPerMinute:   { type: integer, minimum: 0 }
                requestsPerHour:     { type: integer, minimum: 0 }
                concurrentSessions:  { type: integer, minimum: 0 }
            quota:
              $ref: '#/components/schemas/ResourceRef'

        # ----- Approvals -----
        approvals:
          type: array
          items:
            type: object
            required: [when, approver]
            properties:
              when:
                type: object
                properties:
                  expression: { type: string }
              approver:
                type: object
                required: [kind, name]
                properties:
                  kind:
                    type: string
                    enum: [User, Role, Group]
                  name: { type: string }
              timeout:    { $ref: '#/components/schemas/Duration' }
              ifTimeout:
                type: string
                enum: [allow, deny, escalate]
                default: deny

        # ----- Obligations -----
        obligations:
          type: object
          properties:
            audit:
              type: object
              properties:
                level:
                  type: string
                  enum: [off, basic, high, forensic]
                includePromptHashes: { type: boolean }
                forwardTo:
                  type: array
                  items: { $ref: '#/components/schemas/ResourceRef' }
            redact:
              type: object
              properties:
                patternsRef: { $ref: '#/components/schemas/ResourceRef' }
                fields:
                  type: array
                  items: { type: string }
            notify:
              type: object
              properties:
                onMatch:
                  type: array
                  items:
                    type: object
                    required: [condition, channel]
                    properties:
                      condition: { type: string }
                      channel:   { type: string }
            watermark:
              type: object
              properties:
                enabled: { type: boolean, default: false }
                mode:
                  type: string
                  enum: [visible, invisible, both]
                text: { type: string }

    SubjectSelector:
      type: object
      properties:
        anyOf:
          type: array
          minItems: 1
          items:
            type: object
            required: [kind]
            properties:
              kind:
                type: string
                enum: [User, Role, Group, Agent, ServiceAccount, Tenant, Anonymous]
              match:
                type: object
                properties:
                  name: { type: string }
                  namespace: { type: string }
                  labels:
                    type: object
                    additionalProperties: { type: string }
                  attributes:
                    type: object
                    description: 属性匹配,值可为标量或匹配器对象
                    additionalProperties:
                      x-kubernetes-preserve-unknown-fields: true

    ResourceSelector:
      type: object
      required: [kind]
      properties:
        kind:
          type: string
          enum: [Skill, Agent, Tool, ModelEndpoint, DataSource, KnowledgeBase, Channel, Any]
        match:
          type: object
          properties:
            name: { type: string }
            namespace: { type: string }
            ref: { $ref: '#/components/schemas/ResourceRef' }
            labels:
              type: object
              additionalProperties: { type: string }
            classification:
              type: string
              description: "比较表达式,如 '<= confidential'"

    ConditionSet:
      type: object
      properties:
        allOf:
          type: array
          items: { $ref: '#/components/schemas/ConditionItem' }
        anyOf:
          type: array
          items: { $ref: '#/components/schemas/ConditionItem' }
        noneOf:
          type: array
          items: { $ref: '#/components/schemas/ConditionItem' }

    ConditionItem:
      type: object
      description: 多种内置条件 + CEL 兜底,任一字段被设置即生效
      properties:
        timeWindow:
          type: object
          properties:
            schedule:  { type: string, example: "Mon-Fri 08:00-20:00" }
            timezone:  { type: string, example: "Asia/Shanghai" }
        location:
          type: object
          properties:
            countries:
              type: array
              items:
                type: string
                pattern: '^[A-Z]{2}$'
            regions:
              type: array
              items: { type: string }
            ipAllowList:
              type: array
              items:
                type: string
                description: CIDR
            ipDenyList:
              type: array
              items: { type: string }
        dataClassificationCeiling:
          $ref: '#/components/schemas/Classification'
        riskScore:
          type: object
          properties:
            min: { type: number, minimum: 0, maximum: 1 }
            max: { type: number, minimum: 0, maximum: 1 }
        require:
          type: object
          properties:
            mfa: { type: boolean }
            deviceCompliant: { type: boolean }
            sso: { type: boolean }
            stepUpAuth: { type: boolean }
        expression:
          type: string
          description: CEL 表达式,可访问 request.user / agent / resource / context 上下文

    PolicyStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            evaluationCount24h: { type: integer, format: int64 }
            decisions24h:
              type: object
              properties:
                allow: { type: integer, format: int64 }
                deny:  { type: integer, format: int64 }
                requireApproval: { type: integer, format: int64 }
            conflicts:
              type: array
              items:
                type: object
                properties:
                  conflictsWith: { type: string }
                  reason: { type: string }
```

### 7.2 Budget

```yaml
components:
  schemas:
    Budget:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [policy.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [Budget] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/BudgetSpec' }
        status:     { $ref: '#/components/schemas/BudgetStatus' }

    BudgetSpec:
      type: object
      required: [scope, period, limits]
      properties:
        scope:
          type: object
          required: [kind, name]
          properties:
            kind:
              type: string
              enum: [Tenant, Team, User, Agent, Skill, Project]
            name: { type: string }
        period:
          type: string
          enum: [hourly, daily, weekly, monthly, quarterly, yearly]
        limits:
          type: object
          properties:
            usd:    { type: number, minimum: 0 }
            tokens: { type: integer, format: int64, minimum: 0 }
            calls:  { type: integer, format: int64, minimum: 0 }
        alerts:
          type: array
          items:
            type: object
            required: [threshold, channels]
            properties:
              threshold:
                type: string
                pattern: '^\d{1,3}%$'
                example: "80%"
              channels:
                type: array
                items: { type: string }
              action:
                type: string
                enum: [notify, throttle, block]
                default: notify
        rollover: { type: boolean, default: false }
        hardCap: { type: boolean, default: true, description: 是否在达到 100% 时硬阻断 }

    BudgetStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            current:
              type: object
              properties:
                usd: { type: number }
                tokens: { type: integer, format: int64 }
                calls: { type: integer, format: int64 }
            periodStart: { type: string, format: date-time }
            periodEnd:   { type: string, format: date-time }
            daysRemaining: { type: integer }
            burnRate:
              type: string
              enum: [ok, warning, critical, exhausted]
            projectedExhaustionAt: { type: string, format: date-time }
```

### 7.3 Quota

```yaml
components:
  schemas:
    Quota:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [policy.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [Quota] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/QuotaSpec' }
        status:     { $ref: '#/components/schemas/QuotaStatus' }

    QuotaSpec:
      type: object
      required: [scope, limits]
      properties:
        scope:
          type: object
          required: [kind, name]
          properties:
            kind:
              type: string
              enum: [Tenant, Namespace, Team, User]
            name: { type: string }
        limits:
          type: object
          additionalProperties:
            description: "key 为资源类型(agents/skills/models),value 为整数上限"
            oneOf:
              - { type: integer, minimum: 0 }
              - { type: string }
          example:
            agents: 50
            skills: 200
            modelEndpoints: 20
            knowledgeBases: 30
            tokensPerMonth: "100000000"

    QuotaStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            used:
              type: object
              additionalProperties:
                oneOf:
                  - { type: integer }
                  - { type: string }
```

---

## 8. data.ai-keeper.io

### 8.1 DataSource

通过 MCP / 连接器接入的数据源(读、写、流式)。

```yaml
components:
  schemas:
    DataSource:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [data.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [DataSource] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/DataSourceSpec' }
        status:     { $ref: '#/components/schemas/DataSourceStatus' }

    DataSourceSpec:
      type: object
      required: [connector]
      properties:
        connector:
          type: object
          required: [kind]
          properties:
            kind:
              type: string
              description: 连接器类型
              enum:
                - feishu_wiki
                - feishu_doc
                - wecom_doc
                - confluence
                - notion
                - sharepoint
                - jira
                - gitlab
                - github
                - postgres
                - mysql
                - mongodb
                - elasticsearch
                - snowflake
                - databricks
                - s3
                - oss
                - kafka
                - mcp_generic
                - http_api
                - custom
            ref: { $ref: '#/components/schemas/ResourceRef' }
            config:
              type: object
              x-kubernetes-preserve-unknown-fields: true
        authentication:
          type: object
          properties:
            mode:
              type: string
              enum: [none, api_key, oauth2, oauth2_obo, basic, mtls, iam]
            secretRef:
              type: object
              properties:
                name: { type: string }
                key:  { type: string }
        accessMode:
          type: string
          enum: [read_only, read_write, write_only, stream]
          default: read_only
        acl:
          type: object
          properties:
            mode:
              type: string
              enum: [open, inherit_from_source, custom, deny_all]
              default: inherit_from_source
            enforcement:
              type: string
              enum: [pre_filter, post_filter, hybrid]
              default: pre_filter
            policyRef: { $ref: '#/components/schemas/ResourceRef' }
        governance:
          allOf:
            - $ref: '#/components/schemas/GovernanceBlock'
            - type: object
              properties:
                retention: { $ref: '#/components/schemas/Duration' }
                deletionPolicy:
                  type: string
                  enum: [soft, hard, never]

    DataSourceStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            connected: { type: boolean }
            lastSyncAt: { type: string, format: date-time }
            documentCount: { type: integer, format: int64 }
            sizeBytes: { type: integer, format: int64 }
```

### 8.2 KnowledgeBase

由一个或多个 DataSource 组成的检索资产(向量 + 图 + 全文)。

```yaml
components:
  schemas:
    KnowledgeBase:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [data.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [KnowledgeBase] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/KnowledgeBaseSpec' }
        status:     { $ref: '#/components/schemas/KnowledgeBaseStatus' }

    KnowledgeBaseSpec:
      type: object
      required: [sources, pipeline, index]
      properties:
        sources:
          type: array
          minItems: 1
          items:
            type: object
            required: [ref]
            properties:
              ref: { $ref: '#/components/schemas/ResourceRef' }
              sync:
                type: object
                properties:
                  mode:
                    type: string
                    enum: [full, incremental, cdc, manual]
                    default: incremental
                  schedule:
                    type: string
                    description: cron
                  watermarkField: { type: string }
        pipeline:
          type: object
          properties:
            chunking:
              type: object
              properties:
                strategy:
                  type: string
                  enum: [fixed, semantic, recursive, structural, hybrid]
                  default: semantic
                maxTokens: { type: integer, minimum: 64, maximum: 8192 }
                overlap:   { type: integer, minimum: 0 }
            embedding:
              type: object
              properties:
                ref: { $ref: '#/components/schemas/ResourceRef' }
                dimensions: { type: integer }
            enrichment:
              type: array
              items:
                type: string
                enum:
                  - pii_tagging
                  - classification_inheritance
                  - language_detection
                  - entity_extraction
                  - summary
                  - keyword_extraction
                  - custom
        index:
          type: object
          required: [vectorStore]
          properties:
            vectorStore: { $ref: '#/components/schemas/ResourceRef' }
            graphStore:  { $ref: '#/components/schemas/ResourceRef' }
            fullTextStore: { $ref: '#/components/schemas/ResourceRef' }
            hybridSearch: { type: boolean, default: true }
            multiTenant: { type: boolean, default: true }
        retrieval:
          type: object
          properties:
            topK: { type: integer, minimum: 1, maximum: 100, default: 8 }
            reranker: { $ref: '#/components/schemas/ResourceRef' }
            filters:
              type: array
              items:
                type: string
                description: CEL 表达式,基于 metadata 过滤
        acl:
          type: object
          properties:
            mode:
              type: string
              enum: [inherit_from_source, custom]
              default: inherit_from_source
            enforcement:
              type: string
              enum: [pre_filter, post_filter]
              default: pre_filter
        governance:
          allOf:
            - $ref: '#/components/schemas/GovernanceBlock'
            - type: object
              properties:
                retention: { $ref: '#/components/schemas/Duration' }

    KnowledgeBaseStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            documentCount: { type: integer, format: int64 }
            chunkCount:    { type: integer, format: int64 }
            indexSizeBytes: { type: integer, format: int64 }
            lastIndexedAt: { type: string, format: date-time }
            qualityMetrics:
              type: object
              properties:
                avgRecall: { type: number }
                avgMRR:    { type: number }
                lastEvalAt: { type: string, format: date-time }
```

---

## 9. model.ai-keeper.io

### 9.1 ModelEndpoint

具体的模型实例(公有云 API / 私有部署 / 信创栈)。

```yaml
components:
  schemas:
    ModelEndpoint:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [model.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [ModelEndpoint] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/ModelEndpointSpec' }
        status:     { $ref: '#/components/schemas/ModelEndpointStatus' }

    ModelEndpointSpec:
      type: object
      required: [provider, model, endpoint]
      properties:
        provider:
          type: string
          enum:
            - openai
            - anthropic
            - azure_openai
            - bedrock
            - vertex
            - aliyun_dashscope
            - tencent_hunyuan
            - volcengine_ark
            - baichuan
            - moonshot
            - deepseek
            - zhipu
            - minimax
            - self_hosted
            - vllm
            - sglang
            - tgi
            - ollama
            - custom
        model: { type: string, example: "gpt-4o-2024-11-20" }
        endpoint:
          type: string
          format: uri
        region: { type: string, example: "eu-west" }
        authentication:
          type: object
          required: [mode]
          properties:
            mode:
              type: string
              enum: [api_key, oauth2, iam, mtls, none]
            secretRef:
              type: object
              properties:
                name: { type: string }
                key:  { type: string }
        capabilities:
          type: array
          items:
            type: string
            enum:
              - chat
              - completion
              - embedding
              - rerank
              - function_calling
              - tool_use
              - vision
              - audio_in
              - audio_out
              - video
              - reasoning
              - structured_output
        contextWindow: { type: integer, minimum: 0 }
        maxOutputTokens: { type: integer, minimum: 0 }
        cost:
          type: object
          properties:
            inputUsdPerMTok:  { type: number, minimum: 0 }
            outputUsdPerMTok: { type: number, minimum: 0 }
            cachedInputUsdPerMTok: { type: number, minimum: 0 }
            embeddingUsdPerMTok: { type: number, minimum: 0 }
        compliance:
          type: array
          items:
            type: string
            example: GDPR
        dataResidency:
          type: object
          properties:
            primaryRegion: { type: string }
            sovereign: { type: boolean, default: false }
        quota:
          type: object
          properties:
            tpm: { type: integer, minimum: 0, description: tokens per minute }
            rpm: { type: integer, minimum: 0, description: requests per minute }
            tpd: { type: integer, format: int64, minimum: 0 }
        privacy:
          type: object
          properties:
            trainOnInputs: { type: boolean, default: false }
            zeroRetention: { type: boolean, default: false }
            dpaSigned: { type: boolean }
        fallback:
          type: array
          items: { $ref: '#/components/schemas/ResourceRef' }
        deployment:
          type: object
          properties:
            mode:
              type: string
              enum: [public_api, private_api, dedicated, on_premise, edge]
            kserveRef:
              type: string
              description: KServe InferenceService 引用(自部署场景)

    ModelEndpointStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            healthy: { type: boolean }
            lastProbeAt: { type: string, format: date-time }
            currentTpm: { type: integer }
            currentRpm: { type: integer }
            errorRate24h: { type: number }
            avgLatencyMs: { type: integer }
```

### 9.2 ModelRouter

把"逻辑模型别名"路由到一个或多个具体 ModelEndpoint。

```yaml
components:
  schemas:
    ModelRouter:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [model.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [ModelRouter] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/ModelRouterSpec' }
        status:     { $ref: '#/components/schemas/ModelRouterStatus' }

    ModelRouterSpec:
      type: object
      required: [alias, rules]
      properties:
        alias:
          type: string
          description: 上游使用的逻辑别名,如 reasoner、embedder
          example: reasoner
        defaultEndpoint: { $ref: '#/components/schemas/ResourceRef' }
        rules:
          type: array
          minItems: 1
          items:
            type: object
            required: [endpoint]
            properties:
              when:
                type: object
                description: 命中条件
                properties:
                  expression: { type: string, description: CEL,可访问 request 上下文 }
                  taskType:
                    type: string
                    enum: [chat, classify, extract, summarize, code, math, vision, embedding]
                  contextLengthMin: { type: integer }
                  costSensitive: { type: boolean }
                  latencySensitive: { type: boolean }
                  compliance:
                    type: array
                    items: { type: string }
                  tenant: { type: string }
              endpoint: { $ref: '#/components/schemas/ResourceRef' }
              weight:
                type: integer
                minimum: 0
                maximum: 100
                description: 同一规则下多 endpoint 的流量权重
        cache:
          type: object
          properties:
            enabled: { type: boolean, default: false }
            mode:
              type: string
              enum: [exact, semantic, hybrid]
            ttl: { $ref: '#/components/schemas/Duration' }
            similarityThreshold: { type: number, minimum: 0, maximum: 1 }
            ref: { $ref: '#/components/schemas/ResourceRef' }
        loadBalancing:
          type: string
          enum: [round_robin, least_latency, least_cost, weighted, sticky_session]
          default: weighted

    ModelRouterStatus:
      allOf:
        - $ref: '#/components/schemas/StatusBase'
        - type: object
          properties:
            requestsRouted24h: { type: integer, format: int64 }
            cacheHitRate: { type: number }
            distribution:
              type: array
              items:
                type: object
                properties:
                  endpoint: { $ref: '#/components/schemas/ResourceRef' }
                  weight: { type: integer }
                  requests24h: { type: integer, format: int64 }
```

---

## 10. audit.ai-keeper.io

### 10.1 AuditEvent

只读资源,由系统在每次调用后产生,用于合规、追溯、计费、回溯调试。

```yaml
components:
  schemas:
    AuditEvent:
      type: object
      required: [apiVersion, kind, metadata, spec]
      properties:
        apiVersion: { type: string, enum: [audit.ai-keeper.io/v1alpha1] }
        kind:       { type: string, enum: [AuditEvent] }
        metadata:   { $ref: '#/components/schemas/ObjectMeta' }
        spec:       { $ref: '#/components/schemas/AuditEventSpec' }

    AuditEventSpec:
      type: object
      required: [invocationId, timestamp, principal, action]
      properties:
        invocationId:
          type: string
          format: uuid
        timestamp:
          type: string
          format: date-time
        trace:
          type: object
          properties:
            traceId: { type: string }
            spanId:  { type: string }
            parentSpanId: { type: string }

        # ----- Who -----
        principal:
          type: object
          required: [agent]
          properties:
            user:
              type: object
              properties:
                id: { type: string }
                tenantId: { type: string }
                department: { type: string }
                attributes:
                  type: object
                  additionalProperties: { type: string }
            agent:
              type: object
              required: [name]
              properties:
                name: { type: string }
                namespace: { type: string }
                version: { type: string }
            serviceAccount:
              type: object
              properties:
                name: { type: string }
                spiffeId: { type: string }
            onBehalfOf:
              type: string
              description: 终端用户 ID(OBO 模式)
            sourceIp: { type: string }
            userAgent: { type: string }
            channel:
              type: string
              enum: [feishu, wecom, dingtalk, slack, teams, web, api, sdk, voice, email, internal]

        # ----- What -----
        action:
          type: object
          required: [verb, resource]
          properties:
            verb:
              type: string
              enum: [invoke, read, write, delete, admin, list, watch]
            resource: { $ref: '#/components/schemas/ResourceRef' }
            method:
              type: string
              description: 工具/API 方法名

        # ----- Decision -----
        policy:
          type: object
          required: [decision]
          properties:
            decision:
              type: string
              enum: [allow, deny, require_approval, error]
            matchedPolicies:
              type: array
              items: { type: string }
            reason: { type: string }
            obligationsApplied:
              type: array
              items: { type: string }
            approvalId: { type: string }

        # ----- IO -----
        request:
          type: object
          properties:
            inputHash:
              type: string
              pattern: '^sha256:[a-f0-9]{64}$'
            inputRef: { $ref: '#/components/schemas/ResourceRef' }
            classification: { $ref: '#/components/schemas/Classification' }
            redactions:
              type: array
              items: { type: string }
            sizeBytes: { type: integer, format: int64 }
            language: { type: string }
        response:
          type: object
          properties:
            outputHash:
              type: string
              pattern: '^sha256:[a-f0-9]{64}$'
            outputRef: { $ref: '#/components/schemas/ResourceRef' }
            confidence: { type: number, minimum: 0, maximum: 1 }
            citations:
              type: array
              items:
                type: object
                properties:
                  source: { type: string }
                  chunkId: { type: string }
                  score:  { type: number }
            classification: { $ref: '#/components/schemas/Classification' }
            sizeBytes: { type: integer, format: int64 }

        # ----- Steps -----
        steps:
          type: array
          description: Agent 思维链中每一步的工具调用
          items:
            type: object
            properties:
              index: { type: integer }
              type:
                type: string
                enum: [thought, tool_call, model_call, observation, final]
              tool: { $ref: '#/components/schemas/ResourceRef' }
              model: { $ref: '#/components/schemas/ResourceRef' }
              tokensIn: { type: integer }
              tokensOut: { type: integer }
              latencyMs: { type: integer }
              error: { type: string }

        # ----- Cost -----
        cost:
          type: object
          properties:
            tokens:
              type: object
              properties:
                input:  { type: integer }
                output: { type: integer }
                cached: { type: integer }
            usd: { type: number }
            durationMs: { type: integer }

        # ----- Safety -----
        guardrails:
          type: object
          properties:
            triggered:
              type: array
              items:
                type: object
                properties:
                  rule:    { type: string }
                  stage:   { type: string, enum: [input, output, behavior] }
                  action:  { type: string, enum: [allow, mask, block, warn, escalate] }
                  score:   { type: number }
            blocked: { type: boolean }

        # ----- Compliance -----
        compliance:
          type: object
          properties:
            tags:
              type: array
              items: { type: string }
            dataResidency: { type: string }
            reviewed: { type: boolean, default: false }
            reviewedBy: { type: string }
            reviewedAt: { type: string, format: date-time }
            holds:
              type: array
              description: 法律保留(legal hold)标签
              items: { type: string }

        # ----- Outcome -----
        outcome:
          type: object
          properties:
            status:
              type: string
              enum: [success, partial, failed, timeout, blocked, cancelled]
            errorCode: { type: string }
            errorMessage: { type: string }

      additionalProperties: false
```

> AuditEvent 对外**只读**,集群中可启用 `validatingAdmissionPolicy` 拒绝任何 `CREATE/UPDATE/DELETE`(系统组件除外)。
> 推荐落 ClickHouse + S3(对象锁/WORM),CRD 仅作为元数据视图。

---

## 11. K8s CRD 包装示例

把上面任意一个资源的 schema 套成正式的 K8s CRD 时,使用 `apiextensions.k8s.io/v1` 外壳。
以 `Skill` 为例(简化版,完整 schema 引用 §5.1 的 `SkillSpec` / `SkillStatus`):

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: skills.skill.ai-keeper.io
spec:
  group: skill.ai-keeper.io
  scope: Namespaced
  names:
    plural: skills
    singular: skill
    kind: Skill
    shortNames: [sk]
    categories: [aik, ai-keeper]
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          required: [spec]
          properties:
            apiVersion: { type: string }
            kind:       { type: string }
            metadata:   { type: object }
            spec:
              # 把 §5.1 的 SkillSpec 内联展开到这里(K8s CRD 不支持外部 $ref)
              type: object
              required: [version, stability, interface, implementation]
              properties:
                version:   { type: string }
                stability:
                  type: string
                  enum: [experimental, beta, stable, deprecated]
                # ... 略,完整 schema 参考 §5.1
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
        scale:
          specReplicasPath: .spec.implementation.runtime.replicas
          statusReplicasPath: .status.health.replicas
      additionalPrinterColumns:
        - name: Version
          type: string
          jsonPath: .spec.version
        - name: Stage
          type: string
          jsonPath: .spec.stability
        - name: Phase
          type: string
          jsonPath: .status.phase
        - name: Success
          type: string
          jsonPath: .status.health.successRate
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
      conversion:
        strategy: Webhook
        webhook:
          conversionReviewVersions: [v1, v1alpha1]
          clientConfig:
            service:
              name: aik-conversion-webhook
              namespace: aik-system
              path: /convert
```

> **注意 K8s CRD 的限制**:
> 1. CRD `openAPIV3Schema` 不支持外部 `$ref`,需要把所有引用**内联展开**(可用 `kubebuilder` / `controller-gen` / 自写脚本完成)
> 2. CRD schema 是 OpenAPI v3 + JSON Schema Draft 4 的子集,不支持 `oneOf`/`anyOf` 中的复杂多态(部分支持)
> 3. 自定义校验用 `x-kubernetes-validations`(K8s 1.25+)做 CEL 校验
> 4. `AuditEvent` 数量极大,建议用普通 API + ClickHouse 而非 CRD,只为查询门面保留 schema

每个资源的 CRD 套壳模板基本一致,只需替换 `group`/`names`/`spec`/`additionalPrinterColumns` 四处。

---

## 12. 代码生成与校验

### 12.1 代码生成命令

**Go(K8s controller)**:

```bash
# 用 kubebuilder 创建项目骨架
kubebuilder init --domain ai-keeper.io --repo github.com/yourorg/aip
kubebuilder create api --group skill --version v1alpha1 --kind Skill

# 把本文档中的 SkillSpec/SkillStatus 转成 Go struct,粘贴到 api/v1alpha1/skill_types.go
# 用 controller-gen 自动生成 zz_generated.deepcopy.go 与 CRD YAML
controller-gen object paths=./api/...
controller-gen crd paths=./api/... output:crd:dir=./config/crd/bases
```

**TypeScript / Python(SDK & 客户端)**:

```bash
# 把本文档拆出独立的 OpenAPI 3.0 文件 aik-openapi.yaml 后:
openapi-generator-cli generate -i aik-openapi.yaml -g typescript-axios -o ./sdk/ts
openapi-generator-cli generate -i aik-openapi.yaml -g python -o ./sdk/python
openapi-generator-cli generate -i aik-openapi.yaml -g go -o ./sdk/go
```

**JSON Schema 校验(独立脚本)**:

```bash
# 用 ajv 在 CI 阶段校验 YAML manifests
npm i -g ajv-cli
ajv validate -s ./schemas/skill.json -d ./manifests/legal/*.yaml
```

### 12.2 CRD Linter(`aikctl lint`)建议规则

| 规则 | 级别 | 检查内容 |
|---|---|---|
| `skill/has-eval-set` | error | Skill `stability=stable` 必须配 `evaluation.evalSet` |
| `skill/has-fallback` | warn | 生产 Skill 应配置 `reliability.fallback` |
| `skill/budget-set` | warn | 应配 `cost.budget` 防止意外烧 token |
| `skill/version-bumped` | error | spec 改动必须升 `version` |
| `agent/skills-resolved` | error | 所有 `skills[].ref` 必须存在且版本可解 |
| `agent/sandbox-required` | error | `pattern=react` 且使用 code 工具时,sandbox 必须 enabled |
| `agent/audit-min-level` | warn | `governance.classification >= confidential` 时 audit 至少 high |
| `policy/no-conflict` | error | 同 priority 的 allow/deny 不应作用于完全相同 subject+resource |
| `policy/sane-priority` | warn | 不要使用极端 priority(0/1000) |
| `policy/effective-window` | warn | `effectiveWindow.notAfter` 不应超过 5 年 |
| `tool/destructive-needs-approval` | error | `sideEffects=destructive` 必须设 `requiresApproval=true` |
| `model/dpa-required` | warn | `governance.compliance` 含 GDPR 时,`privacy.dpaSigned` 必须为 true |
| `kb/acl-not-open` | error | `governance.classification >= confidential` 的 KB 不能 `acl.mode=open` |
| `kb/post-filter-warn` | warn | 不推荐 `acl.enforcement=post_filter`(侧信道风险) |

### 12.3 CRD 兼容性策略

| 阶段 | API 版本 | 允许变更 |
|---|---|---|
| 内部孵化(0–6 个月) | `v1alpha1` | 任意破坏性变更,需 conversion webhook |
| 公测(6–18 个月) | `v1beta1` | 新增字段、deprecate 字段;不删字段、不改语义 |
| 正式(18 个月+) | `v1` | 仅新增字段;按 K8s deprecation 策略推进 |

API 演进必须遵守:
1. 字段从 `required` 转为 optional 是兼容变更;反向不兼容
2. 枚举只能新增不能删除/改语义
3. 类型变更必须经 conversion webhook 转换
4. 同一 GVR 至少同时 served 两个相邻版本 6 个月

---

## 附:最小端到端示例

下面是部署一个"法务 Copilot"完整需要的 4 个 manifest(可保存为 `legal-copilot.yaml`,
直接 `kubectl apply -f` 测试 schema 通过性)。

```yaml
---
apiVersion: core.ai-keeper.io/v1alpha1
kind: ServiceAccount
metadata:
  name: legal-copilot-sa
  namespace: legal
spec:
  identityProvider: corp-okta
  attributes:
    team: legal-platform
  allowOnBehalfOf: true

---
apiVersion: skill.ai-keeper.io/v1alpha1
kind: Skill
metadata:
  name: contract-review
  namespace: legal
  labels:
    domain: legal
spec:
  version: "1.2.0"
  stability: stable
  interface:
    input:
      schema:
        type: object
        required: [documentId]
        properties:
          documentId: { type: string, format: uuid }
    output:
      schema:
        type: object
        properties:
          summary: { type: string }
          confidence: { type: number, minimum: 0, maximum: 1 }
  implementation:
    type: agentic
    runtime:
      engine: aik-runtime/v2
      entrypoint: skills.legal.contract_review:run
    promptTemplate:
      ref: prompt://legal/contract-review/v3
    requires:
      models:
        - alias: reasoner
          ref: model://gpt-4o-eu
          purpose: reasoning
          fallback: [model://claude-sonnet-eu]
      tools:
        - ref: tool://docusign/get-document
      dataSources:
        - ref: data://legal-knowledge-base
  governance:
    classification: confidential
    dataResidency:
      allowedRegions: [cn-north, eu-west]
      crossBorder: forbidden
    pii:
      onInput: detect_and_mask
      onOutput: detect_and_block
    compliance:
      required: [GDPR, "等保三级"]
  cost:
    budget:
      tokensPerCall: 50000
      usdPerCall: 0.50
  slo:
    p95LatencyMs: 8000
    successRate: 0.98
  evaluation:
    evalSet: ref://evals/legal/contract-review/v1
    gates:
      promoteToStable:
        accuracy: ">= 0.85"
        grounding: ">= 0.90"

---
apiVersion: agent.ai-keeper.io/v1alpha1
kind: Agent
metadata:
  name: legal-copilot
  namespace: legal
spec:
  displayName: "法务 Copilot"
  identity:
    serviceAccount: legal-copilot-sa
    representation:
      mode: on_behalf_of
      requireUserContext: true
  skills:
    - ref: skill://contract-review
      versionConstraint: ">=1.2.0 <2.0.0"
  runtime:
    pattern: react
    maxSteps: 15
    timeout: 120s
    sandbox:
      enabled: true
      type: firecracker
      networkPolicy: deny_all
    budget:
      tokensPerSession: 200000
      usdPerSession: 2.00
      onExceed: terminate
  guardrails:
    input:
      - kind: PromptInjection
        provider: aik-builtin
        action: block
      - kind: PII
        action: mask
    output:
      - kind: PIILeak
        action: block
      - kind: Hallucination
        threshold: 0.3
        action: warn
  audit:
    level: high
    retention: 7y
    storeRaw:
      prompts: hashed
      outputs: full
  channels:
    - kind: feishu
      ref: channel://feishu-bot-legal

---
apiVersion: policy.ai-keeper.io/v1alpha1
kind: Policy
metadata:
  name: legal-copilot-access
  namespace: legal
spec:
  effect: allow
  priority: 100
  subject:
    anyOf:
      - kind: User
        match:
          attributes:
            department: { in: [Legal, Compliance] }
            mfaEnrolled: true
      - kind: Agent
        match:
          name: legal-copilot
  action:
    verbs: [invoke]
    resources:
      anyOf:
        - kind: Skill
          match:
            labels:
              domain: legal
  conditions:
    allOf:
      - timeWindow:
          schedule: "Mon-Fri 08:00-20:00"
          timezone: Asia/Shanghai
      - location:
          countries: [CN]
      - dataClassificationCeiling: confidential
      - require:
          mfa: true
          deviceCompliant: true
  constraints:
    budget:
      tokensPerUserPerDay: 100000
      usdPerUserPerMonth: 50
    rateLimit:
      requestsPerMinute: 30
  obligations:
    audit:
      level: high
      includePromptHashes: true
    redact:
      patternsRef: ref://patterns/pii-cn
    watermark:
      enabled: true
      mode: invisible
```

---

## 文档版本

| 版本 | 日期 | 说明 |
|---|---|---|
| v1alpha1.0 | 2026-05-26 | 初版,覆盖 Skill / Agent / Policy / Tool / Budget / Quota / DataSource / KnowledgeBase / ModelEndpoint / ModelRouter / Tenant / ServiceAccount / AuditEvent |

> 后续修订:
> - 增补 `Channel`、`PromptTemplate`、`EvalSet`、`Connector`(MCP Server)等子资源
> - 完善 `x-kubernetes-validations` CEL 校验示例
> - 输出独立的 `aik-openapi.yaml`(可直接 codegen)与按资源拆分的 JSON Schema 文件
