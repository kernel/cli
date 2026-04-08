"""
Provider factory and fallback logic.

Creates provider instances and handles automatic fallback on provider errors
(rate limits, API errors). Does NOT fall back on task-level failures.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional, Protocol

from tools import KernelExecutor


@dataclass
class ProviderConfig:
    query: str
    model: Optional[str] = None
    api_key: Optional[str] = None
    viewport_width: int = 1280
    viewport_height: int = 800


@dataclass
class ProviderResult:
    result: str
    provider: str


class CUAProvider(Protocol):
    name: str

    async def run(self, config: ProviderConfig, executor: KernelExecutor) -> ProviderResult: ...


def create_provider(name: str) -> CUAProvider:
    if name == "anthropic":
        from providers.anthropic import AnthropicProvider
        return AnthropicProvider()
    elif name == "openai":
        from providers.openai_provider import OpenAIProvider
        return OpenAIProvider()
    elif name == "gemini":
        from providers.gemini import GeminiProvider
        return GeminiProvider()
    else:
        raise ValueError(f"Unknown provider: {name}. Supported: anthropic, openai, gemini")


# Errors that indicate a provider-level failure (should trigger fallback)
_PROVIDER_ERROR_KEYWORDS = [
    "rate limit", "429", "503", "502", "500", "overloaded", "capacity",
    "api key", "authentication", "unauthorized", "forbidden", "quota",
    "timeout", "connection", "refused", "reset",
]


def _is_provider_error(error: Exception) -> bool:
    msg = str(error).lower()
    return any(kw in msg for kw in _PROVIDER_ERROR_KEYWORDS)


async def run_with_fallback(
    providers: list[str],
    config: ProviderConfig,
    executor: KernelExecutor,
) -> ProviderResult:
    """
    Run a CUA task with automatic fallback across providers.

    Tries the primary provider first. On provider-level errors (rate limits, API errors),
    falls back to the next provider. Does NOT fall back on task-level failures.
    """
    errors: list[tuple[str, Exception]] = []

    for provider_name in providers:
        provider = create_provider(provider_name)
        try:
            print(f"[cua] Trying provider: {provider_name}")
            result = await provider.run(config, executor)
            print(f"[cua] Provider {provider_name} succeeded")
            return result
        except Exception as e:
            errors.append((provider_name, e))
            print(f"[cua] Provider {provider_name} failed: {e}")

            if not _is_provider_error(e):
                raise

            if provider_name == providers[-1]:
                error_details = "\n".join(f"  {name}: {err}" for name, err in errors)
                raise RuntimeError(f"All providers failed:\n{error_details}") from e

            print("[cua] Falling back to next provider...")

    raise RuntimeError("No providers configured")
