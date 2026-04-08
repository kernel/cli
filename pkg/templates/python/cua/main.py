"""
Unified CUA (Computer Use Agent) template with multi-provider support.

Supports Anthropic, OpenAI, and Gemini as interchangeable providers.
Configure via environment variables:
  CUA_PROVIDER           — primary provider ("anthropic", "openai", or "gemini")
  CUA_FALLBACK_PROVIDERS — comma-separated fallback order (optional)

Each provider requires its own API key:
  ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY
"""

from __future__ import annotations

import asyncio
from typing import TypedDict

from kernel import Kernel, KernelContext

from providers import resolve_providers, run_with_fallback, TaskOptions
from session import KernelBrowserSession, SessionOptions

kernel = Kernel()
app = kernel.app("python-cua")


class CuaInput(TypedDict, total=False):
    query: str
    record_replay: bool


class CuaOutput(TypedDict, total=False):
    result: str
    provider: str
    replay_url: str


# Provider resolution is deferred to the action handler because env vars
# are not available during Hypeman's build/discovery phase.
_providers: list | None = None


def _get_providers():
    global _providers
    if _providers is None:
        _providers = resolve_providers()
        print(f"Configured providers: {' -> '.join(p.name for p in _providers)}")
    return _providers


@app.action("cua-task")
async def cua_task(ctx: KernelContext, payload: CuaInput | None = None) -> CuaOutput:
    if not payload or not payload.get("query"):
        raise ValueError('Query is required. Payload must include: {"query": "your task description"}')

    providers = _get_providers()

    session = KernelBrowserSession(
        kernel,
        SessionOptions(
            invocation_id=ctx.invocation_id,
            stealth=True,
            record_replay=payload.get("record_replay", False),
        ),
    )

    await session.start()
    print(f"Live view: {session.live_view_url}")

    try:
        task_result = await run_with_fallback(
            providers,
            TaskOptions(
                query=payload["query"],
                kernel=kernel,
                session_id=session.session_id,
                viewport_width=session.opts.viewport_width,
                viewport_height=session.opts.viewport_height,
            ),
        )

        session_info = await session.stop()

        output: CuaOutput = {
            "result": task_result.result,
            "provider": task_result.provider,
        }
        if session_info.replay_view_url:
            output["replay_url"] = session_info.replay_view_url

        return output

    except Exception:
        await session.stop()
        raise
