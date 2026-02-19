import { Kernel, type KernelContext } from '@onkernel/sdk';
import { samplingLoop } from './loop';
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

// LLM API Keys are set in the environment during `kernel deploy <filename> -e ANTHROPIC_API_KEY=XXX`
// See https://www.kernel.sh/docs/launch/deploy#environment-variables
const ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;

if (!ANTHROPIC_API_KEY) {
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
      // Run the sampling loop
      const finalMessages = await samplingLoop({
        model: 'claude-sonnet-4-5-20250929',
        messages: [{
          role: 'user',
          content: payload.query,
        }],
        apiKey: ANTHROPIC_API_KEY,
        thinkingBudget: 1024,
        kernel,
        sessionId: session.sessionId,
      });

      // Extract the final result from the messages
      if (finalMessages.length === 0) {
        throw new Error('No messages were generated during the sampling loop');
      }

      const lastMessage = finalMessages[finalMessages.length - 1];
      if (!lastMessage) {
        throw new Error('Failed to get the last message from the sampling loop');
      }

      const result = typeof lastMessage.content === 'string'
        ? lastMessage.content
        : lastMessage.content.map(block =>
          block.type === 'text' ? block.text : ''
        ).join('');

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
