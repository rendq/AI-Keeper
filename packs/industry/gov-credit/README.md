# 政务信用行业 AI 平台 Pack（信创适配）

## 概述

`industry/gov-credit` 是面向政务信用行业的 AIP 行业包，提供公文起草、政策问答、信用评估等核心 AI 技能，全面适配国产信创技术栈。

## 核心能力

### 技能（Skills）

| 技能 | 说明 | 分类等级 |
|------|------|----------|
| `official-document-drafting` | 公文起草（通知/决定/报告/函/纪要/批复） | secret |
| `policy-qa` | 政策问答（信用/税务/社保/住房/工商） | internal |
| `credit-assessment` | 信用评估（企业/个人多维度评分） | secret |

### 合规策略（Policies）

| 策略 | 说明 | 优先级 |
|------|------|--------|
| `djsanji-crypto-enforcement` | 等保三级 + 密评强制策略 | 5 |
| `xinchuang-model-whitelist` | 信创模型白名单（仅允许认证模型） | 8 |
| `guomi-encryption` | 国密加密策略（SM2/SM3/SM4 全链路） | 3 |

### 模型（Models）

| 模型端点 | 提供商 | 部署方式 |
|----------|--------|----------|
| `baichuan-xinchuang` | 百川智能 | 私有化部署（信创环境） |
| `chatglm-xinchuang` | 智谱 AI | 私有化部署（信创环境） |

## 信创技术栈适配

### 支持的硬件平台

| 组件 | 鲲鹏方案 | 海光方案 |
|------|----------|----------|
| 操作系统 | 银河麒麟 V10 | 银河麒麟 V10 / UOS V20 |
| CPU | 鲲鹏 920 (ARM64) | 海光 C86-7xxx (x86_64) |
| AI 加速卡 | 昇腾 910B | 海光 DCU Z100 |
| 数据库 | 达梦 V8 / OceanBase V4 | OceanBase V4 / 达梦 V8 |

### 国密算法要求

本 Pack 强制全链路使用国密算法：

- **SM2**：非对称加密、数字签名、密钥交换
- **SM3**：哈希完整性校验、审计日志签名
- **SM4**：对称加密（传输加密 SM4-GCM、存储加密 SM4-GCM）
- **国密 TLS**：使用 ECC_SM4_SM3 密码套件

禁止使用 RSA、AES、SHA 等国际密码算法。

## 安装

### 默认安装

```bash
aikctl pack install industry/gov-credit --tenant gov-credit-dept
```

### 鲲鹏环境安装（麒麟 OS + 鲲鹏 ARM64 + 达梦）

```bash
aikctl pack install industry/gov-credit \
  --tenant gov-credit-dept \
  --values packs/industry/gov-credit/values-kylin.yaml
```

### 海光环境安装（麒麟 OS + 海光 x86_64 + OceanBase）

```bash
aikctl pack install industry/gov-credit \
  --tenant gov-credit-dept \
  --values packs/industry/gov-credit/values-haiguang.yaml
```

### Helm 模板渲染验证

```bash
helm template --values packs/industry/gov-credit/values-kylin.yaml
helm template --values packs/industry/gov-credit/values-haiguang.yaml
```

## 前置条件

1. AIP 平台已部署且版本 >= 1.0.0
2. `core/base` 和 `core/policy-engine` Pack 已安装
3. `infra/xinchuang-stack` Pack 已安装（提供信创基础设施支持）
4. 国密证书已签发（SM2 证书 + 国密 CA）
5. 硬件密码机（HSM）已就绪
6. 信创数据库（达梦或 OceanBase）已部署并配置 SM4 加密

## 自定义配置

可通过自定义 values 文件覆盖默认值：

```yaml
# my-values.yaml
tenant:
  name: my-gov-dept
  adminEmail: admin@my-gov.cn

model:
  baichuanEndpoint: https://my-baichuan.internal:8443/v1
  chatglmEndpoint: https://my-chatglm.internal:8443/v1

database:
  type: oceanbase
  host: my-ob-cluster.internal
  port: 2881
```

## 验证

```bash
# Lint 检查
aikctl pack lint industry/gov-credit

# 干运行安装验证
aikctl pack install industry/gov-credit --tenant test --dry-run

# Helm 模板验证
helm template --values packs/industry/gov-credit/values-kylin.yaml
```

## 合规说明

- 本 Pack 满足《信息安全等级保护基本要求》第三级（等保三级）
- 通过《商用密码应用安全性评估》（密评）要求
- 符合《信息技术应用创新》（信创）硬件与软件要求
- 审计日志保留 365 天，使用 SM3 签名保证完整性
