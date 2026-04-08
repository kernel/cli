"""
Provider factory with automatic fallback.

Resolution order:
  1. CUA_PROVIDER env var (required)
  2. CUA_FALLBACK_PROVIDERS env var (optional, comma-separated)
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Protocol

from kernel import Kernel


@dataclass
class TaskOptions:
    query: str
    kernel: Kernel
    session_id: str
    model: str | None = None
    viewport_width: int = 1280
    viewport_height: int = 800


@dataclass
class TaskResult:
    result: str
    provider: str


class CuaProvider(Protocol):
    @property
    def name(self) -> str: ...
    def is_configured(self) -> bool: ...
    async def run_task(self, options: TaskOptions) -> TaskResult: ...


def _build_provider(name: str) -> CuaProvider | None:
    if name == "anthropic":
        from .anthropic import AnthropicProvider
        return AnthropicProvider()
    if name == "openai":
        from .openai import OpenAIProvider
        return OpenAIProvider()
    if name == "gemini":
        from .gemini import GeminiProvider
        return GeminiProvider()
    return None


def resolve_providers() -> list[CuaProvider]:
    """Build the ordered list of providers to try."""
    primary = os.environ.get("CUA_PROVIDER", "").strip().lower()
    fallbacks = [
        s.strip().lower()
        for s in os.environ.get("CUA_FALLBACK_PROVIDERS", "").split(",")
        if s.strip()
    ]

    order = ([primary] if primary else []) + fallbacks

    seen: set[str] = set()
    providers: list[CuaProvider] = []

    for name in order:
        if name in seen:
            continue
        seen.add(name)

        provider = _build_provider(name)
        if provider is None:
            print(f'Warning: Unknown provider "{name}", skipping.')
            continue
        if not provider.is_configured():
            print(f'Warning: Provider "{name}" missing API key, skipping.')
            continue
        providers.append(provider)

    if not providers:
        raise RuntimeError(
            "No CUA provider is configured. "
            "Set CUA_PROVIDER to one of: anthropic, openai, gemini, "
            "and provide the matching API key."
        )

    return providers


async def run_with_fallback(
    providers: list[CuaProvider],
    options: TaskOptions,
) -> TaskResult:
    """Run a CUA task, trying each provider in order until one succeeds."""
    errors: list[tuple[str, Exception]] = []

    for provider in providers:
        try:
            print(f"Attempting provider: {provider.name}")
            return await provider.run_task(options)
        except Exception as exc:
            print(f'Provider "{provider.name}" failed: {exc}')
            errors.append((provider.name, exc))

    summary = "\n".join(f"  {name}: {exc}" for name, exc in errors)
    raise RuntimeError(f"All providers failed:\n{summary}")
