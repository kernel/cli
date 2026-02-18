"""
Yutori n1 Sampling Loop

Implements the agent loop for Yutori's n1 computer use model.
n1 uses an OpenAI-compatible API with specific conventions:
- Screenshots and tool results are sent with role: "user"
- Coordinates are returned in 1000x1000 space and need scaling

Supports two modes:
- computer_use: Uses Kernel's Computer Controls API (full VM screenshots)
- playwright: Uses Playwright via CDP (viewport-only screenshots, optimized for n1)

@see https://docs.yutori.com/reference/n1
"""

import json
import re
from typing import Any, Literal, Optional, Protocol

from kernel import Kernel
from openai import OpenAI

from tools import ComputerTool, N1Action, ToolResult
from tools.playwright_computer import PlaywrightComputerTool

# Mode for browser interaction
BrowserMode = Literal["computer_use", "playwright"]


class N1ComputerToolProtocol(Protocol):
    async def execute(self, action: N1Action) -> ToolResult:
        ...

    async def screenshot(self) -> ToolResult:
        ...

# n1 uses its own system prompt - custom prompts may degrade performance
# Per docs: "we generally do not recommend providing custom system prompts"


async def sampling_loop(
    *,
    model: str = "n1-latest",
    task: str,
    api_key: str,
    kernel: Kernel,
    session_id: str,
    cdp_ws_url: Optional[str] = None,
    max_tokens: int = 4096,
    max_iterations: int = 50,
    viewport_width: int = 1280,
    viewport_height: int = 800,
    mode: BrowserMode = "computer_use",
) -> dict[str, Any]:
    """Run the n1 sampling loop until the model returns a stop action or max iterations."""
    client = OpenAI(
        api_key=api_key,
        base_url="https://api.yutori.com/v1",
    )

    computer_tool: N1ComputerToolProtocol
    playwright_tool: Optional[PlaywrightComputerTool] = None

    print(f"Mode requested: {mode!r}, cdp_ws_url available: {cdp_ws_url is not None}")

    if mode == "playwright":
        if not cdp_ws_url:
            raise ValueError("cdp_ws_url is required for playwright mode")
        print(f"Connecting to CDP WebSocket: {cdp_ws_url[:50]}...")
        playwright_tool = PlaywrightComputerTool(cdp_ws_url, viewport_width, viewport_height)
        await playwright_tool.connect()
        computer_tool = playwright_tool
        print("Using playwright mode (viewport-only screenshots)")
    else:
        computer_tool = ComputerTool(kernel, session_id, viewport_width, viewport_height)
        print("Using computer_use mode (Computer Controls API)")

    try:
        initial_screenshot = await computer_tool.screenshot()

        conversation_messages: list[dict[str, Any]] = [
            {
                "role": "user",
                "content": [{"type": "text", "text": task}],
            }
        ]

        if initial_screenshot.get("base64_image"):
            conversation_messages.append({
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {
                            "url": f"data:image/png;base64,{initial_screenshot['base64_image']}"
                        },
                    }
                ],
            })

        iteration = 0
        final_answer: Optional[str] = None

        while iteration < max_iterations:
            iteration += 1
            print(f"\n=== Iteration {iteration} ===")

            try:
                response = client.chat.completions.create(
                    model=model,
                    messages=conversation_messages,
                    max_tokens=max_tokens,
                    temperature=0.3,
                )
            except Exception as api_error:
                print(f"API call failed: {api_error}")
                raise api_error

            if not response.choices or len(response.choices) == 0:
                print(f"No choices in response: {response}")
                raise ValueError("No choices in API response")

            assistant_message = response.choices[0].message
            if not assistant_message:
                raise ValueError("No response from model")

            response_content = assistant_message.content or ""
            print("Assistant response:", response_content)

            conversation_messages.append({
                "role": "assistant",
                "content": response_content,
            })

            parsed = _parse_n1_response(response_content)

            if not parsed or not parsed.get("actions"):
                print("No actions found in response, ending loop")
                break

            for action in parsed["actions"]:
                print(f"Executing action: {action.get('action_type')}", action)

                if action.get("action_type") == "stop":
                    final_answer = action.get("answer")
                    print(f"Stop action received, final answer: {final_answer}")
                    return {"messages": conversation_messages, "final_answer": final_answer}

                scaled_action = _scale_coordinates(action, viewport_width, viewport_height)

                result: ToolResult
                try:
                    result = await computer_tool.execute(scaled_action)
                except Exception as e:
                    print(f"Action failed: {e}")
                    result = {"error": str(e)}

                if result.get("base64_image") or result.get("output"):
                    result_content = []

                    if result.get("output"):
                        result_content.append({
                            "type": "text",
                            "text": result["output"],
                        })

                    if result.get("base64_image"):
                        result_content.append({
                            "type": "image_url",
                            "image_url": {
                                "url": f"data:image/png;base64,{result['base64_image']}"
                            },
                        })

                    conversation_messages.append({
                        "role": "user",
                        "content": result_content,
                    })
                elif result.get("error"):
                    conversation_messages.append({
                        "role": "user",
                        "content": [{"type": "text", "text": f"Action failed: {result['error']}"}],
                    })

        if iteration >= max_iterations:
            print("Max iterations reached")

        return {
            "messages": conversation_messages,
            "final_answer": final_answer,
        }
    finally:
        if playwright_tool:
            await playwright_tool.disconnect()


def _parse_n1_response(content: str) -> Optional[dict[str, Any]]:
    try:
        # The response should be JSON
        return json.loads(content)
    except json.JSONDecodeError:
        # Try to extract JSON from the response if it's wrapped in text
        json_match = re.search(r'\{[\s\S]*\}', content)
        if json_match:
            try:
                return json.loads(json_match.group(0))
            except json.JSONDecodeError:
                print(f"Failed to parse action JSON: {json_match.group(0)}")
        return None


def _scale_coordinates(action: N1Action, viewport_width: int, viewport_height: int) -> N1Action:
    scaled = dict(action)

    if "center_coordinates" in scaled and scaled["center_coordinates"]:
        coords = scaled["center_coordinates"]
        scaled["center_coordinates"] = [
            round((coords[0] / 1000) * viewport_width),
            round((coords[1] / 1000) * viewport_height),
        ]

    if "start_coordinates" in scaled and scaled["start_coordinates"]:
        coords = scaled["start_coordinates"]
        scaled["start_coordinates"] = [
            round((coords[0] / 1000) * viewport_width),
            round((coords[1] / 1000) * viewport_height),
        ]

    return scaled
