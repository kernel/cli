import os
from typing import Optional, TypedDict

import kernel
from loop import sampling_loop
from session import KernelBrowserSession


class QueryInput(TypedDict):
    query: str
    record_replay: Optional[bool]


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
    Process a user query using Yutori n1 Computer Use with Kernel's browser automation.

    Args:
        ctx: Kernel context containing invocation information
        payload: An object containing:
            - query: The task/query string to process
            - record_replay: Optional boolean to enable video replay recording

    Returns:
        A dictionary containing:
            - result: The result of the sampling loop as a string
            - replay_url: URL to view the replay (if recording was enabled)
    """
    if not payload or not payload.get("query"):
        raise ValueError("Query is required")

    record_replay = payload.get("record_replay", False)

    async with KernelBrowserSession(
        stealth=True,
        record_replay=record_replay,
    ) as session:
        print("Kernel browser live view url:", session.live_view_url)

        loop_result = await sampling_loop(
            model="n1-latest",
            task=payload["query"],
            api_key=str(api_key),
            kernel=session.kernel,
            session_id=str(session.session_id),
            viewport_width=session.viewport_width,
            viewport_height=session.viewport_height,
        )

        final_answer = loop_result.get("final_answer")
        messages = loop_result.get("messages", [])

        if final_answer:
            result = final_answer
        else:
            # Extract last assistant message
            result = _extract_last_assistant_message(messages)

    return {
        "result": result,
        "replay_url": session.replay_view_url,
    }


def _extract_last_assistant_message(messages: list) -> str:
    import json

    for msg in reversed(messages):
        if msg.get("role") == "assistant":
            content = msg.get("content")
            if isinstance(content, str):
                # Try to parse the thoughts from JSON response
                try:
                    parsed = json.loads(content)
                    if parsed.get("thoughts"):
                        return parsed["thoughts"]
                except json.JSONDecodeError:
                    return content
    return "Task completed"
