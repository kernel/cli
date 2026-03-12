"""
Local test script that creates a remote Kernel browser and runs the CUA agent.
No Kernel app deployment needed.

Usage:
    KERNEL_API_KEY=... OPENAI_API_KEY=... uv run run_local.py --task "go to example.com and summarize it"
"""

import argparse
import datetime
import os

from dotenv import load_dotenv

load_dotenv(override=True)

from kernel import Kernel
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
from replay import maybe_start_replay, maybe_stop_replay

DEFAULT_TASK = "go to example.com and summarize what the page says"


def parse_args():
    parser = argparse.ArgumentParser(description="Run OpenAI CUA local test")
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Enable verbose debug payload logging",
    )
    parser.add_argument(
        "--task",
        default=DEFAULT_TASK,
        help="User task prompt to run in the browser session",
    )
    parser.add_argument(
        "--replay",
        action="store_true",
        help="Record a Kernel browser replay for this local run",
    )
    return parser.parse_args()


def main():
    args = parse_args()
    if not os.getenv("KERNEL_API_KEY"):
        raise ValueError("KERNEL_API_KEY is not set")
    if not os.getenv("OPENAI_API_KEY"):
        raise ValueError("OPENAI_API_KEY is not set")

    quiet_http_transport_logs()
    client = Kernel(api_key=os.getenv("KERNEL_API_KEY"))
    on_event = create_event_logger(verbose=args.debug)

    browser_create_started_at = datetime.datetime.now()
    emit_browser_new_started(on_event)
    browser = client.browsers.create(timeout_seconds=300)
    emit_browser_new_done(
        on_event, browser_create_started_at, browser.browser_live_view_url
    )
    emit_session_state(on_event, browser.session_id, browser.browser_live_view_url)
    replay = maybe_start_replay(client, browser.session_id, args.replay, on_event)

    computer = KernelComputer(client, browser.session_id, on_event=on_event)

    try:
        computer.goto("https://duckduckgo.com")

        now_utc = datetime.datetime.now(datetime.UTC)
        items = [
            {
                "role": "system",
                "content": f"- Current date and time: {now_utc.isoformat()} ({now_utc.strftime('%A')})",
            },
            {
                "role": "user",
                "content": args.task,
            },
        ]

        agent = Agent(
            model="gpt-5.4",
            computer=computer,
            tools=[],
            acknowledge_safety_check_callback=lambda message: (
                print(f"> safety check: {message}") or True
            ),
        )

        response_items = agent.run_full_turn(
            items,
            debug=args.debug,
            show_images=False,
            on_event=on_event,
        )
        if not response_items:
            raise ValueError("No response from agent")
    finally:
        browser_delete_started_at = datetime.datetime.now()
        emit_browser_delete_started(on_event)
        try:
            replay_url = maybe_stop_replay(client, browser.session_id, replay, on_event)
            if replay_url:
                print(f"> Replay URL: {replay_url}")
            client.browsers.delete_by_id(browser.session_id)
        finally:
            emit_browser_delete_done(on_event, browser_delete_started_at)
        print("> Browser session deleted")


if __name__ == "__main__":
    main()
