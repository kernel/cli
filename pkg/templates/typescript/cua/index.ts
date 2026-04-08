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
import { resolveProviders, runWithFallback, type ProviderName } from './providers/index';

const kernel = new Kernel();
const app = kernel.app('ts-cua');

interface CuaInput {
  query: string;
  provider?: ProviderName;
  model?: string;
  record_replay?: boolean;
}

interface CuaOutput {
  result: string;
  provider: string;
  replay_url?: string;
}

// Provider resolution is deferred to the action handler because env vars
// are not available during Hypeman's build/discovery phase.
let _providers: ReturnType<typeof resolveProviders> | null = null;
function getProviders() {
  if (!_providers) {
    _providers = resolveProviders();
    console.log(`Configured providers: ${_providers.map(p => p.name).join(' -> ')}`);
  }
  return _providers;
}

app.action<CuaInput, CuaOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: CuaInput): Promise<CuaOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required. Payload must include: { "query": "your task description" }');
    }

    let providers = getProviders();

    // Per-request provider override: move requested provider to front
    if (payload.provider) {
      const requested = providers.find(p => p.name === payload.provider);
      if (requested) {
        providers = [requested, ...providers.filter(p => p !== requested)];
      }
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
        model: payload.model,
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
