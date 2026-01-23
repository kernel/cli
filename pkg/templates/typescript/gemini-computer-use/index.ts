import { Kernel, type KernelContext } from '@onkernel/sdk';
import { samplingLoop } from './loop';
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

// API Key for Gemini
// - GOOGLE_API_KEY: Required for Gemini Computer Use model
// Set via environment variables or `kernel deploy <filename> --env-file .env`
// See https://www.kernel.sh/docs/launch/deploy#environment-variables
const GOOGLE_API_KEY = process.env.GOOGLE_API_KEY;

if (!GOOGLE_API_KEY) {
  throw new Error('GOOGLE_API_KEY is not set');
}

app.action<QueryInput, QueryOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: QueryInput): Promise<QueryOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required');
    }

    // Create browser session with optional replay recording
    const session = new KernelBrowserSession(kernel, {
      stealth: true,
      recordReplay: payload.record_replay ?? false,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    try {
      // Run the Gemini sampling loop
      const result = await samplingLoop({
        model: 'gemini-2.5-computer-use-preview-10-2025',
        query: payload.query,
        apiKey: GOOGLE_API_KEY,
        kernel,
        sessionId: session.sessionId,
      });

      // Stop session and get replay URL if recording was enabled
      const sessionInfo = await session.stop();

      return {
        result: result.finalResponse,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      console.error('Error in sampling loop:', error);
      await session.stop();
      throw error;
    }
  },
);

// Run locally if executed directly (not imported as a module)
// Execute via: npx tsx index.ts
if (import.meta.url === `file://${process.argv[1]}`) {
  const testQuery = "Navigate to https://www.google.com and describe what you see";
  
  console.log('Running local test with query:', testQuery);
  
  const session = new KernelBrowserSession(kernel, {
    stealth: true,
    recordReplay: false,
  });

  session.start().then(async () => {
    try {
      const result = await samplingLoop({
        model: 'gemini-2.5-computer-use-preview-10-2025',
        query: testQuery,
        apiKey: GOOGLE_API_KEY,
        kernel,
        sessionId: session.sessionId,
      });
      console.log('Result:', result.finalResponse);
    } finally {
      await session.stop();
    }
    process.exit(0);
  }).catch(error => {
    console.error('Local execution failed:', error);
    process.exit(1);
  });
}
