"""
Anthropic CUA provider.

Uses Claude's computer use beta API with the sampling loop pattern.
"""

import os
from datetime import datetime
from typing import Any, cast

from anthropic import Anthropic
from anthropic.types.beta import (
    BetaContentBlockParam,
    BetaImageBlockParam,
    BetaMessage,
    BetaMessageParam,
    BetaTextBlockParam,
    BetaToolResultBlockParam,
    BetaToolUseBlockParam,
)

from tools import CommonAction, KernelExecutor, ToolResult
from providers import ProviderConfig, ProviderResult

SYSTEM_PROMPT = f"""<SYSTEM_CAPABILITY>
* You are utilising an Ubuntu virtual machine using {os.uname().machine} architecture with internet access.
* When you connect to the display, CHROMIUM IS ALREADY OPEN. The url bar is not visible but it is there.
* If you need to navigate to a new page, use ctrl+l to focus the url bar and then enter the url.
* You won't be able to see the url bar from the screenshot but ctrl-l still works.
* As the initial step click on the search bar.
* When viewing a page it can be helpful to zoom out so that you can see everything on the page.
* Either that, or make sure you scroll down to see everything before deciding something isn't available.
* Scroll action: scroll_amount and the tool result are in wheel units (not pixels).
* The current date is {datetime.now().strftime("%A, %B %d, %Y")}.
* After each step, take a screenshot and carefully evaluate if you have achieved the right outcome.
* Explicitly show your thinking: "I have evaluated step X..." If not correct, try again.
* Only when you confirm a step was executed correctly should you move on to the next one.
</SYSTEM_CAPABILITY>

<IMPORTANT>
* When using Chromium, if a startup wizard appears, IGNORE IT. Do not even click "skip this step".
* Instead, click on the search bar on the center of the screen where it says "Search or enter address", and enter the appropriate search term or URL there.
</IMPORTANT>"""


def _tool_version_for_model(model: str) -> str:
    if "claude-sonnet-4-6" in model or "claude-opus-4-6" in model or "claude-opus-4-5" in model:
        return "computer_use_20251124"
    return "computer_use_20250124"


def _to_common_action(input_data: dict[str, Any]) -> CommonAction:
    action = input_data.get("action", "screenshot")
    coord = input_data.get("coordinate", [0, 0])

    if action == "screenshot":
        return CommonAction(type="screenshot")
    if action in ("left_click", "right_click", "middle_click", "double_click", "triple_click", "mouse_move"):
        type_map = {
            "left_click": "click", "right_click": "right_click", "middle_click": "middle_click",
            "double_click": "double_click", "triple_click": "triple_click", "mouse_move": "mouse_move",
        }
        return CommonAction(type=type_map[action], x=coord[0] if coord else 0, y=coord[1] if coord else 0)
    if action == "left_mouse_down":
        return CommonAction(type="mouse_down", x=coord[0] if coord else 0, y=coord[1] if coord else 0)
    if action == "left_mouse_up":
        return CommonAction(type="mouse_up", x=coord[0] if coord else 0, y=coord[1] if coord else 0)
    if action == "type":
        return CommonAction(type="type", text=input_data.get("text", ""))
    if action in ("key", "hold_key"):
        return CommonAction(type="key", keys=input_data.get("text", ""))
    if action == "scroll":
        direction = input_data.get("scrollDirection") or input_data.get("scroll_direction", "down")
        amount = input_data.get("scrollAmount") or input_data.get("scroll_amount", 3)
        sx, sy = 0, 0
        if direction == "down": sy = amount
        elif direction == "up": sy = -amount
        elif direction == "right": sx = amount
        elif direction == "left": sx = -amount
        return CommonAction(
            type="scroll",
            x=coord[0] if coord else 640, y=coord[1] if coord else 400,
            scroll_x=sx, scroll_y=sy,
        )
    if action == "left_click_drag":
        start = input_data.get("start_coordinate")
        return CommonAction(
            type="drag",
            start_x=start[0] if start else None, start_y=start[1] if start else None,
            end_x=coord[0] if coord else 0, end_y=coord[1] if coord else 0,
        )
    if action == "wait":
        return CommonAction(type="wait", duration=int(input_data.get("duration", 1) * 1000))

    return CommonAction(type="screenshot")


