import asyncio
import datetime
import os
from typing import NotRequired, TypedDict

import kernel
from agent import Agent
from agent.logging import (
    create_event_logger,
    emit_browser_delete_done,
    emit_browser_delete_started,
    emit_browser_new_done,
    emit_browser_new_started,
    emit_session_state,
    quiet_http_transport_logs,
)
from computers.kernel_computer import KernelComputer
from kernel import Kernel
from replay import maybe_start_replay, maybe_stop_replay

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
    replay: NotRequired[bool]


class CuaOutput(TypedDict):
    result: str
    replay_url: NotRequired[str]


api_key = os.getenv("OPENAI_API_KEY")
if not api_key:
    raise ValueError("OPENAI_API_KEY is not set")

quiet_http_transport_logs()
client = Kernel()
app = kernel.App("python-openai-cua")


@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: CuaInput,
) -> CuaOutput:
    if not payload or not payload.get("task"):
        raise ValueError("task is required")

    on_event = create_event_logger()

    browser_create_started_at = datetime.datetime.now()
    emit_browser_new_started(on_event)
    kernel_browser = await asyncio.to_thread(
        client.browsers.create, invocation_id=ctx.invocation_id, stealth=True
    )
    emit_browser_new_done(
        on_event, browser_create_started_at, kernel_browser.browser_live_view_url
    )
    emit_session_state(
        on_event, kernel_browser.session_id, kernel_browser.browser_live_view_url
    )
    replay = await asyncio.to_thread(
        maybe_start_replay,
        client,
        kernel_browser.session_id,
        bool(payload.get("replay", False)),
        on_event,
    )
    replay_url: str | None = None

    def run_agent():
        computer = KernelComputer(client, kernel_browser.session_id, on_event=on_event)
        computer.goto("https://duckduckgo.com")

        now_utc = datetime.datetime.now(datetime.UTC)
        items = [
            {
                "role": "system",
                "content": f"- Current date and time: {now_utc.isoformat()} ({now_utc.strftime('%A')})",
            },
            {"role": "user", "content": payload["task"]},
        ]

        agent = Agent(
            model="gpt-5.4",
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
        result = await asyncio.to_thread(run_agent)
    finally:
        browser_delete_started_at = datetime.datetime.now()
        emit_browser_delete_started(on_event)
        try:
            replay_url = await asyncio.to_thread(
                maybe_stop_replay,
                client,
                kernel_browser.session_id,
                replay,
                on_event,
            )
            await asyncio.to_thread(client.browsers.delete_by_id, kernel_browser.session_id)
        finally:
            emit_browser_delete_done(on_event, browser_delete_started_at)

    if replay_url:
        result["replay_url"] = replay_url
    return result


