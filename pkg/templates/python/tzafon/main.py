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


api_key = os.getenv("TZAFON_API_KEY")
if not api_key:
    raise ValueError("TZAFON_API_KEY is not set")

app = kernel.App("python-tzafon-cua")


@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: QueryInput,
) -> QueryOutput:
    if not payload or not payload.get("query"):
        raise ValueError("Query is required")

    record_replay = payload.get("record_replay", False)

    async with KernelBrowserSession(
        invocation_id=ctx.invocation_id,
        stealth=True,
        record_replay=record_replay,
    ) as session:
        print("Kernel browser live view url:", session.live_view_url)

        loop_result = await sampling_loop(
            task=payload["query"],
            api_key=str(api_key),
            kernel=session.kernel,
            session_id=str(session.session_id),
            viewport_width=session.viewport_width,
            viewport_height=session.viewport_height,
        )

        final_result = loop_result.get("final_result")
        messages = loop_result.get("messages", [])

        if final_result:
            result = final_result
        elif messages:
            result = messages[-1]
        else:
            result = "Task completed"

    return {
        "result": result,
        "replay_url": session.replay_view_url,
    }
