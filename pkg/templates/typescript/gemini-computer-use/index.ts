import { Kernel, type KernelContext } from '@onkernel/sdk';
import { CuaAgent } from '@onkernel/cua-agent';
import type { AssistantMessage } from '@onkernel/cua-ai';
import { KernelBrowserSession } from './session';

const kernel = new Kernel();

const app = kernel.app('ts-gemini-cua');

interface QueryInput {
  query: string;
  record_replay?: boolean;
}

interface QueryOutput {
  result: string;
  replay_url?: string;
}

// CuaAgent reads GOOGLE_API_KEY (or GEMINI_API_KEY) from the environment by default.
// Set it via environment variable or `kernel deploy index.ts --env-file .env`.
// See https://www.kernel.sh/docs/launch/deploy#environment-variables
if (!process.env.GOOGLE_API_KEY && !process.env.GEMINI_API_KEY) {
  throw new Error(
    'GOOGLE_API_KEY is not set. ' +
    'Set it via environment variable or deploy with: kernel deploy index.ts --env-file .env'
  );
}

app.action<QueryInput, QueryOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: QueryInput): Promise<QueryOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required. Payload must include: { "query": "your task description" }');
    }

    // Create browser session with optional replay recording
    const session = new KernelBrowserSession(kernel, {
      invocationId: ctx.invocation_id,
      stealth: true,
      recordReplay: payload.record_replay ?? false,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    const currentDate = new Date().toLocaleDateString('en-US', {
      weekday: 'long',
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
    const systemPrompt = `You are a helpful assistant operating a Chrome browser on a Kernel cloud VM through computer-use tools.
The browser is already open and ready for use.
When you need to navigate to a page, use the navigate action with a full URL.
After each action, carefully evaluate the screenshot to determine your next step.
The current date is ${currentDate}.`;

    try {
      const agent = new CuaAgent({
        browser: session.browser,
        client: kernel,
        // Set to true to expose a playwright_execute tool for DOM reads, form fills, and selector waits.
        playwright: false,
        initialState: {
          model: 'google:gemini-3-flash-preview',
          systemPrompt,
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
