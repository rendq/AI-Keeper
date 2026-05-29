# AI Platform 是什么 —— 用图讲清楚 "CRD/Schema 到底要实现啥"

> 你之前已经有了 [`aip-crd-openapi.md`](./aip-crd-openapi.md)（CRD schema）和 [`aip-controllers-reconcile.md`](./aip-controllers-reconcile.md)（控制器状态机）。
> 这一篇是**导览图**：把 schema 放回到整个产品里看，让你一眼明白"这堆 YAML 字段最终要变成什么系统、解决什么问题"。

---

## 1. 一句话先定调

> **CRD 是产品的 "API"，控制器是产品的 "运行时"，PDP/Gateway/Runtime 是产品的 "执行体"。**
>
> 用户写 YAML（声明意图）→ 控制器把它变成可运行的系统 → 数据面在每次调用时执行 schema 里写好的"规矩"。

```mermaid
flowchart LR
    A[👤 用户/平台运营] -- "kubectl apply<br/>aikctl apply" --> B[(CRD<br/>YAML)]
    B -- watch --> C[控制器<br/>Reconcile]
    C -- 创建/编排 --> D[执行体<br/>Gateway/PDP/Agent Runtime]
    E[💼 业务请求] --> D
    D -- 决策/调用/审计 --> F[(模型/工具/数据)]
    D -- events --> G[(审计/计费/<br/>可观测)]

    style B fill:#fff3cd,stroke:#856404
    style C fill:#d1ecf1,stroke:#0c5460
    style D fill:#d4edda,stroke:#155724
```

**这个产品最终交付什么？**

| 层 | 交付物 | 类比 |
|---|---|---|
| L1 描述层 | CRD/OpenAPI Schema | "宪法 / API 合同" |
| L2 控制层 | 控制器 + Reconcile | "立法机关 / 政府办公厅" |
| L3 数据层 | Gateway + PDP + Agent Runtime + Vector/Audit Store | "公检法 / 执法部门" |
| L4 体验层 | CLI / Console / SDK | "办事大厅" |

---

## 2. 产品全貌（一张图看完整体）

```mermaid
flowchart TB
    subgraph User["👤 用户视角"]
        U1[Platform Admin<br/>写 YAML]
        U2[业务开发者<br/>调 SDK/API]
        U3[终端用户<br/>飞书/网页对话]
    end

    subgraph CtlPlane["🎛 控制平面 (Control Plane)"]
        direction TB
        Console[Console UI]
        CLI[aikctl CLI]
        APIServer[API Server<br/>K8s apiextensions]

        subgraph CRDStore["CRD Store (etcd / DB)"]
            S1[Skill]
            S2[Agent]
            S3[Policy]
            S4[Tool / Model / KB...]
        end

        subgraph Controllers["Controllers (Reconcile)"]
            CSkill[Skill Ctrl]
            CAgent[Agent Ctrl]
            CPolicy[Policy Ctrl]
            COther[Tool/KB/Model Ctrl]
        end

        Compiler[Policy Compiler<br/>→ Rego/Cedar]
        Registry[Skill/Tool<br/>Registry]
    end

    subgraph DataPlane["⚡ 数据平面 (Data Plane)"]
        direction TB
        GW[AI Gateway<br/>统一入口]
        PEP[策略执行点 PEP]
        PDP[策略决策点 PDP<br/>OPA/Cedar]
        Router[Model Router<br/>+ Cache]
        Sandbox[Sandbox Runtime<br/>firecracker/gvisor]
        AgentRT[Agent Runtime<br/>react/plan/workflow]
        Guardrails[Guardrails<br/>injection/PII/halluc]
        DLP[DLP<br/>脱敏]
    end

    subgraph External["🔌 外部依赖"]
        IDP[SSO/IdP<br/>Okta/Keycloak]
        Vault[Secrets<br/>Vault/KMS]
        Models[LLM Endpoints<br/>OpenAI/通义/私有]
        Tools[MCP Servers<br/>飞书/Jira/DB]
        Vec[(Vector/Graph Store)]
        Audit[(Audit/SIEM<br/>ClickHouse + S3 WORM)]
    end

    U1 --> Console
    U1 --> CLI
    U2 --> GW
    U3 --> GW
    Console --> APIServer
    CLI --> APIServer
    APIServer --> CRDStore
    CRDStore -. watch .-> Controllers
    Controllers -- 写 status --> CRDStore
    CPolicy --> Compiler
    Compiler -- bundle --> PDP
    CSkill --> Registry
    CAgent -- 部署/配置 --> AgentRT

    GW --> PEP
    PEP <--> PDP
    PDP -. 加载策略 .-> Compiler
    PEP --> DLP
    DLP --> AgentRT
    AgentRT --> Guardrails
    AgentRT --> Router
    AgentRT --> Sandbox
    Router --> Models
    AgentRT --> Tools
    AgentRT --> Vec
    AgentRT -- 事件流 --> Audit
    GW -- 身份 --> IDP
    GW -- 取密钥 --> Vault

    style CRDStore fill:#fff3cd,stroke:#856404
    style Controllers fill:#d1ecf1,stroke:#0c5460
    style DataPlane fill:#d4edda,stroke:#155724
```

