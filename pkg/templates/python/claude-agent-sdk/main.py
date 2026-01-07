"""
Claude Agent SDK + Kernel Browser Automation

This example demonstrates how to use the Claude Agent SDK with Kernel's
Playwright Execution API to perform browser automation tasks.

The agent is given a single tool that executes Playwright code against
a Kernel browser, allowing Claude to autonomously browse the web.
"""

import asyncio
import json
import os
import subprocess
import sys
from typing import Any, TypedDict

import kernel
from kernel import Kernel

from claude_agent_sdk import (
    ClaudeSDKClient,
    tool,
    create_sdk_mcp_server,
    ClaudeAgentOptions,
    AssistantMessage,
    ResultMessage,
    TextBlock,
    ToolUseBlock,
)

# Initialize Kernel SDK
client = Kernel()

# Create Kernel app
app = kernel.App("py-claude-agent-sdk")

# Ensure API key is set
ANTHROPIC_API_KEY = os.environ.get("ANTHROPIC_API_KEY")
if not ANTHROPIC_API_KEY:
    raise ValueError("ANTHROPIC_API_KEY is not set")


class AgentInput(TypedDict):
    task: str


class AgentOutput(TypedDict):
    result: str
    cost_usd: float
    duration_ms: int


def create_playwright_tool(session_id: str):
    """Create the execute_playwright tool with the given browser session."""

    @tool(
        "execute_playwright",
        """Execute Playwright/TypeScript code against the browser. 
The code runs in a sandboxed environment with access to page, context, and browser objects.
Use 'return' to return values from the script.
Available objects:
- page: The current page instance
- context: The browser context
- browser: The browser instance

Example code:
- Navigate: await page.goto('https://example.com');
- Get title: return await page.title();
- Click: await page.click('button');
- Type: await page.fill('input', 'text');
- Screenshot: return (await page.screenshot()).toString('base64');
- Extract text: return await page.locator('h1').textContent();""",
        {"code": str, "timeout_sec": int},
    )
    async def execute_playwright(args: dict[str, Any]) -> dict[str, Any]:
        code = args.get("code", "")
        timeout_sec = args.get("timeout_sec", 60)

        print(f"\n--- Executing Playwright code ---\n{code}\n---\n")

        try:
            result = client.browsers.playwright.execute(
                session_id,
                code=code,
                timeout_sec=timeout_sec,
            )

            if result.success:
                output = (
                    json.dumps(result.result, indent=2)
                    if result.result is not None
                    else "Code executed successfully (no return value)"
                )
                print(f"Execution result: {output}")

                return {"content": [{"type": "text", "text": output}]}
            else:
                error_msg = f"Execution failed: {result.error or 'Unknown error'}\n{result.stderr or ''}"
                print(f"Execution error: {error_msg}")

                return {
                    "content": [{"type": "text", "text": error_msg}],
                    "is_error": True,
                }
        except Exception as e:
            error_msg = f"Failed to execute Playwright code: {e}"
            print(error_msg)

            return {
                "content": [{"type": "text", "text": error_msg}],
                "is_error": True,
            }

    return execute_playwright


async def run_agent(task: str, invocation_id: str | None = None) -> AgentOutput:
    """
    Core agent logic that can be called from both local CLI and Kernel app.

    Args:
        task: The task for the agent to perform
        invocation_id: Optional Kernel invocation ID for browser association

    Returns:
        AgentOutput with result, cost, and duration
    """
    # Ensure Claude Code is in PATH
    homedir = os.environ.get("HOME", "/root")
    current_path = os.environ.get("PATH", "")
    if f"{homedir}/.local/bin" not in current_path:
        os.environ["PATH"] = f"{homedir}/.local/bin:{current_path}"

    # Create a Kernel browser session
    print("Creating Kernel browser...")
    kernel_browser = client.browsers.create(
        invocation_id=invocation_id,
        stealth=True,
        timeout_seconds=300,  # 5 minutes timeout
    )

    print(f"Browser live view URL: {kernel_browser.browser_live_view_url}")
    print(f"Session ID: {kernel_browser.session_id}")

    try:
        # Create the execute_playwright tool with the browser session
        playwright_tool = create_playwright_tool(kernel_browser.session_id)

        # Create an in-process MCP server with the Playwright execution tool
        playwright_server = create_sdk_mcp_server(
            name="kernel-playwright",
            version="1.0.0",
            tools=[playwright_tool],
        )

        print("\n=== Starting Claude Agent ===")
        print(f"Task: {task}")
        print("=============================\n")

        # Configure agent options
        options = ClaudeAgentOptions(
            model="claude-sonnet-4-20250514",
            system_prompt="""You are a browser automation assistant that can control a web browser to accomplish tasks.

You have access to a tool called "execute_playwright" that lets you run Playwright code against a real browser.

Guidelines:
1. Always start by navigating to the target URL using page.goto()
2. Wait for pages to load before interacting with elements
3. Use descriptive selectors when possible (text content, aria labels, test IDs)
4. Return the results of your queries using 'return' statements
5. If something fails, try alternative approaches

When you've completed the task, summarize what you found or accomplished.""",
            mcp_servers={"kernel-playwright": playwright_server},
            max_turns=20,
            permission_mode="acceptEdits",
            allowed_tools=["mcp__kernel-playwright__execute_playwright"],
        )

        # Run the agent using ClaudeSDKClient for better control
        final_result = ""
        cost_usd = 0.0
        duration_ms = 0

        async with ClaudeSDKClient(options=options) as sdk_client:
            await sdk_client.query(task)

            async for message in sdk_client.receive_response():
                # Process different message types
                if isinstance(message, AssistantMessage):
                    for block in message.content:
                        if isinstance(block, TextBlock):
                            print(f"Claude: {block.text}")
                        elif isinstance(block, ToolUseBlock):
                            print(f"\nUsing tool: {block.name}")
                elif isinstance(message, ResultMessage):
                    if message.subtype == "success":
                        final_result = message.result or ""
                        cost_usd = message.total_cost_usd or 0.0
                        duration_ms = message.duration_ms
                        print("\n=== Agent completed successfully ===")
                        print(f"Final result: {message.result}")
                        print(f"Cost: ${cost_usd:.4f}")
                        print(f"Duration: {duration_ms}ms")
                    else:
                        print("\n=== Agent failed ===")
                        raise RuntimeError(f"Agent failed: {message}")

        return {
            "result": final_result,
            "cost_usd": cost_usd,
            "duration_ms": duration_ms,
        }
    finally:
        # Clean up: Delete the browser session
        print("\nCleaning up browser session...")
        client.browsers.delete_by_id(kernel_browser.session_id)
        print("Browser session deleted.")


