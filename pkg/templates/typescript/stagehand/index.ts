import { Stagehand } from "@browserbasehq/stagehand";
import { Kernel, type KernelContext } from '@onkernel/sdk';
import { z } from 'zod';

const kernel = new Kernel();

const app = kernel.app('ts-stagehand');

interface CompanyInput {
  company: string;
}

interface TeamSizeOutput {
  teamSize: string;
}

// LLM API Keys are set in the environment during `kernel deploy <filename> -e OPENAI_API_KEY=XXX`
// See https://www.onkernel.com/docs/apps/deploy#environment-variables

const OPENAI_API_KEY = process.env.OPENAI_API_KEY;

if (!OPENAI_API_KEY) {
  throw new Error('OPENAI_API_KEY is not set');
}

app.action<CompanyInput, TeamSizeOutput>(
  'teamsize-task',
  async (ctx: KernelContext, payload?: CompanyInput): Promise<TeamSizeOutput> => {
    // A function that returns the team size of a Y Combinator startup

    // Args:
    //     ctx: Kernel context containing invocation information
    //     payload: A startup name to search for on YCombinator's website

    // Returns:
    //     output: The team size (number of employees) of the startup

    const company = payload?.company || 'kernel';

    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
      stealth: true,
    });

    console.log("Kernel browser live view url: ", kernelBrowser.browser_live_view_url);

    const stagehand = new Stagehand({
      env: "LOCAL",
      localBrowserLaunchOptions: {
        cdpUrl: kernelBrowser.cdp_ws_url,
      },
      model: "openai/gpt-4.1",
      apiKey: OPENAI_API_KEY,
      verbose: 1,
      domSettleTimeout: 30_000
    });
    await stagehand.init();

    /////////////////////////////////////
    // Your Stagehand implementation here
    /////////////////////////////////////
    const page = stagehand.context.pages()[0];
    await page.goto("https://www.ycombinator.com/companies");

    await stagehand.act(`Type in ${company} into the search box`);
    await stagehand.act("Click on the first search result");

    // Schema definition
    const teamSizeSchema = z.object({
      teamSize: z.string(),
    });
    // Extract team size from the YC startup page
    const output = await stagehand.extract(
      "Extract the team size (number of employees) shown on this Y Combinator company page.",
      teamSizeSchema
    );
    await stagehand.close();
    await kernel.browsers.deleteByID(kernelBrowser.session_id);

    return output;
  },
);
