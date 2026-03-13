"""
Tzafon Northstar Sampling Loop

Implements the agent loop for Tzafon's Northstar CUA Fast model.
Northstar uses the Lightcone SDK with a responses API:
- Actions are returned via computer_call outputs
- Tool results use computer_call_output with screenshot images
- The model stops when no computer_call is in the output or the action is terminal
- Continuation uses previous_response_id for multi-turn context

@see https://docs.lightcone.ai
"""

import asyncio
from typing import Any, Optional

from kernel import Kernel
from tzafon import Lightcone

from tools import ComputerTool

MODEL = "tzafon.northstar-cua-fast"

TOOL = {
    "type": "computer_use",
    "display_width": 1280,
    "display_height": 800,
    "environment": "browser",
}

TERMINAL_ACTIONS = {"terminate", "done", "answer"}


async def sampling_loop(
    *,
    task: str,
    api_key: str,
    kernel: Kernel,
    session_id: str,
    model: str = MODEL,
    max_steps: int = 50,
    viewport_width: int = 1280,
    viewport_height: int = 800,
) -> dict[str, Any]:
    """Run the Northstar CUA loop until the model terminates or max steps are reached."""
    tzafon = Lightcone(api_key=api_key)
    computer = ComputerTool(kernel, session_id, viewport_width, viewport_height)

    tool = {
        **TOOL,
        "display_width": viewport_width,
        "display_height": viewport_height,
    }

    screenshot_url = computer.capture_screenshot()

    response = tzafon.responses.create(
        model=model,
        tools=[tool],
        input=[
            {
                "role": "user",
                "content": [
                    {"type": "input_text", "text": task},
                    {"type": "input_image", "image_url": screenshot_url},
                ],
            }
        ],
    )

    final_result: Optional[str] = None

    for step in range(max_steps):
        computer_call = next(
            (o for o in (response.output or []) if o.type == "computer_call"),
            None,
        )
        if not computer_call:
            break

        action = computer_call.action
        label = action.type
        if action.x is not None:
            label += f" @ ({action.x}, {action.y})"
        if action.text:
            label += f" '{action.text}'"
        print(f"[{step + 1}] {label}")

        if action.type in TERMINAL_ACTIONS:
            final_result = action.result or action.text or action.status
            print(f"Result: {final_result}")
            break

        await computer.execute(action)
        await asyncio.sleep(1)

        screenshot_url = computer.capture_screenshot()
        response = tzafon.responses.create(
            model=model,
            previous_response_id=response.id,
            tools=[tool],
            input=[
                {
                    "type": "computer_call_output",
                    "call_id": computer_call.call_id,
                    "output": {
                        "type": "input_image",
                        "image_url": screenshot_url,
                    },
                }
            ],
        )

    messages: list[str] = []
    for item in response.output or []:
        if item.type == "message":
            for block in item.content or []:
                if block.text:
                    messages.append(block.text)

    return {
        "messages": messages,
        "final_result": final_result,
    }
