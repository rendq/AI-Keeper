# Healthcare Industry Pack — 医疗行业 AI 平台 Pack

面向医疗行业的 AIP 行业包，提供病历摘要、医学问答、药物交互检查等核心 AI 技能，配套 HIPAA 合规策略及 on-premise vLLM/TGI 私有化部署配置。

## 概述

本 Pack 为医疗机构提供开箱即用的 AI 平台能力，涵盖：

- **3 项核心技能**：病历摘要、医学问答、药物交互检查
- **3 项合规策略**：HIPAA 合规强制策略、患者 ID ACL、PHI 脱敏
- **2 个模型端点**：vLLM On-Premise + TGI On-Premise
- **1 个模型路由**：基于临床场景的智能路由
- **1 个知识库**：医学指南与临床知识库
- **1 个数据源**：医疗文档数据源连接器
- **2 套评估集**：医疗术语 + 诊断准确度 benchmark

## 架构

```
用户 → 网关 → PEP/PDP → Skill Runtime
                              ↓
              ┌───────────────┼───────────────┐
              ↓               ↓               ↓
       医学指南 KB       药物交互引擎      模型路由
       (pre_filter)   (Drug Interaction)  (healthcare-router)
              ↓               ↓               ↓
              └───────┬───────┘       ┌───────┴───────┐
                      ↓               ↓               ↓
           审计日志 + PHI 脱敏    vLLM Medical     TGI Medical
           (HIPAA 6年留存)      (us-east-1)      (us-west-2)
```

## 资源清单 (13 CRs)

| # | Kind | Name | 说明 |
|---|------|------|------|
| 1 | Tenant | healthcare-dept | 医疗事业部租户（HIPAA 合规） |
| 2 | ServiceAccount | healthcare-sa | 服务账户（15min Token，OBO） |
| 3 | Skill | medical-record-summary | 病历摘要技能 |
| 4 | Skill | medical-qa | 医学问答技能 |
| 5 | Skill | drug-interaction-check | 药物交互检查技能 |
| 6 | Policy | hipaa-enforcement | HIPAA 合规强制策略 |
| 7 | Policy | patient-id-acl | 患者 ID 访问控制策略 |
| 8 | Policy | phi-redaction | PHI 脱敏策略 |
| 9 | ModelEndpoint | vllm-medical | vLLM On-Premise 模型端点 |
| 10 | ModelEndpoint | tgi-medical | TGI On-Premise 模型端点 |
| 11 | ModelRouter | healthcare-router | 医疗业务模型路由 |
| 12 | KnowledgeBase | medical-guidelines-kb | 医学指南知识库 |
| 13 | DataSource | medical-docs-source | 医疗文档数据源 |

## 部署

### 前提条件

- AIP 平台 >= 1.0.0
- `core/base` Pack 已安装
- `core/policy-engine` Pack 已安装
- On-premise GPU 集群已部署（vLLM 需 4×A100，TGI 需 2×A100）
- 网络隔离环境，PHI 数据不出数据中心

### 安装

```bash
# 校验 Pack 格式
aikctl pack lint industry/healthcare

# 试运行（不实际部署）
aikctl pack install industry/healthcare --tenant test --dry-run

# 正式安装到指定租户
aikctl pack install industry/healthcare --tenant healthcare-dept

# 使用自定义 values
aikctl pack install industry/healthcare --tenant healthcare-dept --values my-values.yaml
```

### 验证

```bash
# 检查所有资源状态
aikctl get tenant healthcare-dept
aikctl get skill -n healthcare-dept
aikctl get policy -n healthcare-dept
aikctl get modelendpoint -n healthcare-dept
aikctl get knowledgebase -n healthcare-dept
```

## 配置说明

通过 `values.yaml` 自定义部署配置：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `tenant.name` | healthcare-dept | 租户名称 |
| `tenant.adminEmail` | healthcare-admin@example.com | 管理员邮箱 |
| `model.vllmEndpoint` | vllm-medical.internal:8000 | vLLM 端点 |
| `model.tgiEndpoint` | tgi-medical.internal:8080 | TGI 端点 |
| `compliance.hipaaEnabled` | true | 是否启用 HIPAA 策略 |
| `compliance.phiRedactionEnabled` | true | 是否启用 PHI 脱敏 |
| `compliance.patientIdAclEnabled` | true | 是否启用患者 ID ACL |

## On-Premise 部署说明

### vLLM 配置

本 Pack 使用 vLLM 作为主要推理引擎，适用于大参数医疗模型：

- **模型**: medical-llama-70b（70B 参数医疗微调模型）
- **GPU**: 4×NVIDIA A100 80GB（Tensor Parallel）
- **量化**: AWQ 4-bit
- **最大上下文**: 32,768 tokens
- **适用场景**: 药物交互检查（高准确度）、长文本病历摘要

### TGI 配置

TGI 作为辅助推理引擎，适用于中等规模 MoE 模型：

- **模型**: medical-mixtral-8x7b（MoE 医疗微调模型）
- **GPU**: 2×NVIDIA A100 80GB
- **量化**: GPTQ 4-bit
- **最大上下文**: 32,768 tokens
- **适用场景**: 医学问答（高吞吐量）、常规查询

## 评估集

| 评估集 | 说明 | 核心指标 |
|--------|------|----------|
| medical-terminology-benchmark | 医疗术语理解（定义、编码、指南） | accuracy >= 0.85 |
| diagnosis-accuracy-benchmark | 诊断准确度（药物交互、ICD-10、红旗识别） | F1 >= 0.91, safety >= 0.95 |

评估定时运行（每周一凌晨 2 点），结果保留 30 天。

## HIPAA 合规说明

本 Pack 实施以下 HIPAA 要求：

| HIPAA 条款 | 实施措施 |
|------------|----------|
| §164.312(a) Access Control | patient-id-acl 策略 + Break-the-Glass 机制 |
| §164.312(b) Audit Controls | 所有 PHI 访问产生审计日志，保留 6 年 |
| §164.312(c) Integrity | 数据完整性校验 + 水印 |
| §164.312(e) Transmission Security | 强制 TLS 1.3 + AES-256 加密存储 |
| §164.502(b) Minimum Necessary | 最小必要原则访问控制 |
| §164.514(b) Safe Harbor | PHI 18 类标识符自动脱敏 |

## Requirements Coverage

- **C5.5** (Industry Pack): 完整医疗行业 Pack，含技能、策略、模型、评估
- **D6** (Data & Knowledge): 医学知识库 + PHI 安全管控
