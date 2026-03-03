import asyncio
import datetime
import os
import subprocess
import sys
from typing import NotRequired, TypedDict

import kernel
from agent import Agent
from agent.logging import create_event_logger
from computers.kernel_computer import KernelComputer
from kernel import Kernel

"""
Example app that runs an agent using openai CUA
Args:
    ctx: Kernel context containing invocation information
    payload: An object with a `task` property
Returns:
    An answer to the task, elapsed time and optionally the messages stack
Invoke this via CLI:
    kernel login  # or: export KERNEL_API_KEY=<your_api_key>
    kernel deploy main.py -e OPENAI_API_KEY=XXXXX --force
    kernel invoke python-openai-cua cua-task -p '{"task":"go to https://news.ycombinator.com and list top 5 articles"}'
"""


class CuaInput(TypedDict):
    task: str
    output: NotRequired[str]


class CuaOutput(TypedDict):
    result: str


api_key = os.getenv("OPENAI_API_KEY")
if not api_key:
    raise ValueError("OPENAI_API_KEY is not set")

client = Kernel()
app = kernel.App("python-openai-cua")


@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: CuaInput,
) -> CuaOutput:
    if not payload or not payload.get("task"):
        raise ValueError("task is required")

    output_mode = payload.get("output", "text")
    if output_mode not in ("text", "jsonl"):
        output_mode = "text"
    on_event = create_event_logger(output=output_mode)

    browser_create_started_at = datetime.datetime.now()
    on_event({"event": "backend", "data": {"op": "browsers.new"}})
    kernel_browser = await asyncio.to_thread(
        client.browsers.create, invocation_id=ctx.invocation_id, stealth=True
    )
    on_event(
        {
            "event": "backend",
            "data": {
                "op": "browsers.new.done",
                "detail": kernel_browser.browser_live_view_url or "",
                "elapsed_ms": int(
                    (datetime.datetime.now() - browser_create_started_at).total_seconds()
                    * 1000
                ),
            },
        }
    )
    on_event(
        {
            "event": "session_state",
            "data": {
                "session_id": kernel_browser.session_id,
                "live_view_url": kernel_browser.browser_live_view_url or "",
            },
        }
    )

    def run_agent():
        computer = KernelComputer(client, kernel_browser.session_id, on_event=on_event)
        computer.goto("https://duckduckgo.com")

        items = [
            {
                "role": "system",
                "content": f"- Current date and time: {datetime.datetime.utcnow().isoformat()} ({datetime.datetime.utcnow().strftime('%A')})",
            },
            {"role": "user", "content": payload["task"]},
        ]

        agent = Agent(
            computer=computer,
            tools=[],
            acknowledge_safety_check_callback=lambda message: (
                print(f"> agent : safety check message (skipping): {message}")
                or True
            ),
        )

        response_items = agent.run_full_turn(
            items,
            debug=False,
            show_images=False,
            on_event=on_event,
        )

        if not response_items or "content" not in response_items[-1]:
            raise ValueError("No response from agent")
        content = response_items[-1]["content"]
        if (
            isinstance(content, list)
            and content
            and isinstance(content[0], dict)
            and "text" in content[0]
        ):
            result = content[0]["text"]
        elif isinstance(content, str):
            result = content
        else:
            result = str(content)
        return {"result": result}

    try:
        return await asyncio.to_thread(run_agent)
    finally:
        browser_delete_started_at = datetime.datetime.now()
        on_event({"event": "backend", "data": {"op": "browsers.delete"}})
        try:
            await asyncio.to_thread(client.browsers.delete_by_id, kernel_browser.session_id)
        finally:
            on_event(
                {
                    "event": "backend",
                    "data": {
                        "op": "browsers.delete.done",
                        "elapsed_ms": int(
                            (datetime.datetime.now() - browser_delete_started_at).total_seconds()
                            * 1000
                        ),
                    },
                }
            )


if __name__ == "__main__":
    # `main.py` is the deployable Kernel app entrypoint.
    # For local execution, forward to the existing local harness.
    command = [sys.executable, "run_local.py", *sys.argv[1:]]
    raise SystemExit(subprocess.call(command))
