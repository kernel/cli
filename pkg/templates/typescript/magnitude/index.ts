import { Kernel, type KernelContext } from "@onkernel/sdk";
import { startBrowserAgent } from "magnitude-core";
import { z } from "zod";

const kernel = new Kernel();

const app = kernel.app("ts-magnitude");

interface UrlInput {
  url: string;
}

interface UrlsOutput {
  urls: string[];
}

// Use env var + inline model directly in the agent call.

app.action<UrlInput, UrlsOutput>(
  "mag-url-extract",
  async (ctx: KernelContext, payload?: UrlInput): Promise<UrlsOutput> => {
    if (!payload?.url) {
      throw new Error("URL is required");
    }

    let target = payload.url;
    if (!target.startsWith("http://") && !target.startsWith("https://")) {
      target = `https://${target}`;
    }
    try {
      // Validate URL
      new URL(target);
    } catch {
      throw new Error(`Invalid URL: ${target}`);
    }

    // Create a Kernel browser so we get a live view and observability
    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
      stealth: true,
    });

    console.log(
      "Kernel browser live view url:",
      kernelBrowser.browser_live_view_url
    );

    // Start a Magnitude BrowserAgent connected to the Kernel browser via CDP
    const agent = await startBrowserAgent({
      narrate: true,
      url: target,
      llm: {
        provider: "anthropic",
        options: {
          model: "claude-sonnet-4-20250514",
          apiKey: process.env.ANTHROPIC_API_KEY as string,
        },
      },
      browser: { 
        cdp: kernelBrowser.cdp_ws_url, 
      },
      virtualScreenDimensions: { width: 1024, height: 768 }
    });

    try {
      //////////////////////////////////////
      // Navigate and extract via Magnitude
      //////////////////////////////////////
      await agent.act(
        `Go to ${target}. Explore the page by scrolling down the page twice. Narrate key steps.`
      );

      const urls = await agent.extract(
        "Extract up to 5 absolute URLs on the current page",
        z.array(z.string().url()).describe("List of absolute URLs")
      );

      return { urls };
    } finally {
      try {
        await agent.stop();
      } catch (e) {
        console.warn("Warning: failed to stop agent", e);
      }
      try {
        await kernel.invocations.deleteBrowsers(ctx.invocation_id);
        console.log(`Browsers for invocation ${ctx.invocation_id} cleaned up successfully`);
      } catch (e) {
        console.warn(
          `Warning: failed to clean up browsers for ${ctx.invocation_id}`,
          e
        );
      }
    }
  }
);
