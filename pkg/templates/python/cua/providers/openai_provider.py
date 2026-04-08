"""
OpenAI CUA provider.

Uses OpenAI's Responses API with the computer tool.
"""

import json
import os
from datetime import datetime, UTC
from typing import Any

import httpx

from tools import CommonAction, KernelExecutor
from providers import ProviderConfig, ProviderResult

SYSTEM_INSTRUCTIONS = (
    f"- Current date and time: {datetime.now(UTC).isoformat()} "
    f"({datetime.now(UTC).strftime('%A')})\n"
    "- CHROMIUM IS ALREADY OPEN. Use it directly.\n"
    "- To navigate, use the browser URL bar (Ctrl+L)."
)


def _to_common_action(action: dict[str, Any]) -> CommonAction:
    action_type = action.get("type", "screenshot")
    if action_type == "click":
        if action.get("button") == "back":
            return CommonAction(type="back")
        if action.get("button") == "wheel":
            return CommonAction(
                type="scroll",
                x=action.get("x", 0), y=action.get("y", 0),
                scroll_x=int(action.get("scroll_x", 0)), scroll_y=int(action.get("scroll_y", 0)),
            )
        return CommonAction(type="click", x=action.get("x", 0), y=action.get("y", 0))
    if action_type == "double_click":
        return CommonAction(type="double_click", x=action.get("x", 0), y=action.get("y", 0))
    if action_type == "type":
        return CommonAction(type="type", text=action.get("text", ""))
    if action_type == "keypress":
        keys = action.get("keys", [])
        hold = action.get("hold_keys", [])
        combo = "+".join(hold + keys) if hold else "+".join(keys)
        return CommonAction(type="key", keys=combo)
    if action_type == "scroll":
        return CommonAction(
            type="scroll",
            x=action.get("x", 0), y=action.get("y", 0),
            scroll_x=int(action.get("scroll_x", 0)), scroll_y=int(action.get("scroll_y", 0)),
        )
    if action_type == "move":
        return CommonAction(type="mouse_move", x=action.get("x", 0), y=action.get("y", 0))
    if action_type == "drag":
        path = action.get("path", [])
        return CommonAction(type="drag", path=[[p["x"], p["y"]] for p in path if isinstance(p, dict)])
    if action_type == "wait":
        return CommonAction(type="wait", duration=action.get("ms", 1000))
    if action_type == "goto":
        return CommonAction(type="goto", url=action.get("url", ""))
    if action_type == "back":
        return CommonAction(type="back")
    return CommonAction(type="screenshot")


def _create_response(**kwargs) -> dict:
    """Call OpenAI Responses API."""
    api_key = os.getenv("OPENAI_API_KEY", "")
    with httpx.Client(timeout=300) as client:
        resp = client.post(
            "https://api.openai.com/v1/responses",
            headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
            json=kwargs,
        )
        resp.raise_for_status()
        return resp.json()


class OpenAIProvider:
    name = "openai"

    async def run(self, config: ProviderConfig, executor: KernelExecutor) -> ProviderResult:
        api_key = config.api_key or os.getenv("OPENAI_API_KEY")
        if not api_key:
            raise ValueError("OPENAI_API_KEY is required for OpenAI provider")

        os.environ["OPENAI_API_KEY"] = api_key
        model = config.model or "gpt-5.4"

        # Navigate to starting page
        await executor.execute(CommonAction(type="goto", url="https://duckduckgo.com"))

        input_items: list[dict[str, Any]] = [
            {"role": "system", "content": SYSTEM_INSTRUCTIONS},
            {"role": "user", "content": config.query},
        ]
        new_items: list[dict[str, Any]] = []

        while True:
            last = new_items[-1] if new_items else None
            if last and last.get("role") == "assistant":
                break

            response = _create_response(
                model=model,
                input=input_items + new_items,
                tools=[{"type": "computer"}],
                truncation="auto",
                reasoning={"effort": "low", "summary": "concise"},
                instructions=SYSTEM_INSTRUCTIONS,
            )

            if "output" not in response:
                raise ValueError("No output from OpenAI model")

            for item in response["output"]:
                new_items.append(item)

                if item.get("type") == "computer_call":
                    actions = item.get("actions") or ([item["action"]] if "action" in item else [])
                    for action in actions:
                        if isinstance(action, dict):
                            common = _to_common_action(action)
                            print(f"[openai] action: {common.type}")
                            await executor.execute(common)

                    screenshot = await executor.screenshot()
                    pending = item.get("pending_safety_checks", [])
                    new_items.append({
                        "type": "computer_call_output",
                        "call_id": item["call_id"],
                        "acknowledged_safety_checks": pending,
                        "output": {
                            "type": "computer_screenshot",
                            "image_url": f"data:image/png;base64,{screenshot.base64_image}",
                        },
                    })

                if item.get("type") == "message" and item.get("role") == "assistant":
                    content = item.get("content", [])
                    if isinstance(content, list):
                        for part in content:
                            if isinstance(part, dict) and "text" in part:
                                return ProviderResult(result=part["text"], provider="openai")
                    elif isinstance(content, str) and content:
                        return ProviderResult(result=content, provider="openai")

        # Extract from accumulated items
        for item in reversed(new_items):
            if item.get("type") == "message" and item.get("role") == "assistant":
                content = item.get("content", [])
                if isinstance(content, list) and content:
                    text = content[-1].get("text", "") if isinstance(content[-1], dict) else str(content[-1])
                    if text:
                        return ProviderResult(result=text, provider="openai")

        return ProviderResult(result="Task completed.", provider="openai")
