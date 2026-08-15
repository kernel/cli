import { Stagehand, localBrowser, type ModelName } from "@browserbasehq/stagehand";
import { Kernel, type KernelContext } from "@onkernel/sdk";
// Stagehand v4 uses zod v4 schema types.
import { z } from "zod";
import { loadStagehandExtension } from "./stagehand-extension";

const kernel = new Kernel();

const app = kernel.app("ts-stagehand");

interface CompanyInput {
  company: string;
}

interface TeamSizeOutput {
  teamSize: string;
}

// Set MODEL (provider-prefixed, e.g. anthropic/claude-sonnet-4-5, openai/gpt-4.1,
// google/gemini-2.5-flash) and MODEL_API_KEY for that provider in the environment
// during `kernel deploy <filename> -e MODEL=XXX -e MODEL_API_KEY=YYY`.
// See https://www.kernel.sh/docs/apps/deploy#environment-variables
const MODEL = process.env.MODEL as ModelName | undefined;
const MODEL_API_KEY = process.env.MODEL_API_KEY;

if (!MODEL || !MODEL_API_KEY) {
  throw new Error("MODEL and MODEL_API_KEY must be set");
}

app.action<CompanyInput, TeamSizeOutput>(
  "teamsize-task",
  async (ctx: KernelContext, payload?: CompanyInput): Promise<TeamSizeOutput> => {
    // A function that returns the team size of a Y Combinator startup

    // Args:
    //     ctx: Kernel context containing invocation information
    //     payload: A startup name to search for on YCombinator's website

    // Returns:
    //     output: The team size (number of employees) of the startup

    const company = payload?.company || "kernel";

    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
      stealth: true,
    });

    console.log("Kernel browser live view url: ", kernelBrowser.browser_live_view_url);

    // Stagehand only closes browsers it launched, so we close the connection and
    // delete the Kernel browser ourselves, even if the automation throws.
    let stagehand: Awaited<ReturnType<typeof Stagehand.create>> | undefined;
    let browser: Awaited<ReturnType<typeof localBrowser.connect>> | undefined;
    try {
      // Load the Stagehand extension into the running browser. With no
      // `extensionId`, `localBrowser.connect` then loads it over CDP.
      await loadStagehandExtension(kernel, kernelBrowser.session_id);
      browser = await localBrowser.connect({
        cdpUrl: kernelBrowser.cdp_ws_url,
      });
      stagehand = await Stagehand.create({
        browser,
        model: { modelName: MODEL, apiKey: MODEL_API_KEY },
        logging: { level: "info" },
      });

      /////////////////////////////////////
      // Your Stagehand implementation here
      /////////////////////////////////////
      const page = await browser.context.activePage();
      if (!page) throw new Error("No active page in the Kernel browser");
      await page.goto("https://www.ycombinator.com/companies");

      await stagehand.act(`Type in ${company} into the search box`);
      await stagehand.act("Click on the first search result");

      // Extract team size from the YC startup page. Every v4 primitive returns { data }.
      const { data } = await stagehand.extract(
        "Extract the team size (number of employees) shown on this Y Combinator company page.",
        z.object({ teamSize: z.string() }),
      );

      return data;
    } finally {
      // Nested so a rejected close() never skips deleting the Kernel browser.
      try {
        await stagehand?.close();
      } finally {
        try {
          await browser?.close();
        } finally {
          await kernel.browsers.deleteByID(kernelBrowser.session_id);
        }
      }
    }
  },
);
