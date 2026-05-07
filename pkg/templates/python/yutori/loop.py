"""
Yutori n1.5 Sampling Loop

Implements the agent loop for Yutori's n1.5-latest computer use model.
n1.5-latest uses an OpenAI-compatible API with tool_calls:
- Actions are returned via tool_calls in the assistant message
- Tool results use role: "tool" with matching tool_call_id
- The model stops by returning content without tool_calls
- Coordinates are returned in 1000x1000 space and need scaling

@see https://docs.yutori.com/reference/n1-5
"""

import copy
import json
from typing import Any, Optional

from kernel import Kernel
from openai import OpenAI

from tools import ComputerTool, N15Action, ToolResult

# Tools that require a Playwright page / DOM access. The default core tool set
# already excludes them, but we also list them in `disable_tools` so the
# exclusion is explicit and survives if the default ever changes.
DISABLED_TOOLS = ["extract_elements", "find", "set_element_value", "execute_js"]
TOOL_SET = "browser_tools_core-20260403"

# Screenshot-trimming defaults mirror Yutori's reference loop:
# https://github.com/yutori-ai/yutori-sdk-python/blob/main/yutori/navigator/payload.py
# Trimming is size-triggered — we only drop old screenshots when the payload
# exceeds MAX_REQUEST_BYTES, and we always keep at least KEEP_RECENT_SCREENSHOTS.
MAX_REQUEST_BYTES = 9_500_000
KEEP_RECENT_SCREENSHOTS = 6


async def sampling_loop(
    *,
    model: str = "n1.5-latest",
    task: str,
    api_key: str,
    kernel: Kernel,
    session_id: str,
    max_completion_tokens: int = 4096,
    max_iterations: int = 50,
    viewport_width: int = 1280,
    viewport_height: int = 800,
    kiosk_mode: bool = False,
) -> dict[str, Any]:
    """Run the n1 sampling loop until the model stops calling tools or max iterations."""
    client = OpenAI(
        api_key=api_key,
        base_url="https://api.yutori.com/v1",
    )

    computer_tool = ComputerTool(kernel, session_id, viewport_width, viewport_height, kiosk_mode=kiosk_mode)

    initial_screenshot = await computer_tool.screenshot()

    user_content: list[dict[str, Any]] = [{"type": "text", "text": task}]
    if initial_screenshot.get("base64_image"):
        user_content.append({
            "type": "image_url",
            "image_url": {
                "url": f"data:image/webp;base64,{initial_screenshot['base64_image']}"
            },
        })

    conversation_messages: list[dict[str, Any]] = [
        {"role": "user", "content": user_content}
    ]

    iteration = 0
    final_answer: Optional[str] = None

    while iteration < max_iterations:
        iteration += 1
        print(f"\n=== Iteration {iteration} ===")

        request_messages, dropped = _trimmed_for_request(conversation_messages)
        if dropped:
            print(f"Trimmed {dropped} old screenshot(s) to fit request size limit")

        try:
            response = client.chat.completions.create(
                model=model,
                messages=request_messages,
                max_completion_tokens=max_completion_tokens,
                temperature=0.3,
                # n1.5-specific knobs go in extra_body.
                # tool_set selects the core (coordinate-based) tools.
                # disable_tools is a defense-in-depth exclusion of DOM/Playwright tools.
                extra_body={
                    "tool_set": TOOL_SET,
                    "disable_tools": DISABLED_TOOLS,
                },
            )
        except Exception as api_error:
            print(f"API call failed: {api_error}")
            raise

        if not response.choices or len(response.choices) == 0:
            print(f"No choices in response: {response}")
            raise ValueError("No choices in API response")

        choice = response.choices[0]
        assistant_message = choice.message
        if not assistant_message:
            raise ValueError("No response from model")

        print("Assistant content:", assistant_message.content or "(none)")

        conversation_messages.append(assistant_message.model_dump(exclude_none=True))

        tool_calls = assistant_message.tool_calls

        # No tool_calls means the model is done
        if not tool_calls:
            final_answer = assistant_message.content or None
            print(f"No tool_calls, model is done. Final answer: {final_answer}")
            break

        for tc in tool_calls:
            action_name = tc.function.name
            try:
                args = json.loads(tc.function.arguments)
            except json.JSONDecodeError:
                print(f"Failed to parse tool_call arguments: {tc.function.arguments}")
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": "Error: failed to parse arguments",
                })
                continue

            action: N15Action = {"action_type": action_name, **args}
            print(f"Executing action: {action_name}", args)

            scaled_action = _scale_coordinates(action, viewport_width, viewport_height)

            result: ToolResult
            try:
                result = await computer_tool.execute(scaled_action)
            except Exception as e:
                print(f"Action failed: {e}")
                result = {"error": str(e)}

            if result.get("base64_image"):
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": [
                        {
                            "type": "image_url",
                            "image_url": {
                                "url": f"data:image/webp;base64,{result['base64_image']}"
                            },
                        }
                    ],
                })
            elif result.get("error"):
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": f"Action failed: {result['error']}",
                })
            else:
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": result.get("output", "OK"),
                })

    if iteration >= max_iterations:
        print("Max iterations reached")

    return {
        "messages": conversation_messages,
        "final_answer": final_answer,
    }


