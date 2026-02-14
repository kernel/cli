import { Kernel, type KernelContext } from '@onkernel/sdk';
import { runAgent, type AgentOptions, type StepInput } from './loop';
import { runLlmAgent, type LlmOptions } from './llm_loop';
import { KernelBrowserSession } from './session';
import { MoondreamClient } from './moondream';

const kernel = new Kernel();

const app = kernel.app('ts-moondream-cua');

interface QueryInput {
  query?: string;
  steps?: StepInput[];
  record_replay?: boolean;
  max_retries?: number;
  retry_delay_ms?: number;
  strict?: boolean;
  max_iterations?: number;
  post_action_wait_ms?: number;
}

interface QueryOutput {
  result: string;
  replay_url?: string;
  error?: string;
}

const MOONDREAM_API_KEY = process.env.MOONDREAM_API_KEY;
const GROQ_API_KEY = process.env.GROQ_API_KEY;

if (!MOONDREAM_API_KEY) {
  throw new Error(
    'MOONDREAM_API_KEY is not set. ' +
    'Set it via environment variable or deploy with: kernel deploy index.ts --env-file .env'
  );
}

app.action<QueryInput, QueryOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: QueryInput): Promise<QueryOutput> => {
    if (!payload?.query && !payload?.steps?.length) {
      throw new Error('Query is required. Payload must include: { "query": "your task description" }');
    }

    const options: AgentOptions = {
      maxRetries: payload.max_retries,
      retryDelayMs: payload.retry_delay_ms,
      strict: payload.strict,
    };
    const llmOptions: LlmOptions = {
      maxIterations: payload.max_iterations,
      postActionWaitMs: payload.post_action_wait_ms,
    };

    const session = new KernelBrowserSession(kernel, {
      stealth: true,
      recordReplay: payload.record_replay ?? false,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    const moondream = new MoondreamClient({ apiKey: MOONDREAM_API_KEY });

    try {
      const result = payload.steps?.length
        ? await runAgent({
            query: payload.query,
            steps: payload.steps,
            moondream,
            kernel,
            sessionId: session.sessionId,
            options,
          })
        : await runLlmAgent({
            query: String(payload.query),
            moondream,
            kernel,
            sessionId: session.sessionId,
            groqApiKey: requireGroqKey(GROQ_API_KEY),
            options: llmOptions,
          });

      const sessionInfo = await session.stop();

      return {
        result: result.finalResponse,
        replay_url: sessionInfo.replayViewUrl,
        error: result.error,
      };
    } catch (error) {
      console.error('Error in agent loop:', error);
      await session.stop();
      throw error;
    }
  },
);

// Run locally if executed directly (not imported as a module)
// Execute via: npx tsx index.ts
if (import.meta.url === `file://${process.argv[1]}`) {
  const testQuery = process.env.TEST_QUERY || 'Navigate to https://example.com and describe the page';

  console.log('Running local test with query:', testQuery);

  const session = new KernelBrowserSession(kernel, {
    stealth: true,
    recordReplay: false,
  });

  session.start().then(async () => {
    const moondream = new MoondreamClient({ apiKey: MOONDREAM_API_KEY });

    try {
      const result = await runLlmAgent({
        query: testQuery,
        moondream,
        kernel,
        sessionId: session.sessionId,
        groqApiKey: requireGroqKey(GROQ_API_KEY),
      });
      console.log('Result:', result.finalResponse);
      if (result.error) {
        console.error('Error:', result.error);
      }
    } finally {
      await session.stop();
    }
    process.exit(0);
  }).catch(error => {
    console.error('Local execution failed:', error);
    process.exit(1);
  });
}

function requireGroqKey(key: string | undefined): string {
  if (!key) {
    throw new Error(
      'GROQ_API_KEY is not set. ' +
      'Set it via environment variable or deploy with: kernel deploy index.ts --env-file .env'
    );
  }
  return key;
}
