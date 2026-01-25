import asyncio
import datetime
import os
from typing import TypedDict, Optional

import kernel
from agent import Agent
from computers.default import KernelComputer
from session import KernelBrowserSession
from kernel import Kernel

"""
Example app that runs an agent using OpenAI CUA with Kernel Computer Controls API.

This uses OS-level input emulation (mouse, keyboard) instead of CDP/Playwright,
which reduces bot detection signals.

Args:
    ctx: Kernel context containing invocation information
    payload: An object with a `task` property and optional `record_replay` flag
Returns:
    An answer to the task and optional replay URL
Invoke this via CLI:
    kernel login  # or: export KERNEL_API_KEY=<your_api_key>
    kernel deploy main.py -e OPENAI_API_KEY=XXXXX --force
    kernel invoke python-openai-cua cua-task -p '{"task":"go to https://news.ycombinator.com and list top 5 articles"}'
"""


class CuaInput(TypedDict):
    task: str
    record_replay: Optional[bool]


class CuaOutput(TypedDict):
    result: str
    replay_url: Optional[str]


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
    """Process a user task using Kernel Computer Controls API."""

    if not payload or not payload.get("task"):
        raise ValueError("task is required")

    record_replay = payload.get("record_replay", False)

    def run_agent():
        with KernelBrowserSession(
            kernel=client,
            stealth=True,
            record_replay=record_replay,
            invocation_id=ctx.invocation_id,
        ) as session:
            print("Kernel browser live view url:", session.live_view_url)

            computer = KernelComputer(
                kernel=client,
                session_id=session.session_id,
            )

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
                debug=True,
                show_images=False,
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

            return {"result": result, "replay_url": session.replay_view_url}

    return await asyncio.to_thread(run_agent)
