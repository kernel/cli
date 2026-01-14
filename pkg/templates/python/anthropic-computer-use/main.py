import os
from typing import Dict, Optional, TypedDict

import kernel
from loop import sampling_loop
from session import KernelBrowserSession


class QueryInput(TypedDict):
    query: str
    record_replay: Optional[bool]


class QueryOutput(TypedDict):
    result: str
    replay_url: Optional[str]


api_key = os.getenv("ANTHROPIC_API_KEY")
if not api_key:
    raise ValueError("ANTHROPIC_API_KEY is not set")

app = kernel.App("python-anthropic-cua")


@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: QueryInput,
) -> QueryOutput:
    """
    Process a user query using Anthropic Computer Use with Kernel's browser automation.

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

        # Run the sampling loop
        final_messages = await sampling_loop(
            model="claude-sonnet-4-20250514",
            messages=[
                {
                    "role": "user",
                    "content": payload["query"],
                }
            ],
            api_key=str(api_key),
            thinking_budget=1024,
            kernel=session.kernel,
            session_id=session.session_id,
        )

        # Extract the final result
        if not final_messages:
            raise ValueError("No messages were generated during the sampling loop")

        last_message = final_messages[-1]
        if not last_message:
            raise ValueError(
                "Failed to get the last message from the sampling loop"
            )

        result = ""
        if isinstance(last_message.get("content"), str):
            result = last_message["content"]  # type: ignore[assignment]
        else:
            result = "".join(
                block["text"]
                for block in last_message["content"]  # type: ignore[index]
                if isinstance(block, Dict) and block.get("type") == "text"
            )

    # Session is cleaned up, replay_url is available if recording was enabled
    return {
        "result": result,
        "replay_url": session.replay_view_url,
    }