def _make_tool_result(result: ToolResult, tool_use_id: str) -> BetaToolResultBlockParam:
    content: list[BetaTextBlockParam | BetaImageBlockParam] | str = []
    is_error = False
    if result.error:
        is_error = True
        content = result.error
    else:
        if result.output:
            content.append(BetaTextBlockParam(type="text", text=result.output))
        if result.base64_image:
            content.append({"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": result.base64_image}})  # type: ignore
    return {"type": "tool_result", "content": content, "tool_use_id": tool_use_id, "is_error": is_error}


class AnthropicProvider:
    name = "anthropic"

    async def run(self, config: ProviderConfig, executor: KernelExecutor) -> ProviderResult:
        api_key = config.api_key or os.getenv("ANTHROPIC_API_KEY")
        if not api_key:
            raise ValueError("ANTHROPIC_API_KEY is required for Anthropic provider")

        model = config.model or "claude-sonnet-4-6"
        tool_version = _tool_version_for_model(model)
        betas = ["prompt-caching-2024-07-31"]
        if tool_version == "computer_use_20251124":
            betas.append("computer-use-2025-11-24")
        else:
            betas.append("computer-use-2025-01-24")

        client = Anthropic(api_key=api_key, max_retries=4)

        system = BetaTextBlockParam(type="text", text=SYSTEM_PROMPT)
        system["cache_control"] = {"type": "ephemeral"}  # type: ignore

        tool_type = "computer_20251124" if tool_version == "computer_use_20251124" else "computer_20250124"
        tool_params = [{
            "name": "computer",
            "type": tool_type,
            "display_width_px": config.viewport_width,
            "display_height_px": config.viewport_height,
            "display_number": None,
        }]

        messages: list[BetaMessageParam] = [{"role": "user", "content": config.query}]

        while True:
            response = client.beta.messages.create(
                max_tokens=4096,
                messages=messages,
                model=model,
                system=[system],
                tools=tool_params,  # type: ignore
                betas=betas,
                extra_body={"thinking": {"type": "enabled", "budget_tokens": 1024}},
            )

            response_params = _response_to_params(response)
            print(f"[anthropic] stop_reason={response.stop_reason}")
            messages.append({"role": "assistant", "content": response_params})

            if response.stop_reason == "end_turn":
                text_blocks = [b for b in response.content if getattr(b, "type", None) == "text"]
                result_text = " ".join(getattr(b, "text", "") for b in text_blocks)
                return ProviderResult(result=result_text, provider="anthropic")

            tool_results: list[BetaToolResultBlockParam] = []
            for block in response.content:
                if getattr(block, "type", None) == "tool_use":
                    input_data = cast(dict[str, Any], block.input)
                    common = _to_common_action(input_data)
                    print(f"[anthropic] action: {common.type}")
                    result = await executor.execute(common)
                    tool_results.append(_make_tool_result(result, block.id))

            if not tool_results:
                text_blocks = [b for b in response.content if getattr(b, "type", None) == "text"]
                return ProviderResult(
                    result=" ".join(getattr(b, "text", "") for b in text_blocks),
                    provider="anthropic",
                )

            messages.append({"role": "user", "content": tool_results})


def _response_to_params(response: BetaMessage) -> list[BetaContentBlockParam]:
    res: list[BetaContentBlockParam] = []
    for block in response.content:
        block_type = getattr(block, "type", None)
        if block_type == "thinking":
            thinking_block = {"type": "thinking", "thinking": getattr(block, "thinking", None)}
            if hasattr(block, "signature"):
                thinking_block["signature"] = getattr(block, "signature", None)
            res.append(cast(BetaContentBlockParam, thinking_block))
        elif block_type == "text" and getattr(block, "text", None):
            res.append(BetaTextBlockParam(type="text", text=block.text))
        elif block_type == "tool_use":
            res.append({"type": "tool_use", "id": block.id, "name": block.name, "input": block.input})  # type: ignore
        else:
            if hasattr(block, "model_dump"):
                res.append(cast(BetaContentBlockParam, block.model_dump()))
    return res
