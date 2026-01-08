import kernel
from kernel import Kernel
from playwright.async_api import async_playwright

client = Kernel()
app = kernel.App("python-captcha-solver")

"""
Example showing Kernel's auto-CAPTCHA solver.
Visit the live view url to see the Kernel browser auto-solve the CAPTCHA on the site.

Args:
    ctx: Kernel context containing invocation information
Returns:
    None
    
Invoke this via CLI:
    kernel login  # or: export KERNEL_API_KEY=<your_api_key>
    kernel deploy main.py  # If you haven't already deployed this app
    kernel invoke python-captcha-solver test-captcha-solver
"""


@app.action("test-captcha-solver")
async def test_captcha_solver(ctx: kernel.KernelContext) -> None:
    kernel_browser = client.browsers.create(
        invocation_id=ctx.invocation_id,
        stealth=True,
    )

    try:
        async with async_playwright() as playwright:
            browser = await playwright.chromium.connect_over_cdp(
                kernel_browser.cdp_ws_url
            )

            # Get or create context and page
            context = (
                browser.contexts[0] if browser.contexts else await browser.new_context()
            )
            page = context.pages[0] if context.pages else await context.new_page()

            # Access the live view URL printed below in the logs (logs stream automatically with kernel invoke)
            print(
                "Kernel browser live view url: ", kernel_browser.browser_live_view_url
            )

            # Navigate to a site with a CAPTCHA
            await page.wait_for_timeout(
                10000
            )  # Add a delay to give you time to visit the live view url
            await page.goto("https://www.google.com/recaptcha/api2/demo")
            # Watch Kernel auto-solve the CAPTCHA!
            await browser.close()
    finally:
        client.browsers.delete_by_id(kernel_browser.session_id)
