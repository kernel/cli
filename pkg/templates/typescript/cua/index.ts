/**
 * Unified CUA (Computer Use Agent) template.
 *
 * Supports Anthropic, OpenAI, and Gemini providers with automatic fallback.
 * Configure via environment variables:
 *   CUA_PROVIDER       - Primary provider: anthropic, openai, or gemini (default: anthropic)
 *   CUA_FALLBACK_PROVIDERS - Comma-separated fallback order (e.g. "openai,gemini")
 *   ANTHROPIC_API_KEY   - Required if using Anthropic
 *   OPENAI_API_KEY      - Required if using OpenAI
 *   GOOGLE_API_KEY      - Required if using Gemini
 */

import { Kernel, type KernelContext } from '@onkernel/sdk';
import { KernelBrowserSession } from './session';
import { KernelExecutor } from './tools';
import { runWithFallback, type ProviderName } from './providers/index';

const kernel = new Kernel();
const app = kernel.app('ts-cua');

// Parse provider configuration from environment
const PRIMARY_PROVIDER = (process.env.CUA_PROVIDER || 'anthropic') as ProviderName;
const FALLBACK_PROVIDERS = process.env.CUA_FALLBACK_PROVIDERS
  ? process.env.CUA_FALLBACK_PROVIDERS.split(',').map(p => p.trim()) as ProviderName[]
  : [];

const PROVIDER_CHAIN: ProviderName[] = [PRIMARY_PROVIDER, ...FALLBACK_PROVIDERS];

// Validate that at least one provider has an API key
const API_KEY_MAP: Record<ProviderName, string> = {
  anthropic: 'ANTHROPIC_API_KEY',
  openai: 'OPENAI_API_KEY',
  gemini: 'GOOGLE_API_KEY',
};

const configuredProviders = PROVIDER_CHAIN.filter(p => process.env[API_KEY_MAP[p]]);
if (configuredProviders.length === 0) {
  throw new Error(
    `No API keys found for configured providers [${PROVIDER_CHAIN.join(', ')}]. ` +
    `Set at least one of: ${PROVIDER_CHAIN.map(p => API_KEY_MAP[p]).join(', ')}`
  );
}

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

app.action<CuaInput, CuaOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: CuaInput): Promise<CuaOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required');
    }

    // Allow per-request provider override
    const providerChain = payload.provider
      ? [payload.provider, ...PROVIDER_CHAIN.filter(p => p !== payload.provider)]
      : PROVIDER_CHAIN;

    const session = new KernelBrowserSession(kernel, {
      invocationId: ctx.invocation_id,
      stealth: true,
      recordReplay: payload.record_replay ?? false,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    try {
      const executor = new KernelExecutor(kernel, session.sessionId);

      const result = await runWithFallback(providerChain, {
        query: payload.query,
        model: payload.model,
        viewportWidth: session.viewportWidth,
        viewportHeight: session.viewportHeight,
      }, executor);

      const sessionInfo = await session.stop();

      return {
        result: result.result,
        provider: result.provider,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      console.error('CUA task failed:', error);
      await session.stop();
      throw error;
    }
  },
);
