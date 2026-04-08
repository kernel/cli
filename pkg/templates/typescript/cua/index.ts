/**
 * Unified CUA (Computer Use Agent) template with multi-provider support.
 *
 * Supports Anthropic, OpenAI, and Gemini as interchangeable providers.
 * Configure via environment variables:
 *   CUA_PROVIDER         — primary provider ("anthropic", "openai", or "gemini")
 *   CUA_FALLBACK_PROVIDERS — comma-separated fallback order (optional)
 *
 * Each provider requires its own API key:
 *   ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY
 */

import { Kernel, type KernelContext } from '@onkernel/sdk';
import { KernelBrowserSession } from './session';
import { resolveProviders, runWithFallback } from './providers/index';

const kernel = new Kernel();
const app = kernel.app('ts-cua');

interface CuaInput {
  query: string;
  record_replay?: boolean;
}

interface CuaOutput {
  result: string;
  provider: string;
  replay_url?: string;
}

// Resolve providers at startup so misconfiguration fails fast.
const providers = resolveProviders();
console.log(`Configured providers: ${providers.map(p => p.name).join(' -> ')}`);

app.action<CuaInput, CuaOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: CuaInput): Promise<CuaOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required. Payload must include: { "query": "your task description" }');
    }

    const session = new KernelBrowserSession(kernel, {
      invocationId: ctx.invocation_id,
      stealth: true,
      recordReplay: payload.record_replay ?? false,
    });

    await session.start();
    console.log('Live view:', session.liveViewUrl);

    try {
      const { result, provider } = await runWithFallback(providers, {
        query: payload.query,
        kernel,
        sessionId: session.sessionId,
        viewportWidth: session.viewportWidth,
        viewportHeight: session.viewportHeight,
      });

      const sessionInfo = await session.stop();

      return {
        result,
        provider,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      console.error('CUA task failed:', error);
      await session.stop();
      throw error;
    }
  },
);
