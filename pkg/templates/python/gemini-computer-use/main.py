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
    error: Optional[str]


api_key = os.getenv("GOOGLE_API_KEY")
if not api_key:
    raise ValueError(
        "GOOGLE_API_KEY is not set. "
        "Set it via environment variable or deploy with: kernel deploy main.py --env-file .env"
    )

app = kernel.App("python-gemini-cua")


@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: QueryInput,
) -> QueryOutput:
    """
    Process a user query using Gemini Computer Use with Kernel's browser automation.

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
        raise ValueError('Query is required. Payload must include: {"query": "your task description"}')

    record_replay = payload.get("record_replay", False)

    async with KernelBrowserSession(
        stealth=True,
        record_replay=record_replay,
    ) as session:
        print("Kernel browser live view url:", session.live_view_url)

        result = await sampling_loop(
            model="gemini-2.5-computer-use-preview-10-2025",
            query=payload["query"],
            api_key=str(api_key),
            kernel=session.kernel,
            session_id=session.session_id,
        )

        final_response = result.get("final_response", "")

    return {
        "result": final_response,
        "replay_url": session.replay_view_url,
        "error": result.get("error"),
    }
