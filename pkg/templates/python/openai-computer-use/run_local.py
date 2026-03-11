"""
Local test script that creates a remote Kernel browser and runs the CUA agent.
No Kernel app deployment needed.

Usage:
    KERNEL_API_KEY=... OPENAI_API_KEY=... uv run run_local.py --output text
"""

import argparse
import datetime
import os

from dotenv import load_dotenv

load_dotenv(override=True)

from kernel import Kernel
from agent import Agent
from agent.logging import create_event_logger
from computers.kernel_computer import KernelComputer


def parse_args():
    parser = argparse.ArgumentParser(description="Run OpenAI CUA local test")
    parser.add_argument(
        "--output",
        choices=["text", "jsonl"],
        default="text",
        help="Log output mode",
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Enable verbose debug payload logging",
    )
    return parser.parse_args()


def main():
    args = parse_args()
    if not os.getenv("KERNEL_API_KEY"):
        raise ValueError("KERNEL_API_KEY is not set")
    if not os.getenv("OPENAI_API_KEY"):
        raise ValueError("OPENAI_API_KEY is not set")

    client = Kernel(api_key=os.getenv("KERNEL_API_KEY"))
    on_event = create_event_logger(output=args.output, verbose=args.debug)

    browser_create_started_at = datetime.datetime.now()
    on_event({"event": "backend", "data": {"op": "browsers.new"}})
    browser = client.browsers.create(timeout_seconds=300)
    on_event(
        {
            "event": "backend",
            "data": {
                "op": "browsers.new.done",
                "detail": browser.browser_live_view_url or "",
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
                "session_id": browser.session_id,
                "live_view_url": browser.browser_live_view_url or "",
            },
        }
    )

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
                "content": "go to ebay.com and look up oberheim ob-x prices and give me a report",
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
        on_event({"event": "backend", "data": {"op": "browsers.delete"}})
        try:
            client.browsers.delete_by_id(browser.session_id)
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
        print("> Browser session deleted")


if __name__ == "__main__":
    main()
