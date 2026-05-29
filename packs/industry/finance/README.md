# Finance Industry Pack — 金融行业 AI 平台 Pack

面向金融行业的 AIP 行业包，提供合规审查、投研助手、反洗钱检测、尽调摘要等核心 AI 技能，配套等保三级合规策略及国产大模型私有化部署配置。

## 概述

本 Pack 为金融机构提供开箱即用的 AI 平台能力，涵盖：

- **4 项核心技能**：合规审查、投研助手、反洗钱检测、尽调摘要
- **3 项合规策略**：等保三级强制策略、投研人审、数据分级
- **2 个模型端点**：通义千问（阿里云）+ 百川私有化部署
- **1 个模型路由**：基于业务场景的智能路由
- **1 个知识库**：金融法规与制度知识库
- **1 个数据源**：金融文档数据源连接器
- **2 套评估集**：金融问答 + 合规判断 benchmark

## 架构

```
用户 → 网关 → PEP/PDP → Skill Runtime
                              ↓
              ┌───────────────┼───────────────┐
              ↓               ↓               ↓
       金融法规 KB      反洗钱引擎      模型路由
       (pre_filter)    (AML Engine)   (finance-router)
              ↓               ↓               ↓
              └───────┬───────┘       ┌───────┴───────┐
                      ↓               ↓               ↓
               审计日志 + 水印    通义千问         百川私有化
               (等保三级审计)   (cn-shanghai)    (cn-beijing)
```

## 资源清单 (14 CRs)

| # | Kind | Name | 说明 |
|---|------|------|------|
| 1 | Tenant | finance-dept | 金融事业部租户（DJCP-L3 合规） |
| 2 | ServiceAccount | finance-sa | 服务账户（30min Token，OBO） |
| 3 | Skill | compliance-review | 合规审查技能 |
| 4 | Skill | investment-research | 投研助手技能 |
| 5 | Skill | aml-detection | 反洗钱检测技能 |
| 6 | Skill | due-diligence-summary | 尽调摘要技能 |
| 7 | Policy | djsanji-enforcement | 等保三级强制策略 |
| 8 | Policy | investment-hitl | 投研人审策略（HITL） |
| 9 | Policy | data-classification | 数据分级策略 |
| 10 | ModelEndpoint | tongyi-qwen | 通义千问模型端点 |
| 11 | ModelEndpoint | baichuan-private | 百川私有化部署端点 |
| 12 | ModelRouter | finance-router | 金融业务模型路由 |
| 13 | KnowledgeBase | finance-regulations-kb | 金融法规知识库 |
| 14 | DataSource | finance-docs-source | 金融文档数据源 |

## 部署

### 前提条件

- AIP 平台 >= 1.0.0
- `core/base` Pack 已安装
- `core/policy-engine` Pack 已安装
- 百川私有化推理服务已部署（可选，如不使用可在 values 中禁用）

### 安装

```bash
# 校验 Pack 格式
aikctl pack lint industry/finance

# 试运行（不实际部署）
aikctl pack install industry/finance --tenant test --dry-run

# 正式安装到指定租户
aikctl pack install industry/finance --tenant finance-dept

# 使用自定义 values
aikctl pack install industry/finance --tenant finance-dept --values my-values.yaml
```

### 验证

```bash
# 检查所有资源状态
aikctl get tenant finance-dept
aikctl get skill -n finance-dept
aikctl get policy -n finance-dept
aikctl get modelendpoint -n finance-dept
aikctl get knowledgebase -n finance-dept
```

## 配置说明

通过 `values.yaml` 自定义部署配置：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `tenant.name` | finance-dept | 租户名称 |
| `tenant.adminEmail` | finance-admin@example.com | 管理员邮箱 |
| `model.tongyiEndpoint` | dashscope.aliyuncs.com | 通义千问端点 |
| `model.baichuanEndpoint` | baichuan-private.internal:8443 | 百川私有化端点 |
| `compliance.djsanjiEnabled` | true | 是否启用等保三级策略 |
| `compliance.investmentHitlEnabled` | true | 是否启用投研人审 |

## 评估集

| 评估集 | 说明 | 核心指标 |
|--------|------|----------|
| finance-qa-benchmark | 金融问答（法规、产品、风控） | accuracy >= 0.85 |
| compliance-judgment-benchmark | 合规判断（违规识别、法规匹配） | F1 >= 0.87 |

评估定时运行（每周一凌晨 3 点），结果保留 30 天。

## Requirements Coverage

- **C5.5** (Industry Pack): 完整金融行业 Pack，含技能、策略、模型、评估
- **D5** (Model Management): 国产大模型端点 + 智能路由
- **D7** (Compliance): 等保三级策略 + 数据分级 + 审计
