"""
Yutori n2 Sampling Loop

Implements the agent loop for Yutori's n2 computer use model.
n2 uses an OpenAI-compatible API with tool_calls:
- GUI actions arrive as a single `computer_batch` call holding an action list
- `bash`, `read`, `write`, and `edit` arrive as their own calls
- Every call needs one tool result with a matching tool_call_id
- The model stops by returning content without tool_calls
- Coordinates are returned in 1000x1000 space and need scaling

@see https://docs.yutori.com/reference/n2
"""

from __future__ import annotations

import copy
import json
import platform
from datetime import datetime
from typing import Any, Optional
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from kernel import Kernel
from openai import OpenAI

from tools import ComputerTool, SystemTools

# Pin the tool set so a change to the server default can't change which tools
# this run exposes. n2 rejects `disable_tools`, so the set is served whole.
TOOL_SET = "computer_use_tools-20260825"

# Requests are capped at 10 MB. n2 also only reads images from the last 2
# image-bearing messages, so anything older is dropped before sending rather
# than paying to upload screenshots the model will discard server-side.
MAX_REQUEST_BYTES = 9_500_000
KEEP_IMAGE_MESSAGES = 2


async def sampling_loop(
    *,
    model: str = "n2",
    task: str,
    api_key: str,
    kernel: Kernel,
    session_id: str,
    # Shared by the reasoning trace and the tool call, so a low value truncates
    # the call.
    max_completion_tokens: int = 16384,
    max_iterations: int = 100,
    screen_width: int = 1280,
    screen_height: int = 800,
    reasoning_effort: Optional[str] = None,
    user_timezone: str = "America/Los_Angeles",
    user_location: str = "San Francisco, CA, US",
) -> dict[str, Any]:
    """Run the n2 sampling loop until the model stops calling tools or max iterations."""
    client = OpenAI(api_key=api_key, base_url="https://api.yutori.com/v1")

    computer_tool = ComputerTool(kernel, session_id, screen_width, screen_height)
    system_tools = SystemTools(kernel, session_id)

    # Append location/timezone/current-date context to the task — mirrors Yutori's
    # format_task_with_context helper and helps the model with date-sensitive
    # judgments. https://github.com/yutori-ai/yutori-sdk-python/blob/main/yutori/navigator/context.py
    conversation_messages: list[dict[str, Any]] = [
        {
            "role": "user",
            "content": [
                {"type": "text", "text": _format_task_with_context(task, user_timezone, user_location)},
                _image_part(await computer_tool.screenshot()),
            ],
        }
    ]

    iteration = 0
    final_answer: Optional[str] = None
    prev_request_id: Optional[str] = None

    while iteration < max_iterations:
        iteration += 1
        print(f"\n=== Iteration {iteration} ===")

        request_messages = _trimmed_for_request(conversation_messages)

        # n2-specific knobs go in extra_body.
        extra_body: dict[str, Any] = {"tool_set": TOOL_SET}
        if reasoning_effort:
            extra_body["reasoning_effort"] = reasoning_effort
        if prev_request_id:
            # Groups the trajectory into one conversation for usage reporting.
            extra_body["prev_request_id"] = prev_request_id

        try:
            response = client.chat.completions.create(
                model=model,
                messages=request_messages,
                max_completion_tokens=max_completion_tokens,
                temperature=0.6,
                extra_body=extra_body,
            )
        except Exception as api_error:
            print(f"API call failed: {api_error}")
            raise

        prev_request_id = getattr(response, "_request_id", None) or prev_request_id

        if not response.choices:
            print(f"No choices in response: {response}")
            raise ValueError("No response from model")

        assistant_message = response.choices[0].message
        print("Assistant content:", assistant_message.content or "(none)")

        # Push the assistant message unchanged — `reasoning_content` rides along
        # on it, and n2 needs it echoed back to keep its reasoning across turns.
        conversation_messages.append(assistant_message.model_dump(exclude_none=True))

        tool_calls = assistant_message.tool_calls

        # No tool_calls means the model is done
        if not tool_calls:
            final_answer = assistant_message.content or None
            print(f"No tool_calls, model is done. Final answer: {final_answer}")
            break

        for index, tool_call in enumerate(tool_calls):
            # n2 usually answers with one call, but `parallel_tool_calls` is
            # pinned on server-side so a turn can carry several. Only the first
            # one was planned against the screenshot the model actually saw, so
            # run it and make the rest re-plan.
            if index > 0:
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tool_call.id,
                    "content": (
                        "Not executed: planned against a screenshot the previous call already "
                        "changed. Re-plan from the latest screenshot."
                    ),
                })
                continue

            conversation_messages.append(
                await _run_tool_call(tool_call, computer_tool, system_tools)
            )

    # If the loop exhausted iterations, prompt the model for a final summary so
    # the caller gets a usable answer instead of empty content. Mirrors Yutori's
    # format_stop_and_summarize helper.
    if iteration >= max_iterations and not final_answer:
        print("Max iterations reached — requesting summary")
        try:
            conversation_messages.append({
                "role": "user",
                "content": [
                    {"type": "text", "text": _format_stop_and_summarize(task)},
                    _image_part(await computer_tool.screenshot()),
                ],
            })

            summary_response = client.chat.completions.create(
                model=model,
                messages=_trimmed_for_request(conversation_messages),
                max_completion_tokens=max_completion_tokens,
                temperature=0.6,
                extra_body={"tool_set": TOOL_SET},
            )
            if summary_response.choices:
                summary = summary_response.choices[0].message
                conversation_messages.append(summary.model_dump(exclude_none=True))
                final_answer = summary.content or None
        except Exception as summary_error:
            print(f"Stop-and-summarize call failed: {summary_error}")

    return {
        "messages": conversation_messages,
        "final_answer": final_answer,
    }