def install_claude_code() -> None:
    """
    Install Claude Code runtime (required for Claude Agent SDK).
    This is called on the Kernel app VM before running the agent.
    """
    print("Installing Claude Code runtime...")

    try:
        # First ensure curl is available (try without sudo)
        print("Installing curl...")
        try:
            subprocess.run(
                ["apt-get", "update"],
                capture_output=True,
                timeout=60,
                check=False,
            )
            subprocess.run(
                ["apt-get", "install", "-y", "curl"],
                capture_output=True,
                timeout=60,
                check=False,
            )
        except Exception:
            # If that fails, curl might already be available
            print("apt-get failed, checking if curl is already available...")

        # Now install Claude Code
        print("Installing Claude Code...")
        result = subprocess.run(
            ["bash", "-c", "curl -fsSL https://claude.ai/install.sh | bash"],
            capture_output=True,
            text=True,
            timeout=120,
        )

        if result.stdout:
            print(f"Claude Code install stdout: {result.stdout}")
        if result.stderr:
            print(f"Claude Code install stderr: {result.stderr}")

        # Add Claude Code to PATH for this process
        homedir = os.environ.get("HOME", "/root")
        current_path = os.environ.get("PATH", "")
        os.environ["PATH"] = f"{homedir}/.local/bin:{current_path}"
        print("Added ~/.local/bin to PATH")

        print("Claude Code installed successfully")
    except Exception as e:
        raise RuntimeError(f"Failed to install Claude Code: {e}") from e


# ============================================================================
# Kernel App Action
# ============================================================================


@app.action("agent-task")
async def agent_task(
    ctx: kernel.KernelContext, payload: AgentInput | None = None
) -> AgentOutput:
    """
    Kernel app action for browser automation with Claude Agent SDK.

    Deploy and invoke via CLI:
        kernel login  # or: export KERNEL_API_KEY=<your_api_key>
        kernel deploy main.py --env-file .env
        kernel invoke py-claude-agent-sdk agent-task -p '{"task": "Go to https://news.ycombinator.com and get the top 3 stories"}'

    Args:
        ctx: Kernel context containing invocation information
        payload: An object with a task property

    Returns:
        AgentOutput with result, cost, and duration
    """
    if not payload or not payload.get("task"):
        raise ValueError("task is required")

    # Install Claude Code runtime on the Kernel app VM
    install_claude_code()

    # Run the agent
    return await run_agent(payload["task"], ctx.invocation_id)


# ============================================================================
# Local CLI Execution
# ============================================================================


def is_running_on_kernel() -> bool:
    """
    Check if running on Kernel infrastructure.
    On Kernel, the app runs from /boot/ directory.
    """
    cwd = os.getcwd()
    script_path = sys.argv[0] if sys.argv else ""
    return cwd.startswith("/boot") or script_path.startswith("/boot")


# Run locally if executed directly via CLI (not on Kernel)
if __name__ == "__main__" and not is_running_on_kernel():
    if len(sys.argv) > 1:
        task = sys.argv[1]
    else:
        task = "Go to https://news.ycombinator.com and tell me the titles of the top 3 stories on the front page"

    async def main():
        try:
            result = await run_agent(task)
            print("\n=== Done ===")
            print(f"Result: {result['result']}")
        except Exception as e:
            import traceback

            print(f"Fatal error: {e}")
            traceback.print_exc()
            sys.exit(1)

    asyncio.run(main())
