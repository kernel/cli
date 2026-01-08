import { Kernel, type KernelContext } from "@onkernel/sdk";
import { chromium } from "playwright-core";

const kernel = new Kernel();

const app = kernel.app("ts-captcha-solver");

/**
 * Example showing Kernel's auto-CAPTCHA solver
 * Visit the live view url to see the Kernel browser auto-solve the CAPTCHA on the site
 *
 * Args:
 *     ctx: Kernel context containing invocation information
 * Returns:
 *     None
 *
 * Invoke this via CLI:
 *  kernel login  # or: export KERNEL_API_KEY=<your_api_key>
 *  kernel deploy index.ts # If you haven't already deployed this app
 *  kernel invoke ts-captcha-solver test-captcha-solver
 */
app.action("test-captcha-solver", async (ctx: KernelContext): Promise<void> => {
  const kernelBrowser = await kernel.browsers.create({
    invocation_id: ctx.invocation_id,
    stealth: true,
  });
  const browser = await chromium.connectOverCDP(kernelBrowser.cdp_ws_url);

  // Access the live view URL printed below in the logs (logs stream automatically with kernel invoke)
  console.log(
    "Kernel browser live view url: ",
    kernelBrowser.browser_live_view_url
  );

  try {
    // Navigate to a site with a CAPTCHA
    const context = browser.contexts()[0] || (await browser.newContext());
    const page = context.pages()[0] || (await context.newPage());
    await page.waitForTimeout(10000); // Add a delay to give you time to visit the live view url
    await page.goto("https://www.google.com/recaptcha/api2/demo");
    // Watch Kernel auto-solve the CAPTCHA!
  } finally {
    await browser.close();
    await kernel.browsers.deleteByID(kernelBrowser.session_id);
  }
});