**关键认知**：

- **CRD 在最上面**——用户能看到的就是它，所有功能都从这里"声明"
- **Controllers 在中间**——把 YAML 翻译成"实际运行的系统"
- **Gateway/PDP/Runtime 在最下面**——真正处理每一次 AI 调用，并且它们的"行为"完全由上面的 CRD 决定

---

## 3. CRD 字段 → 系统能力的映射（最关键的一张图）

很多人看 schema 觉得"字段好多没必要"，但每一组字段都对应一个**真实的运行时组件**。下面这张图把 schema 的每一段映射到具体要实现的服务。

```mermaid
flowchart LR
    subgraph SkillSpec["📜 Skill spec"]
        Sk1[interface<br/>input/output schema]
        Sk2[implementation<br/>runtime/image/prompt]
        Sk3[requires<br/>models/tools/data]
        Sk4[governance<br/>classification/PII/合规]
        Sk5[cost & slo & budget]
        Sk6[evaluation & gates]
        Sk7[reliability<br/>retry/fallback/CB]
    end

    subgraph SkComp["⚙️ 要实现的组件"]
        SC1[Schema Validator<br/>JSON Schema 校验器]
        SC2[Skill Runtime<br/>容器/Function 执行器]
        SC3[Dependency Resolver<br/>引用解析与版本求解]
        SC4[DLP / Classification<br/>数据分级与脱敏]
        SC5[Cost Tracker<br/>+ Budget Enforcer]
        SC6[Eval Runner<br/>+ Stage Gate]
        SC7[Resilience Layer<br/>重试/熔断/Fallback]
    end

    Sk1 --> SC1
    Sk2 --> SC2
    Sk3 --> SC3
    Sk4 --> SC4
    Sk5 --> SC5
    Sk6 --> SC6
    Sk7 --> SC7
```

```mermaid
flowchart LR
    subgraph AgentSpec["📜 Agent spec"]
        Ag1[identity<br/>SA / OBO]
        Ag2[skills 组合<br/>+ versionConstraint]
        Ag3[memory<br/>shortTerm/longTerm]
        Ag4[runtime pattern<br/>react/plan/workflow]
        Ag5[sandbox]
        Ag6[guardrails<br/>input/output/behavior]
        Ag7[budget.onExceed]
        Ag8[channels<br/>飞书/web/api]
        Ag9[deployment<br/>canary/replicas]
        Ag10[audit level]
    end

    subgraph AgComp["⚙️ 要实现的组件"]
        AC1[Identity Broker<br/>OIDC/OBO 交换]
        AC2[Skill Composer<br/>编排执行链]
        AC3[Memory Service<br/>会话+长期向量]
        AC4[Agent Runtime<br/>状态机 + Tool Loop]
        AC5[Sandbox Runtime<br/>microVM]
        AC6[Guardrail Engine<br/>多 provider 链]
        AC7[Budget Enforcer<br/>session 级]
        AC8[Channel Adapter<br/>飞书/wecom/web SDK]
        AC9[Rollout Controller<br/>金丝雀+分析]
        AC10[Audit Sink]
    end

    Ag1 --> AC1
    Ag2 --> AC2
    Ag3 --> AC3
    Ag4 --> AC4
    Ag5 --> AC5
    Ag6 --> AC6
    Ag7 --> AC7
    Ag8 --> AC8
    Ag9 --> AC9
    Ag10 --> AC10
```

