import { Kernel, type KernelContext } from '@onkernel/sdk';
import { samplingLoop } from './loop';
import { KernelBrowserSession } from './session';

const kernel = new Kernel();

const app = kernel.app('ts-tzafon-cua');

interface QueryInput {
  query: string;
  record_replay?: boolean;
}

interface QueryOutput {
  result: string;
  replay_url?: string;
}

const TZAFON_API_KEY = process.env.TZAFON_API_KEY;

if (!TZAFON_API_KEY) {
  throw new Error('TZAFON_API_KEY is not set');
}

app.action<QueryInput, QueryOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: QueryInput): Promise<QueryOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required');
    }

    const session = new KernelBrowserSession(kernel, {
      invocationId: ctx.invocation_id,
      stealth: true,
      recordReplay: payload.record_replay ?? false,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    try {
      const { finalResult, messages } = await samplingLoop({
        task: payload.query,
        apiKey: TZAFON_API_KEY,
        kernel,
        sessionId: session.sessionId,
        viewportWidth: session.viewportWidth,
        viewportHeight: session.viewportHeight,
      });

      const result = finalResult ?? messages[messages.length - 1] ?? 'Task completed';

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
