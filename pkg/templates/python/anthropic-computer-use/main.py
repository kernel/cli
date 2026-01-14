import os
from typing import Dict, TypedDict

import kernel
from kernel import Kernel
from loop import sampling_loop
from playwright.async_api import async_playwright


class QueryInput(TypedDict):
    query: str


class QueryOutput(TypedDict):
    result: str


api_key = os.getenv("ANTHROPIC_API_KEY")
if not api_key:
    raise ValueError("ANTHROPIC_API_KEY is not set")

client = Kernel()
app = kernel.App("python-anthropic-cua")

@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: QueryInput,
) -> QueryOutput:
    # A function that processes a user query using a browser-based sampling loop

    # Args:
    #     ctx: Kernel context containing invocation information
    #     payload: An object containing a query string to process

    # Returns:
    #     A dictionary containing the result of the sampling loop as a string
    if not payload or not payload.get("query"):
        raise ValueError("Query is required")

    kernel_browser = client.browsers.create(
        invocation_id=ctx.invocation_id, stealth=True
    )
    print("Kernel browser live view url: ", kernel_browser.browser_live_view_url)

    try:
        async with async_playwright() as playwright:
            browser = await playwright.chromium.connect_over_cdp(
                kernel_browser.cdp_ws_url
            )
            context = (
                browser.contexts[0] if browser.contexts else await browser.new_context()
            )
            page = context.pages[0] if context.pages else await context.new_page()

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
                playwright_page=page,
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

            return {"result": result}
    except Exception as exc:
        print(f"Error in sampling loop: {exc}")
        raise
    finally:
        if browser is not None:
            await browser.close()
        client.browsers.delete_by_id(kernel_browser.session_id)