```mermaid
flowchart LR
    subgraph PolSpec["📜 Policy spec"]
        P1[effect+priority]
        P2[subject<br/>User/Agent/SA]
        P3[action<br/>verbs+resources]
        P4[conditions<br/>time/loc/risk/CEL]
        P5[constraints<br/>budget/rate/quota]
        P6[approvals<br/>人审]
        P7[obligations<br/>audit/redact/notify]
    end

    subgraph PolComp["⚙️ 要实现的组件"]
        PC1[Decision Engine<br/>OPA/Cedar]
        PC2[Subject Resolver<br/>身份属性查询]
        PC3[Resource Selector<br/>命中匹配]
        PC4[CEL Evaluator<br/>+ Risk Engine]
        PC5[Rate Limiter<br/>+ Quota Service]
        PC6[Approval Workflow<br/>飞书审批集成]
        PC7[Obligation Executor<br/>审计/脱敏/水印]
    end

    P1 --> PC1
    P2 --> PC2
    P3 --> PC3
    P4 --> PC4
    P5 --> PC5
    P6 --> PC6
    P7 --> PC7
```

> **看出门道了吗？**
> 每一个 schema 字段背后,都对应一个"必须开发的服务"。schema 写得越完整,产品边界就越清晰。
> 没有这套 schema,你可能要写 50 份散落的 PRD;有了它,产品 backlog 一目了然。

---

## 4. 一次真实调用的端到端流程

把所有抽象拉到一个具体场景：**法务小张在飞书里问"帮我审一下这份合同"**。

```mermaid
sequenceDiagram
    autonumber
    participant U as 👤 法务小张
    participant FS as 飞书
    participant GW as AI Gateway
    participant PEP as PEP
    participant PDP as PDP (OPA)
    participant DLP as DLP
    participant ART as Agent Runtime
    participant GR as Guardrails
    participant SR as Skill Runtime<br/>contract-review
    participant TR as Tool: docusign
    participant KB as KB: legal-kb
    participant MR as Model Router
    participant LLM as gpt-4o-eu
    participant AU as Audit Sink

    U->>FS: "审一下这份合同 doc-xxx"
    FS->>GW: webhook + 用户 token
    GW->>GW: 1. 验证用户身份 (OIDC)
    GW->>PEP: 2. 构造调用上下文<br/>(user, agent=legal-copilot, action=invoke)

    PEP->>PDP: 3. 决策请求 (Policy[legal-copilot-access])
    Note over PDP: 检查:<br/>- subject.dept ∈ Legal ✓<br/>- mfa ✓<br/>- timeWindow ✓<br/>- budget < limit ✓
    PDP-->>PEP: allow + obligations:<br/>{audit:high, redact:pii-cn, watermark}

    PEP->>DLP: 4. 入站脱敏(身份证/手机号)
    DLP-->>PEP: redacted text

    PEP->>ART: 5. invoke agent (with context)
    ART->>GR: 6. 输入护栏:Injection/PII
    GR-->>ART: pass

    ART->>SR: 7. invoke skill[contract-review@1.2.0]
    SR->>TR: 8a. 取合同 (OBO 用小张身份)
    TR-->>SR: 合同文本
    SR->>KB: 8b. 检索类似条款 (ACL=小张可见)
    KB-->>SR: 引用片段

    SR->>MR: 9. LLM 推理请求
    MR->>LLM: 路由到 EU 区域 (合规)
    LLM-->>MR: 推理结果
    MR-->>SR: structured output

    SR-->>ART: {risks:[...], confidence:0.91}
    ART->>GR: 10. 输出护栏:Hallucination/PIILeak
    GR-->>ART: pass

    ART-->>PEP: response
    PEP->>PEP: 11. 执行 obligations<br/>(打水印、写审计)
    PEP-->>GW: response + watermark
    GW-->>FS: 富文本回复
    FS-->>U: 风险点 + 引用 + 置信度

    par 异步审计
        PEP->>AU: AuditEvent(invocationId, ...)
        ART->>AU: tool_calls / model_calls / tokens / cost
        SR->>AU: skill metrics
    end
```