async def _run_tool_call(
    tool_call: Any,
    computer_tool: ComputerTool,
    system_tools: SystemTools,
) -> dict[str, Any]:
    name = tool_call.function.name

    try:
        args = json.loads(tool_call.function.arguments)
    except json.JSONDecodeError:
        print(f"Failed to parse tool_call arguments: {tool_call.function.arguments}")
        return {
            "role": "tool",
            "tool_call_id": tool_call.id,
            "content": "Error: failed to parse arguments",
        }

    print(f"Executing tool: {name}", json.dumps(args)[:500])

    if name == "computer_batch":
        outcome = await computer_tool.run_batch(args.get("actions") or [])
        return {
            "role": "tool",
            "tool_call_id": tool_call.id,
            "content": [
                {"type": "text", "text": outcome.describe()},
                _image_part(await computer_tool.screenshot()),
            ],
        }

    try:
        content = system_tools.execute(name, args)
    except Exception as error:
        print(f"{name} failed: {error}")
        content = f"{name} failed: {error}"

    return {"role": "tool", "tool_call_id": tool_call.id, "content": content}


def _image_part(base64_image: str) -> dict[str, Any]:
    return {
        "type": "image_url",
        "image_url": {"url": f"data:image/webp;base64,{base64_image}"},
    }


def _format_task_with_context(task: str, user_timezone: str, user_location: str) -> str:
    """Append location, timezone, and current date/time to the task message."""
    for timezone_name in [user_timezone, "America/Los_Angeles", "UTC"]:
        try:
            tz = ZoneInfo(timezone_name)
            tz_label = timezone_name
            break
        except (ZoneInfoNotFoundError, ValueError, OSError):
            continue
    else:
        return task

    now = datetime.now(tz)
    day_fmt = "%#d" if platform.system() == "Windows" else "%-d"
    context = "\n".join([
        f"User's location: {user_location}",
        f"User's timezone: {tz_label}",
        f"Current Date: {now.strftime(f'%B {day_fmt}, %Y')}",
        f"Current Time: {now.strftime('%H:%M:%S %Z')}",
        f"Today is: {now.strftime('%A')}",
    ])
    return f"{task}\n\n{context}"


def _format_stop_and_summarize(task: str) -> str:
    return (
        f"Stop here. "
        f"Summarize your current progress and list in detail all the findings "
        f"relevant to the given task:\n{task}\n"
        f"Provide URLs for all relevant results you find and return them in your response. "
        f"If there is no specific URL for a result, "
        f"cite the page URL that the information was found on."
    )


def _trimmed_for_request(messages: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Drop the screenshots n2 will not look at, keeping the text of every turn.

    The model reads images from the last KEEP_IMAGE_MESSAGES image-bearing
    messages and ignores the rest, so those are stripped every request. If the
    payload is still over the cap after that, the previous screenshot goes too —
    the latest one is never dropped, since a request without one badly degrades
    grounding.
    """
    trimmed = copy.deepcopy(messages)
    image_indices = [i for i, m in enumerate(trimmed) if _message_has_image(m)]

    for idx in image_indices[:-KEEP_IMAGE_MESSAGES]:
        _strip_images(trimmed[idx])

    for idx in image_indices[-KEEP_IMAGE_MESSAGES:-1]:
        if _estimate_size(trimmed) <= MAX_REQUEST_BYTES:
            break
        print("Payload still over the 10 MB cap — dropping the previous screenshot too")
        _strip_images(trimmed[idx])

    return trimmed


def _estimate_size(messages: list[dict[str, Any]]) -> int:
    return len(json.dumps(messages, separators=(",", ":"), ensure_ascii=False).encode("utf-8"))


def _message_has_image(msg: dict[str, Any]) -> bool:
    content = msg.get("content")
    if not isinstance(content, list):
        return False
    return any(isinstance(p, dict) and p.get("type") == "image_url" for p in content)


def _strip_images(msg: dict[str, Any]) -> None:
    content = msg.get("content")
    if not isinstance(content, list):
        return

    new_content = [
        part for part in content
        if not (isinstance(part, dict) and part.get("type") == "image_url")
    ]

    has_text = any(isinstance(p, dict) and p.get("type") == "text" for p in new_content)
    if not has_text:
        new_content.append({"type": "text", "text": "Screenshot omitted — superseded by a newer one."})

    msg["content"] = new_content
