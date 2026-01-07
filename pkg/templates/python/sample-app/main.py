import kernel
from kernel import Kernel
from playwright.async_api import async_playwright
from typing import TypedDict
from urllib.parse import urlparse

client = Kernel()

# Create a new Kernel app
app = kernel.App("python-basic")

"""
Example app that extracts the title of a webpage
Args:
    ctx: Kernel context containing invocation information
    payload: An object with a URL property
Returns:
    A dictionary containing the page title
Invoke this via CLI:
    kernel login  # or: export KERNEL_API_KEY=<your_api_key>
    kernel deploy main.py # If you haven't already deployed this app
    kernel invoke python-basic get-page-title -p '{"url": "https://www.google.com"}'
    kernel logs python-basic -f # Open in separate tab
"""
class PageTitleInput(TypedDict):
    url: str

class PageTitleOutput(TypedDict):
    title: str

@app.action("get-page-title")
async def get_page_title(ctx: kernel.KernelContext, input_data: PageTitleInput) -> PageTitleOutput:
    url = input_data.get("url")
    if not url or not isinstance(url, str):
        raise ValueError("URL is required and must be a string")

    # Add https:// if no protocol is present
    if not url.startswith(('http://', 'https://')):
        url = f"https://{url}"

    # Validate the URL
    try:
        urlparse(url)
    except Exception:
        raise ValueError(f"Invalid URL: {url}")

    # Create a browser instance using the context's invocation_id
    kernel_browser = client.browsers.create(invocation_id=ctx.invocation_id)
    print("Kernel browser live view url: ", kernel_browser.browser_live_view_url)
    
    async with async_playwright() as playwright:
        browser = await playwright.chromium.connect_over_cdp(kernel_browser.cdp_ws_url)
        context = await browser.new_context()
        page = await context.new_page()
        
        try:
            ####################################
            # Your browser automation logic here
            ####################################
            await page.goto(url)
            title = await page.title()
            
            return {"title": title}
        finally:
            client.browsers.delete_by_id(kernel_browser.session_id)


"""
Example app that creates a long-running Kernel browser for manual testing
Invoke this action to test Kernel browsers manually with our browser live view
https://onkernel.com/docs/browsers/live-view
Args:
    ctx: Kernel context containing invocation information
Returns:
    A dictionary containing the browser live view url
Invoke this via CLI:
    kernel login  # or: export KERNEL_API_KEY=<your_api_key>
    kernel deploy main.py # If you haven't already deployed this app
    kernel invoke python-basic create-browser-for-testing
    kernel logs python-basic -f # Open in separate tab
"""
class CreateBrowserForTestingOutput(TypedDict):
    browser_live_view_url: str

@app.action("create-browser-for-testing")
async def create_browser_for_testing(ctx: kernel.KernelContext) -> CreateBrowserForTestingOutput:
    kernel_browser = client.browsers.create(
        invocation_id=ctx.invocation_id,
        stealth=True,
        timeout_seconds=3600,  # Keep browser alive for 1 hour
    )

    return {
      "browser_live_view_url": kernel_browser.browser_live_view_url,
    }