**这张图里每一个箭头都对应 schema 里的一段配置**：

| 步骤 | 对应 schema 字段 |
|---|---|
| ① 验身份 | `Agent.identity.serviceAccount` + IdP 配置 |
| ② 上下文 | `Agent.identity.representation.mode = on_behalf_of` |
| ③ 决策 | `Policy.{subject, action, conditions, constraints}` |
| ③ obligations | `Policy.obligations.{audit, redact, watermark}` |
| ④ DLP | `Skill.governance.pii.onInput` |
| ⑥⑩ 护栏 | `Agent.guardrails.{input, output}` |
| ⑦ skill 解析 | `Agent.skills[].versionConstraint` + `Skill.implementation` |
| ⑧ tool/KB | `Skill.implementation.requires.{tools, dataSources}` |
| ⑨ 路由 | `ModelRouter.rules` + `ModelEndpoint.region` |
| ⑪ 审计 | `Agent.audit.level` + `Policy.obligations.audit` + `AuditEvent` CRD |

> 你现在能直观看到：**schema 不是文档，schema 就是这条链路的"接线图"**。
> 控制器把 YAML 编译进来，调用时数据面照着执行。

---

## 5. 控制器把 YAML 变成什么？(声明 → 实物的转化)

```mermaid
flowchart LR
    subgraph YAML["📜 用户声明"]
        Y1[Skill YAML]
        Y2[Agent YAML]
        Y3[Policy YAML]
    end

    subgraph K8s["☸️ Kubernetes 实物"]
        D1["Deployment + HPA + Service"]
        D2["ConfigMap<br/>systemPrompt 等"]
        D3["Secret<br/>API Keys"]
        D4["ServiceAccount + Token"]
        D5["Ingress / Route<br/>飞书 webhook"]
        D6["NetworkPolicy<br/>sandbox 网络"]
    end

    subgraph Stores["📦 平台 Store"]
        T1["Skill Registry<br/>实现可发现"]
        T2["Tool Registry"]
        T3["Prompt Registry"]
        T4["Eval Result Store"]
    end

    subgraph PDPStore["🛡️ PDP 状态"]
        P1["Compiled Policy Bundle"]
        P2["Subject Cache"]
        P3["Resource Index"]
    end

    subgraph Routing["🚏 路由配置"]
        R1["Gateway Route<br/>channel → agent"]
        R2["Model Router Rule"]
        R3["Rate Limit Token Bucket"]
    end

    Y1 --> T1
    Y1 --> T3
    Y1 --> T4
    Y2 --> D1
    Y2 --> D2
    Y2 --> D3
    Y2 --> D4
    Y2 --> D5
    Y2 --> D6
    Y2 --> R1
    Y2 --> R3
    Y3 --> P1
    Y3 --> P2
    Y3 --> P3
```

> 这就是"声明式平台"的精髓——**用户只描述意图，控制器制造一切**。
>
> 你做的工作量是"实现 X 个控制器"，而不是"为每个客户写 X 个集成"。

---

## 6. 多租户视角：一份 schema，百家公司用

通用产品最大的难点是**让同一份内核服务多个行业 / 客户 / 合规域**。schema 在这里起到关键作用：

```mermaid
flowchart TB
    subgraph Core["🧱 平台内核(单一代码库)"]
        Schema[CRD Schema v1alpha1]
        Ctrls[Controllers]
        Engine[Gateway/PDP/Runtime]
    end

    subgraph Pack1["📦 金融行业包"]
        F1[Skill: 合规审查]
        F2[Skill: 投研助手]
        F3[Tool: 核心银行 MCP]
        F4[Policy: 等保三级模板]
        F5[Eval: 金融问答集]
    end

    subgraph Pack2["📦 医疗行业包"]
        M1[Skill: 病历摘要]
        M2[Tool: HIS MCP]
        M3[Policy: HIPAA 模板]
        M4[Eval: 医疗术语集]
    end

    subgraph Pack3["📦 政企行业包"]
        G1[Skill: 公文起草]
        G2[Tool: 12345 MCP]
        G3[Policy: 信创 + 等保]
        G4[ModelEndpoint: 国产模型白名单]
    end

    subgraph Tenants["👥 客户租户(多个)"]
        T1[Tenant A 银行]
        T2[Tenant B 医院]
        T3[Tenant C 政府]
    end

    Schema --> Pack1
    Schema --> Pack2
    Schema --> Pack3
    Pack1 --> T1
    Pack2 --> T2
    Pack3 --> T3
    Core -. 内核共享 .-> T1
    Core -. 内核共享 .-> T2
    Core -. 内核共享 .-> T3
```

