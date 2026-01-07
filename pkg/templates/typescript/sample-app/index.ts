import { Kernel, type KernelContext } from "@onkernel/sdk";
import { chromium } from "playwright-core";

const kernel = new Kernel();

const app = kernel.app("ts-basic");

/**
 * Example app that extracts the title of a webpage
 * Args:
 *     ctx: Kernel context containing invocation information
 *     payload: An object with a URL property
 * Returns:
 *     A dictionary containing the page title
 * Invoke this via CLI:
 *  kernel login  # or: export KERNEL_API_KEY=<your_api_key>
 *  kernel deploy index.ts # If you haven't already deployed this app
 *  kernel invoke ts-basic get-page-title -p '{"url": "https://www.google.com"}'
 *  kernel logs ts-basic -f # Open in separate tab
 */
interface PageTitleInput {
  url: string;
}

interface PageTitleOutput {
  title: string;
}
app.action<PageTitleInput, PageTitleOutput>(
  "get-page-title",
  async (
    ctx: KernelContext,
    payload?: PageTitleInput
  ): Promise<PageTitleOutput> => {
    if (!payload?.url) {
      throw new Error("URL is required");
    }

    if (
      !payload.url.startsWith("http://") &&
      !payload.url.startsWith("https://")
    ) {
      payload.url = `https://${payload.url}`;
    }

    // Validate the URL
    try {
      new URL(payload.url);
    } catch {
      throw new Error(`Invalid URL: ${payload.url}`);
    }

    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
    });

    console.log(
      "Kernel browser live view url: ",
      kernelBrowser.browser_live_view_url
    );

    const browser = await chromium.connectOverCDP(kernelBrowser.cdp_ws_url);
    const context = browser.contexts()[0] || (await browser.newContext());
    const page = context.pages()[0] || (await context.newPage());

    try {
      //////////////////////////////////////
      // Your browser automation logic here
      //////////////////////////////////////
      await page.goto(payload.url);
      const title = await page.title();
      return { title };
    } finally {
      await kernel.browsers.deleteByID(kernelBrowser.session_id);
    }
  }
);

/**
 * Example app that creates a long-running Kernel browser for manual testing
 * Invoke this action to test Kernel browsers manually with our browser live view
 * https://www.kernel.sh/docs/browsers/live-view
 * Args:
 *     ctx: Kernel context containing invocation information
 * Returns:
 *     A dictionary containing the browser live view url
 * Invoke this via CLI:
 *  kernel login  # or: export KERNEL_API_KEY=<your_api_key>
 *  kernel deploy index.ts # If you haven't already deployed this app
 *  kernel invoke ts-basic create-browser-for-testing
 *  kernel logs ts-basic -f # Open in separate tab
 */
interface CreateBrowserForTestingOutput {
  browser_live_view_url: string;
}
app.action(
  "create-browser-for-testing",
  async (ctx: KernelContext): Promise<CreateBrowserForTestingOutput> => {
    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
      stealth: true,
      timeout_seconds: 3600, // Keep browser alive for 1 hour
    });

    return {
      browser_live_view_url: kernelBrowser.browser_live_view_url,
    };
  }
);
