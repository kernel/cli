import os
from typing import Optional, TypedDict

import kernel
from loop import sampling_loop
from session import KernelBrowserSession


class _QueryInputOptional(TypedDict, total=False):
    record_replay: Optional[bool]
    reasoning_effort: Optional[str]
    user_timezone: Optional[str]
    user_location: Optional[str]


class QueryInput(_QueryInputOptional):
    query: str


class QueryOutput(TypedDict):
    result: str
    replay_url: Optional[str]


api_key = os.getenv("YUTORI_API_KEY")
if not api_key:
    raise ValueError("YUTORI_API_KEY is not set")

app = kernel.App("python-yutori-cua")


@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: QueryInput,
) -> QueryOutput:
    """
    Process a user query using Yutori n2 Computer Use with Kernel's browser automation.

    Args:
        ctx: Kernel context containing invocation information
        payload: An object containing:
            - query: The task/query string to process
            - record_replay: Optional boolean to enable video replay recording
            - reasoning_effort: Optional "none" | "low" | "medium" | "xhigh"
            - user_timezone: Optional IANA tz (e.g. "America/New_York")
            - user_location: Optional free-text location for model context

    Returns:
        A dictionary containing:
            - result: The result of the sampling loop as a string
            - replay_url: URL to view the replay (if recording was enabled)
    """
    if not payload or not payload.get("query"):
        raise ValueError("Query is required")

    async with KernelBrowserSession(
        invocation_id=ctx.invocation_id,
        stealth=True,
        record_replay=payload.get("record_replay", False),
    ) as session:
        print("Kernel browser live view url:", session.live_view_url)

        loop_kwargs: dict = {
            "model": "n2",
            "task": payload["query"],
            "api_key": str(api_key),
            "kernel": session.kernel,
            "session_id": str(session.session_id),
            # n2 sees the whole screen, browser chrome included — the Kernel
            # viewport is the window size, so it is the screen size here.
            "screen_width": session.viewport_width,
            "screen_height": session.viewport_height,
        }
        if payload.get("reasoning_effort"):
            loop_kwargs["reasoning_effort"] = payload["reasoning_effort"]
        if payload.get("user_timezone"):
            loop_kwargs["user_timezone"] = payload["user_timezone"]
        if payload.get("user_location"):
            loop_kwargs["user_location"] = payload["user_location"]

        loop_result = await sampling_loop(**loop_kwargs)

        final_answer = loop_result.get("final_answer")
        messages = loop_result.get("messages", [])

        if final_answer:
            result = final_answer
        else:
            result = _extract_last_assistant_message(messages)

    return {
        "result": result,
        "replay_url": session.replay_view_url,
    }


def _extract_last_assistant_message(messages: list) -> str:
    for msg in reversed(messages):
        if msg.get("role") == "assistant":
            content = msg.get("content")
            if isinstance(content, str) and content:
                return content
    return "Task completed"