def _trimmed_for_request(
    messages: list[dict[str, Any]],
) -> tuple[list[dict[str, Any]], int]:
    """Return a deep-copied messages list with old screenshots stripped to fit MAX_REQUEST_BYTES.

    The most recent KEEP_RECENT_SCREENSHOTS screenshots are protected. The full
    `messages` list is preserved unchanged for the caller's return value.
    """
    trimmed = copy.deepcopy(messages)
    size = _estimate_size(trimmed)
    if size <= MAX_REQUEST_BYTES:
        return trimmed, 0

    image_indices = [i for i, m in enumerate(trimmed) if _message_has_image(m)]
    if not image_indices:
        return trimmed, 0

    protected = set(image_indices[-max(1, KEEP_RECENT_SCREENSHOTS):])
    removed = 0

    for idx in image_indices:
        if size <= MAX_REQUEST_BYTES:
            break
        if idx in protected:
            continue
        if _strip_one_image(trimmed[idx]):
            removed += 1
            size = _estimate_size(trimmed)

    # If still over, strip from the protected window too — but always keep the latest.
    if size > MAX_REQUEST_BYTES:
        last_idx = image_indices[-1]
        for idx in image_indices:
            if size <= MAX_REQUEST_BYTES:
                break
            if idx == last_idx:
                continue
            if _strip_one_image(trimmed[idx]):
                removed += 1
                size = _estimate_size(trimmed)

    return trimmed, removed


def _estimate_size(messages: list[dict[str, Any]]) -> int:
    return len(json.dumps(messages, separators=(",", ":"), ensure_ascii=False).encode("utf-8"))


def _message_has_image(msg: dict[str, Any]) -> bool:
    content = msg.get("content")
    if not isinstance(content, list):
        return False
    return any(isinstance(p, dict) and p.get("type") == "image_url" for p in content)


def _strip_one_image(msg: dict[str, Any]) -> bool:
    content = msg.get("content")
    if not isinstance(content, list):
        return False

    removed = False
    new_content: list[dict[str, Any]] = []
    for part in content:
        if not removed and isinstance(part, dict) and part.get("type") == "image_url":
            removed = True
            continue
        new_content.append(part)

    if not removed:
        return False

    has_text = any(isinstance(p, dict) and p.get("type") == "text" for p in new_content)
    if not has_text:
        new_content.append({"type": "text", "text": "Screenshot omitted to stay under request size limit."})

    msg["content"] = new_content
    return True


def _scale_coordinates(action: N15Action, viewport_width: int, viewport_height: int) -> N15Action:
    scaled = dict(action)

    if "coordinates" in scaled and scaled["coordinates"]:
        coords = scaled["coordinates"]
        scaled["coordinates"] = [
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
