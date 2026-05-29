# Legal Copilot — Industry Pack

Reference industry pack demonstrating the full AIP platform capabilities through an end-to-end legal contract review assistant.

## Overview

This pack deploys a **Legal Copilot** agent that assists legal professionals with contract review and legal research. It demonstrates:

- Multi-resource orchestration (10 Custom Resources)
- ACL-aware knowledge base retrieval (pre_filter)
- OBO (On-Behalf-Of) identity propagation to external tools
- Model routing with compliance constraints (EU data residency)
- Audit with watermarking and citation requirements
- Policy-based access control with obligations

## Architecture

```
User (Feishu) → Gateway → PEP/PDP → Agent Runtime (tool_calling)
                                          ↓
                          ┌───────────────┼───────────────┐
                          ↓               ↓               ↓
                   KB pre_filter    DocuSign MCP     GPT-4o (EU)
                   (legal-kb)      (OBO token)     (reasoner-router)
                          └───────────────┼───────────────┘
                                          ↓
                              Answer + Citations + Watermark
                                          ↓
                                    Audit Sink (ClickHouse + S3 WORM)
```

## Resources (10 CRs)

| # | Kind | Name | Purpose |
|---|------|------|---------|
| 1 | Tenant | legal-dept | Isolation unit with GDPR/SOC2 compliance |
| 2 | ServiceAccount | legal-copilot-sa | Agent identity with OBO enabled |
| 3 | Skill | contract-review | Core review logic (Python) |
| 4 | Tool | docusign-mcp | DocuSign document retrieval (MCP) |
| 5 | Tool | legal-search-openapi | Legal corpus search (OpenAPI) |
| 6 | DataSource | legal-kb-source | Document connector |
| 7 | KnowledgeBase | legal-kb | Vector + semantic search with ACL |
| 8 | Agent | legal-copilot | tool_calling pattern, Feishu channel |
| 9 | Policy | legal-acl | ABAC access control with obligations |
| 10 | ModelEndpoint | gpt-4o-eu | EU-region GPT-4o |
| 11 | ModelRouter | reasoner-router | Routes "reasoner" alias to EU endpoint |

## Deployment

```bash
# Apply all manifests in order
aikctl apply -f packs/industry/legal-copilot/manifests/

# Verify all resources are Active
aikctl get tenant legal-dept
aikctl get agent legal-copilot -n legal-dept
aikctl get skill contract-review -n legal-dept
```

## Skill Implementation

The `contract-review` skill (`skill/contract_review.py`) uses the AIP Python Skill SDK:

1. **KB pre_filter** — Queries `legal-kb` with user ACL context, returning only authorized document chunks
2. **DocuSign tool** — Retrieves contract content via MCP with OBO token propagation
3. **GPT-4o** — Generates answer via `reasoner-router` (EU-compliant endpoint)
4. **Output** — Answer with source citations and invisible watermark for audit tracing

## Requirements Coverage

- **C5** (Industry Pack): Complete pack with manifests + skill implementation
- **D1** (Audit): High-level audit with prompt hashes and watermark obligations
- **C8** (Short-lived secrets): ServiceAccount with 1h token lifetime, OBO for tool auth
