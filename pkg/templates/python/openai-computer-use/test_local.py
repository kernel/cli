"""
Local test script that creates a remote Kernel browser and runs the CUA agent.
No Kernel app deployment needed.

Usage:
    KERNEL_API_KEY=... OPENAI_API_KEY=... uv run test_local.py
"""

import datetime
import os
import json

from dotenv import load_dotenv

load_dotenv(override=True)

from kernel import Kernel
from agent import Agent
from computers.kernel_computer import KernelComputer


def main():
    if not os.getenv("KERNEL_API_KEY"):
        raise ValueError("KERNEL_API_KEY is not set")
    if not os.getenv("OPENAI_API_KEY"):
        raise ValueError("OPENAI_API_KEY is not set")

    client = Kernel(api_key=os.getenv("KERNEL_API_KEY"))
    browser = client.browsers.create(timeout_seconds=300)
    print(f"> Browser session: {browser.session_id}")
    print(f"> Live view: {browser.browser_live_view_url}")

    computer = KernelComputer(client, browser.session_id)

    try:
        computer.goto("https://duckduckgo.com")

        items = [
            {
                "role": "system",
                "content": f"- Current date and time: {datetime.datetime.utcnow().isoformat()} ({datetime.datetime.utcnow().strftime('%A')})",
            },
            {
                "role": "user",
                "content": "go to ebay.com and look up oberheim ob-x prices and give me a report",
            },
        ]

        agent = Agent(
            computer=computer,
            tools=[],
            acknowledge_safety_check_callback=lambda message: (
                print(f"> safety check: {message}") or True
            ),
        )

        response_items = agent.run_full_turn(
            items,
            debug=True,
            show_images=False,
        )

        print(json.dumps(response_items, indent=2, default=str))
    finally:
        client.browsers.delete_by_id(browser.session_id)
        print("> Browser session deleted")


if __name__ == "__main__":
    main()
