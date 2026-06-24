import { Kernel, type KernelContext } from '@onkernel/sdk';
import { CuaAgent } from '@onkernel/cua-agent';
import type { AssistantMessage } from '@onkernel/cua-ai';
import { KernelBrowserSession } from './session';

const kernel = new Kernel();

const app = kernel.app('ts-anthropic-cua');

interface QueryInput {
  query: string;
  record_replay?: boolean;
}

interface QueryOutput {
  result: string;
  replay_url?: string;
}

const CURRENT_DATE = new Intl.DateTimeFormat('en-US', {
  weekday: 'long',
  month: 'long',
  day: 'numeric',
  year: 'numeric',
}).format(new Date());

// System prompt optimized for the Kernel cloud browser environment.
const SYSTEM_PROMPT = `<SYSTEM_CAPABILITY>
* You are utilising an Ubuntu virtual machine using ${process.arch} architecture with internet access.
* When you connect to the display, CHROMIUM IS ALREADY OPEN. The url bar is not visible but it is there.
* If you need to navigate to a new page, use ctrl+l to focus the url bar and then enter the url.
* You won't be able to see the url bar from the screenshot but ctrl-l still works.
* As the initial step click on the search bar.
* When viewing a page it can be helpful to zoom out so that you can see everything on the page.
* Either that, or make sure you scroll down to see everything before deciding something isn't available.
* Scroll action: scroll_amount and the tool result are in wheel units (not pixels).
* When using your computer function calls, they take a while to run and send back to you.
* Where possible/feasible, try to chain multiple of these calls all into one function calls request.
* The current date is ${CURRENT_DATE}.
* After each step, take a screenshot and carefully evaluate if you have achieved the right outcome.
* Explicitly show your thinking: "I have evaluated step X..." If not correct, try again.
* Only when you confirm a step was executed correctly should you move on to the next one.
</SYSTEM_CAPABILITY>

<IMPORTANT>
* When using Chromium, if a startup wizard appears, IGNORE IT. Do not even click "skip this step".
* Instead, click on the search bar on the center of the screen where it says "Search or enter address", and enter the appropriate search term or URL there.
</IMPORTANT>`;

// LLM API keys are set in the environment during `kernel deploy <filename> -e ANTHROPIC_API_KEY=XXX`.
// See https://www.kernel.sh/docs/launch/deploy#environment-variables
// CuaAgent reads ANTHROPIC_API_KEY (or ANTHROPIC_OAUTH_TOKEN) from the environment by default.
if (!process.env.ANTHROPIC_API_KEY) {
  throw new Error('ANTHROPIC_API_KEY is not set');
}

app.action<QueryInput, QueryOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: QueryInput): Promise<QueryOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required');
    }

    // Create browser session with optional replay recording
    const session = new KernelBrowserSession(kernel, {
      invocationId: ctx.invocation_id,
      stealth: true,
      recordReplay: payload.record_replay ?? false,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    try {
      const agent = new CuaAgent({
        browser: session.browser,
        client: kernel,
        initialState: {
          model: 'anthropic:claude-sonnet-4-6',
          systemPrompt: SYSTEM_PROMPT,
        },
      });

      await agent.prompt(payload.query);

      const lastAssistant = [...agent.state.messages]
        .reverse()
        .find((message): message is AssistantMessage => message.role === 'assistant');
      const result = lastAssistant?.content
        .flatMap((block) => (block.type === 'text' ? [block.text] : []))
        .join('') ?? '';

      const sessionInfo = await session.stop();

      return {
        result,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      console.error('Error running CUA task:', error);
      await session.stop();
      throw error;
    }
  },
);
