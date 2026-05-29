"""gRPC server that exposes all registered skills via the SkillRuntime service."""

from __future__ import annotations

import asyncio
import json
import logging
from concurrent import futures
from typing import Any

import grpc
from grpc import aio as grpc_aio

from dataplane.skillruntime.decorator import SkillDefinition, get_registry
from dataplane.skillruntime.schema import SchemaValidationError

logger = logging.getLogger(__name__)

# ─── gRPC proto-like messages (inline, no codegen dependency for the SDK) ────
# The Skill Runtime gRPC service uses a simple request/response envelope:
#   service SkillRuntimeService {
#     rpc RunSkill(RunSkillRequest) returns (RunSkillResponse);
#   }
#
# We implement it using grpc reflection-free approach with a Generic handler.
# This avoids requiring proto codegen for the SDK consumer.

_SERVICE_NAME = "aip.v1.SkillRuntimeService"
_METHOD_RUN_SKILL = f"/{_SERVICE_NAME}/RunSkill"


class SkillServer:
    """A gRPC server hosting all skills registered via @skill decorator.

    Example:
        server = SkillServer(port=50051)
        await server.start()
        await server.wait_for_termination()
    """

    def __init__(self, port: int = 50051, host: str = "0.0.0.0") -> None:
        self.port = port
        self.host = host
        self._server: grpc_aio.Server | None = None

    async def start(self) -> None:
        """Start the gRPC server."""
        self._server = grpc_aio.server()
        handler = _SkillHandler()
        self._server.add_generic_rpc_handlers([handler])
        bind_addr = f"{self.host}:{self.port}"
        self._server.add_insecure_port(bind_addr)
        await self._server.start()
        logger.info("SkillServer listening on %s with %d skills", bind_addr, len(get_registry()))

    async def stop(self, grace: float = 5.0) -> None:
        """Gracefully stop the server."""
        if self._server:
            await self._server.stop(grace)

    async def wait_for_termination(self) -> None:
        """Block until the server stops."""
        if self._server:
            await self._server.wait_for_termination()


class _SkillHandler(grpc.GenericRpcHandler):
    """Generic gRPC handler that dispatches RunSkill calls."""

    def service_name(self) -> str | None:
        return _SERVICE_NAME

    def service(
        self, handler_call_details: grpc.HandlerCallDetails
    ) -> grpc.RpcMethodHandler | None:
        if handler_call_details.method == _METHOD_RUN_SKILL:
            return grpc.unary_unary_rpc_method_handler(
                self._run_skill,
                request_deserializer=lambda b: json.loads(b),
                response_serializer=lambda r: json.dumps(r).encode("utf-8"),
            )
        return None

    def _run_skill(
        self, request: dict[str, Any], context: grpc.ServicerContext
    ) -> dict[str, Any]:
        """Synchronous wrapper that runs the async skill invoke in the event loop."""
        skill_name = request.get("skill_name", "")
        input_data = request.get("input", {})

        registry = get_registry()
        if skill_name not in registry:
            context.abort(grpc.StatusCode.NOT_FOUND, f"Skill '{skill_name}' not found")
            return {}

        skill_def = registry[skill_name]

        try:
            loop = asyncio.get_event_loop()
            if loop.is_running():
                # Create a future in the running loop
                import concurrent.futures

                with concurrent.futures.ThreadPoolExecutor() as pool:
                    result = pool.submit(
                        asyncio.run, skill_def.invoke(input_data)
                    ).result()
            else:
                result = asyncio.run(skill_def.invoke(input_data))
        except SchemaValidationError as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
            return {}
        except Exception as exc:
            context.abort(grpc.StatusCode.INTERNAL, f"Skill execution failed: {exc}")
            return {}

        return {"output": result}
