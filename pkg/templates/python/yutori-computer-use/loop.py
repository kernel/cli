"""
Yutori n1 Sampling Loop

Implements the agent loop for Yutori's n1 computer use model.
n1 uses an OpenAI-compatible API with specific conventions:
- Screenshots use role: "observation" (not "user")
- Coordinates are returned in 1000x1000 space and need scaling
- WebP format recommended for screenshots

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
    """Protocol for tools that can execute n1 actions."""

    async def execute(self, action: N1Action) -> ToolResult:
        ...

    async def screenshot(self) -> ToolResult:
        ...

# n1 uses its own system prompt - custom prompts may degrade performance
# Per docs: "we generally do not recommend providing custom system prompts"


async def sampling_loop(
    *,
    model: str = "n1-preview-2025-11",
    task: str,
    api_key: str,
    kernel: Kernel,
    session_id: str,
    cdp_ws_url: Optional[str] = None,
    max_tokens: int = 4096,
    max_iterations: int = 50,
    # Default viewport matches Yutori's recommended 1280x800 (WXGA 16:10)
    viewport_width: int = 1280,
    viewport_height: int = 800,
    mode: BrowserMode = "computer_use",
) -> dict[str, Any]:
    """
    Run the n1 sampling loop until the model returns a stop action or max iterations.

    Args:
        model: The n1 model to use
        task: The user's task/query
        api_key: Yutori API key
        kernel: Kernel client instance
        session_id: Browser session ID
        cdp_ws_url: CDP WebSocket URL for playwright mode
        max_tokens: Maximum tokens per response
        max_iterations: Maximum loop iterations
        viewport_width: Browser viewport width for coordinate scaling
        viewport_height: Browser viewport height for coordinate scaling
        mode: Browser interaction mode ('computer_use' or 'playwright')

    Returns:
        Dict with 'messages' (conversation history) and 'final_answer' (if stopped)
    """
    client = OpenAI(
        api_key=api_key,
        base_url="https://api.yutori.com/v1",
    )

    # Create the appropriate tool based on mode
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
        # Take initial screenshot
        initial_screenshot = await computer_tool.screenshot()

        # Build conversation per n1 format:
        # 1. User message with task
        # 2. Observation message with screenshot
        conversation_messages: list[dict[str, Any]] = [
            {
                "role": "user",
                "content": [{"type": "text", "text": task}],
            }
        ]

        # Add initial screenshot as observation (n1's required format)
        if initial_screenshot.get("base64_image"):
            conversation_messages.append({
                "role": "observation",
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

            # Call the n1 API (no system prompt - n1 uses its own)
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

            # Add assistant message to conversation
            conversation_messages.append({
                "role": "assistant",
                "content": response_content,
            })

            # Parse the action(s) from the response
            # n1 returns JSON with "thoughts" and "actions" array
            parsed = _parse_n1_response(response_content)

            if not parsed or not parsed.get("actions"):
                print("No actions found in response, ending loop")
                break

            # Execute each action in the actions array
            for action in parsed["actions"]:
                print(f"Executing action: {action.get('action_type')}", action)

                # Check for stop action
                if action.get("action_type") == "stop":
                    final_answer = action.get("answer")
                    print(f"Stop action received, final answer: {final_answer}")
                    return {"messages": conversation_messages, "final_answer": final_answer}

                # Scale coordinates from n1's 1000x1000 space to actual viewport
                scaled_action = _scale_coordinates(action, viewport_width, viewport_height)

                # Execute the action
                result: ToolResult
                try:
                    result = await computer_tool.execute(scaled_action)
                except Exception as e:
                    print(f"Action failed: {e}")
                    result = {"error": str(e)}

                # After action, add observation with screenshot and optional text output
                if result.get("base64_image") or result.get("output"):
                    observation_content = []

                    # Add text output first (e.g., from read_texts_and_links)
                    if result.get("output"):
                        observation_content.append({
                            "type": "text",
                            "text": result["output"],
                        })

                    # Add screenshot
                    if result.get("base64_image"):
                        observation_content.append({
                            "type": "image_url",
                            "image_url": {
                                "url": f"data:image/png;base64,{result['base64_image']}"
                            },
                        })

                    conversation_messages.append({
                        "role": "observation",
                        "content": observation_content,
                    })
                elif result.get("error"):
                    # If there was an error, add it as text observation
                    conversation_messages.append({
                        "role": "observation",
                        "content": [{"type": "text", "text": f"Action failed: {result['error']}"}],
                    })

        if iteration >= max_iterations:
            print("Max iterations reached")

        return {
            "messages": conversation_messages,
            "final_answer": final_answer,
        }
    finally:
        # Clean up playwright connection if used
        if playwright_tool:
            await playwright_tool.disconnect()


def _parse_n1_response(content: str) -> Optional[dict[str, Any]]:
    """
    Parse n1's response format: { "thoughts": "...", "actions": [...] }
    """
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
    """
    Scale coordinates from n1's 1000x1000 space to actual viewport dimensions.
    Per docs: "n1-preview-2025-11 outputs relative coordinates in 1000×1000"
    """
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