**关键事实**：

- 内核 = 一份代码、一份 CRD、一份控制器
- 行业包 = 一组**预填好的 CR（YAML）**，套同一个 schema
- 客户租户 = 装一个或多个行业包 + 自定义补充
- 这意味着：**schema 不变，产品就稳定；行业适配靠加 YAML 包，不改代码**

这就是为什么 CRD/Schema 设计要花最多时间——**一旦发布就是产品的脊柱，不能轻易变**。

---

## 7. 围绕 schema 要建的工程能力（开发清单）

把 schema 落地需要建的能力,按优先级排：

```mermaid
flowchart TB
    subgraph P0["P0: MVP 必须有"]
        A1[CRD Schema 校验器<br/>OpenAPI + CEL]
        A2[Skill Controller]
        A3[Agent Controller]
        A4[Policy Controller]
        A5[Gateway + PEP]
        A6[PDP OPA/Cedar]
        A7[Audit Sink<br/>ClickHouse + S3]
        A8[Identity Broker<br/>OIDC/OBO]
    end

    subgraph P1["P1: 6 个月内"]
        B1[Model Router + Cache]
        B2[Guardrail Engine]
        B3[Sandbox Runtime]
        B4[Eval Runner]
        B5[KB / RAG Pipeline]
        B6[Cost / Budget Service]
        B7[Channel Adapters<br/>飞书/wecom]
    end

    subgraph P2["P2: 12 个月内"]
        C1[Skill Marketplace]
        C2[Industry Pack Manager<br/>类 Helm]
        C3[Compliance Report<br/>等保/SOC2 自动出]
        C4[Red Team Platform]
        C5[Multi-cluster<br/>Federation]
        C6[Console UI]
    end

    P0 --> P1 --> P2
```

> **顺序很重要**:
> 先把 P0 跑通(声明 → 控制器 → Gateway → PDP → 审计),哪怕 Skill 只支持最简单的一种实现也没关系。
> P1 是把"可用"扩到"够用"。
> P2 才是"产品化、商品化、生态化"。

---

## 8. 三句话总结

如果只能记三件事:

1. **CRD/Schema 不是文档,是产品的合同**——它定义了"用户能声明什么,平台必须实现什么",每一个字段都对应一个要写的服务。
2. **控制器是合同的执行机关**——把 YAML 翻译成 Deployment、Policy Bundle、路由规则这些"实物",并持续把实际状态向期望状态收敛。
3. **Gateway + PDP + Runtime 是最终的执法者**——每一次真实的 AI 调用都按 CRD 里写好的规矩执行:谁能调、走哪个模型、怎么审计、怎么计费,全部由 schema 决定。

```
用户写 YAML
    ↓
控制器 reconcile
    ↓
真实可用的 AI 平台
    ↓
合规、可审、可控、可降本
```

---

## 9. 推荐阅读顺序

如果你或团队成员要快速理解这套设计:

1. **本文**(`aip-overview.md`) ← 你在这里 🟢 先看图,建立心智
2. [`aip-crd-openapi.md`](./aip-crd-openapi.md) — 看具体字段,理解每个字段在第 3 节图里对应什么组件
3. [`aip-controllers-reconcile.md`](./aip-controllers-reconcile.md) — 看控制器怎么把 YAML 变成实物
4. (待写) `aik-gateway-pep-pdp.md` — 看数据面具体的执行机制
5. (待写) `aip-industry-packs.md` — 看行业包怎么组织和分发

---

## 文档版本

| 版本 | 日期 | 说明 |
|---|---|---|
| v1.0 | 2026-05-26 | 用 9 张图把 CRD/Schema 在产品中的位置、作用、要实现的组件、端到端调用链、多租户与行业包机制讲清楚 |
