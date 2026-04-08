"""
Unified CUA (Computer Use Agent) template.

Supports Anthropic, OpenAI, and Gemini providers with automatic fallback.
Configure via environment variables:
  CUA_PROVIDER            - Primary provider: anthropic, openai, or gemini (default: anthropic)
  CUA_FALLBACK_PROVIDERS  - Comma-separated fallback order (e.g. "openai,gemini")
  ANTHROPIC_API_KEY       - Required if using Anthropic
  OPENAI_API_KEY          - Required if using OpenAI
  GOOGLE_API_KEY          - Required if using Gemini
"""

import os
from typing import NotRequired, Optional, TypedDict

import kernel
from session import KernelBrowserSession
from tools import KernelExecutor
from providers import run_with_fallback, ProviderConfig

# Parse provider configuration
PRIMARY_PROVIDER = os.getenv("CUA_PROVIDER", "anthropic")
FALLBACK_PROVIDERS = [
    p.strip() for p in os.getenv("CUA_FALLBACK_PROVIDERS", "").split(",") if p.strip()
]
PROVIDER_CHAIN = [PRIMARY_PROVIDER] + FALLBACK_PROVIDERS

API_KEY_MAP = {
    "anthropic": "ANTHROPIC_API_KEY",
    "openai": "OPENAI_API_KEY",
    "gemini": "GOOGLE_API_KEY",
}

configured = [p for p in PROVIDER_CHAIN if os.getenv(API_KEY_MAP.get(p, ""))]
if not configured:
    keys = [API_KEY_MAP.get(p, p) for p in PROVIDER_CHAIN]
    raise ValueError(
        f"No API keys found for configured providers {PROVIDER_CHAIN}. "
        f"Set at least one of: {', '.join(keys)}"
    )


class CuaInput(TypedDict):
    query: str
    provider: NotRequired[str]
    model: NotRequired[str]
    record_replay: NotRequired[bool]


class CuaOutput(TypedDict):
    result: str
    provider: str
    replay_url: Optional[str]


app = kernel.App("python-cua")


@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: CuaInput,
) -> CuaOutput:
    if not payload or not payload.get("query"):
        raise ValueError("Query is required")

    # Allow per-request provider override
    if payload.get("provider"):
        provider_chain = [payload["provider"]] + [p for p in PROVIDER_CHAIN if p != payload["provider"]]
    else:
        provider_chain = PROVIDER_CHAIN

    record_replay = payload.get("record_replay", False)

    async with KernelBrowserSession(
        invocation_id=ctx.invocation_id,
        stealth=True,
        record_replay=record_replay,
    ) as session:
        print("Kernel browser live view url:", session.live_view_url)

        executor = KernelExecutor(session.kernel, session.session_id)

        result = await run_with_fallback(
            provider_chain,
            ProviderConfig(
                query=payload["query"],
                model=payload.get("model"),
                viewport_width=session.viewport_width,
                viewport_height=session.viewport_height,
            ),
            executor,
        )

    return {
        "result": result.result,
        "provider": result.provider,
        "replay_url": session.replay_view_url,
    }
