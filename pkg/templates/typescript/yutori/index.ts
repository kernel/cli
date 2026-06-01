import { Kernel, type KernelContext } from '@onkernel/sdk';
import type OpenAI from 'openai';
import { samplingLoop } from './loop';
import { KernelBrowserSession } from './session';

const kernel = new Kernel();

const app = kernel.app('ts-yutori-cua');

interface QueryInput {
  query: string;
  record_replay?: boolean;
  kiosk?: boolean;
  user_timezone?: string;
  user_location?: string;
}

interface QueryOutput {
  result: string;
  replay_url?: string;
}

// LLM API Keys are set in the environment during `kernel deploy <filename> -e YUTORI_API_KEY=XXX`
// See https://www.kernel.sh/docs/launch/deploy#environment-variables
const YUTORI_API_KEY = process.env.YUTORI_API_KEY;

if (!YUTORI_API_KEY) {
  throw new Error('YUTORI_API_KEY is not set');
}

app.action<QueryInput, QueryOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: QueryInput): Promise<QueryOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required');
    }

    // Create browser session with optional replay recording and kiosk mode
    const kioskMode = payload.kiosk ?? false;
    const session = new KernelBrowserSession(kernel, {
      invocationId: ctx.invocation_id,
      stealth: true,
      recordReplay: payload.record_replay ?? false,
      kioskMode,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    try {
      // Run the sampling loop
      const { finalAnswer, messages } = await samplingLoop({
        model: 'n1.5-latest',
        task: payload.query,
        apiKey: YUTORI_API_KEY,
        kernel,
        sessionId: session.sessionId,
        viewportWidth: session.viewportWidth,
        viewportHeight: session.viewportHeight,
        kioskMode,
        userTimezone: payload.user_timezone,
        userLocation: payload.user_location,
      });

      // Extract the result
      const result = finalAnswer || extractLastAssistantMessage(messages);

      // Stop session and get replay URL if recording was enabled
      const sessionInfo = await session.stop();

      return {
        result,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      console.error('Error in sampling loop:', error);
      await session.stop();
      throw error;
    }
  },
);

function extractLastAssistantMessage(messages: OpenAI.ChatCompletionMessageParam[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    if (msg && msg.role === 'assistant' && typeof msg.content === 'string' && msg.content) {
      return msg.content;
    }
  }
  return 'Task completed';
}
