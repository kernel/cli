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
from typing import Literal, TypedDict

import kernel
from kernel import Kernel

from providers import resolve_providers, run_with_fallback, TaskOptions
from session import KernelBrowserSession, SessionOptions

kernel_client = Kernel()
app = kernel.App("python-cua")


class BrowserProfile(TypedDict, total=False):
    id: str
    name: str
    save_changes: bool


class BrowserExtension(TypedDict, total=False):
    id: str
    name: str


class BrowserConfig(TypedDict, total=False):
    proxy_id: str
    profile: BrowserProfile
    extensions: list[BrowserExtension]
    timeout_seconds: int


class CuaInput(TypedDict, total=False):
    query: str
    provider: Literal["anthropic", "openai", "gemini"]
    model: str
    record_replay: bool
    session_id: str
    browser: BrowserConfig


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
async def cua_task(ctx: kernel.KernelContext, payload: CuaInput | None = None) -> CuaOutput:
    if not payload or not payload.get("query"):
        raise ValueError('Query is required. Payload must include: {"query": "your task description"}')

    providers = _get_providers()

    # Per-request provider override: move requested provider to front
    if payload.get("provider"):
        requested = next((p for p in providers if p.name == payload["provider"]), None)
        if requested:
            providers = [requested] + [p for p in providers if p is not requested]

    # Use an existing browser session (BYOB) or create a new one.
    # BYOB is useful for multi-turn CUA on a persistent browser, or HITL
    # where a human uses the live view between CUA calls.
    if payload.get("session_id"):
        browser = await asyncio.to_thread(
            kernel_client.browsers.retrieve, payload["session_id"],
        )
        vp = getattr(browser, "viewport", None)
        task_result = await run_with_fallback(
            providers,
            TaskOptions(
                query=payload["query"],
                kernel=kernel_client,
                session_id=payload["session_id"],
                model=payload.get("model"),
                viewport_width=getattr(vp, "width", 1280),
                viewport_height=getattr(vp, "height", 800),
            ),
        )
        return {"result": task_result.result, "provider": task_result.provider}

    browser_cfg = payload.get("browser") or {}
    session = KernelBrowserSession(
        kernel_client,
        SessionOptions(
            invocation_id=ctx.invocation_id,
            stealth=True,
            record_replay=payload.get("record_replay", False),
            proxy_id=browser_cfg.get("proxy_id"),
            profile=browser_cfg.get("profile"),
            extensions=browser_cfg.get("extensions"),
            timeout_seconds=browser_cfg.get("timeout_seconds", 300),
        ),
    )

    await session.start()
    print(f"Live view: {session.live_view_url}")

    try:
        task_result = await run_with_fallback(
            providers,
            TaskOptions(
                query=payload["query"],
                kernel=kernel_client,
                session_id=session.session_id,
                model=payload.get("model"),
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
