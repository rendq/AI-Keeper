"""Contract Review Skill — Legal Copilot Industry Pack.

Implements the contract-review skill using the AIP Python Skill SDK.
Flow: KB pre_filter → docusign tool → GPT-4o (via reasoner router) → output with citations + watermark.
"""

from __future__ import annotations

import hashlib
import time
import uuid
from typing import Any

from dataplane.skillruntime import skill


INPUT_SCHEMA = {
    "type": "object",
    "required": ["contractId", "query"],
    "properties": {
        "contractId": {"type": "string", "description": "The contract document ID to review"},
        "query": {"type": "string", "description": "User question about the contract"},
        "language": {"type": "string", "enum": ["en", "zh"], "default": "en"},
    },
}

OUTPUT_SCHEMA = {
    "type": "object",
    "required": ["answer", "citations"],
    "properties": {
        "answer": {"type": "string", "description": "The review answer with watermark"},
        "citations": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "source": {"type": "string"},
                    "chunk": {"type": "string"},
                    "page": {"type": "integer"},
                },
            },
        },
        "watermark": {"type": "string", "description": "Invisible watermark identifier"},
    },
}


def _generate_watermark(contract_id: str, user_context: dict[str, Any]) -> str:
    """Generate an invisible watermark for audit tracing."""
    payload = f"{contract_id}:{user_context.get('userId', 'anonymous')}:{int(time.time())}"
    return hashlib.sha256(payload.encode()).hexdigest()[:16]


@skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
async def contract_review(input_data: dict[str, Any], context: Any = None) -> dict[str, Any]:
    """Review a contract document by querying the legal KB and DocuSign.

    Pipeline:
    1. Query KnowledgeBase with ACL pre_filter (only user-permitted chunks)
    2. Retrieve contract document via DocuSign MCP tool
    3. Call GPT-4o (via reasoner-router) with context + query
    4. Return answer with citations and watermark
    """
    contract_id = input_data["contractId"]
    query = input_data["query"]
    language = input_data.get("language", "en")

    # Extract context from runtime (injected by Agent Runtime)
    kb_client = context.kb_client if context else None
    tool_client = context.tool_client if context else None
    model_client = context.model_client if context else None
    user_context = context.user_context if context else {}

    # Step 1: KB pre_filter retrieval — ACL ensures user only sees authorized chunks
    kb_results = []
    if kb_client:
        kb_results = await kb_client.query(
            knowledge_base="legal-kb",
            query=query,
            filters={"contractId": contract_id},
            top_k=5,
            acl_context=user_context,  # pre_filter uses this for ACL
        )

    # Step 2: Retrieve contract document from DocuSign
    docusign_content = ""
    if tool_client:
        docusign_response = await tool_client.invoke(
            tool="docusign-mcp",
            input_data={
                "action": "get_document",
                "envelopeId": contract_id,
            },
        )
        docusign_content = docusign_response.get("content", "")

    # Step 3: Call GPT-4o via reasoner-router
    kb_context = "\n\n".join(
        f"[Source: {r.get('source', 'unknown')}, Page {r.get('page', '?')}]\n{r.get('chunk', '')}"
        for r in kb_results
    )

    system_prompt = (
        "You are a legal contract review assistant. "
        "Answer questions about contracts using ONLY the provided context. "
        "Always cite your sources with page numbers. "
        f"Respond in {'English' if language == 'en' else 'Chinese'}."
    )

    user_prompt = (
        f"Contract content:\n{docusign_content[:4000]}\n\n"
        f"Knowledge base context:\n{kb_context}\n\n"
        f"Question: {query}"
    )

    answer = ""
    if model_client:
        response = await model_client.chat(
            model_alias="reasoner",
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            temperature=0.1,
            max_tokens=2000,
        )
        answer = response.get("content", "")
    else:
        answer = f"[Demo mode] Contract review for {contract_id}: {query}"

    # Step 4: Build citations from KB results
    citations = [
        {
            "source": r.get("source", "unknown"),
            "chunk": r.get("chunk", "")[:200],
            "page": r.get("page", 0),
        }
        for r in kb_results
        if r.get("chunk")
    ]

    # Step 5: Generate watermark
    watermark = _generate_watermark(contract_id, user_context)

    return {
        "answer": answer,
        "citations": citations,
        "watermark": watermark,
    }
