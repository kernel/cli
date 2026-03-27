import { Kernel, type KernelContext } from '@onkernel/sdk';
import { samplingLoop } from './loop';
import { KernelBrowserSession } from './session';

interface Input {
  task?: string;
  record_replay?: boolean;
}

interface Output {
  elapsed: number;
  result: string | null;
  replay_url?: string | null;
}

const kernel = new Kernel();
const app = kernel.app('ehr-system');

// LLM API Keys are set in the environment during `kernel deploy <filename> -e ANTHROPIC_API_KEY=XXX`
// See https://www.kernel.sh/docs/launch/deploy#environment-variables
const ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;

if (!ANTHROPIC_API_KEY) {
  throw new Error('ANTHROPIC_API_KEY is not set');
}

const LOGIN_URL = 'https://ehr-system-six.vercel.app/login';

const DEFAULT_TASK = `
Go to ${LOGIN_URL}
Login with username: Phil1 | password: phil | email: heya@invalid.email.com.
Navigate to the "Medical Reports" page.
Find the "Download Summary of Care" button and click it to download the report.
`;

app.action<Input, Output>(
  'export-report',
  async (ctx: KernelContext, payload?: Input): Promise<Output> => {
    const start = Date.now();
    const task = payload?.task || DEFAULT_TASK;

    // Create browser session with optional replay recording
    const session = new KernelBrowserSession(kernel, {
      stealth: true,
      recordReplay: payload?.record_replay ?? false,
    });

    await session.start();
    console.log('> Kernel browser live view url:', session.liveViewUrl);

    try {
      // Run the sampling loop with Anthropic Computer Use
      const finalMessages = await samplingLoop({
        model: 'claude-sonnet-4-5-20250929',
        messages: [{
          role: 'user',
          content: `You are an automated agent. Current date and time: ${new Date().toISOString()}. You must complete the task fully without asking for permission.\n\nTask: ${task}`,
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

      const elapsed = parseFloat(((Date.now() - start) / 1000).toFixed(2));

      // Stop session and get replay URL if recording was enabled
      const sessionInfo = await session.stop();

      return {
        elapsed,
        result,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      const elapsed = parseFloat(((Date.now() - start) / 1000).toFixed(2));
      console.error('Error in export-report:', error);
      await session.stop();
      return {
        elapsed,
        result: null,
      };
    }
  },
);
