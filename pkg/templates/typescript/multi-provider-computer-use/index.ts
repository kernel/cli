import { Kernel, type KernelContext } from '@onkernel/sdk';
import { KernelBrowserSession } from './shared/session';
import { type ProviderType, DEFAULT_VIEWPORTS, validateApiKey } from './shared/config';
import { runProvider } from './providers';

const kernel = new Kernel();
const app = kernel.app('ts-multi-provider-cua');

interface QueryInput {
  query: string;
  provider: ProviderType;
  record_replay?: boolean;
}

interface QueryOutput {
  result: string;
  replay_url?: string;
  error?: string;
}

app.action<QueryInput, QueryOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: QueryInput): Promise<QueryOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required');
    }
    if (!payload?.provider) {
      throw new Error('Provider is required (anthropic or gemini)');
    }
    if (payload.provider !== 'anthropic' && payload.provider !== 'gemini') {
      throw new Error(`Unknown provider: ${payload.provider}. Must be 'anthropic' or 'gemini'`);
    }

    const apiKey = validateApiKey(payload.provider);
    const viewport = DEFAULT_VIEWPORTS[payload.provider];

    const session = new KernelBrowserSession(kernel, {
      stealth: true,
      recordReplay: payload.record_replay ?? false,
      viewport,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    try {
      const result = await runProvider({
        provider: payload.provider,
        query: payload.query,
        apiKey,
        kernel,
        sessionId: session.sessionId,
      });

      const sessionInfo = await session.stop();

      return {
        result: result.result,
        replay_url: sessionInfo.replayViewUrl,
        error: result.error,
      };
    } catch (error) {
      await session.stop();
      throw error;
    }
  },
);
