/**
 * Provider factory and fallback logic.
 *
 * Creates provider instances and handles automatic fallback on provider errors
 * (rate limits, API errors). Does NOT fall back on task-level failures.
 */

import type { KernelExecutor } from '../tools';
import { AnthropicProvider } from './anthropic';
import { OpenAIProvider } from './openai';
import { GeminiProvider } from './gemini';

export type ProviderName = 'anthropic' | 'openai' | 'gemini';

export interface ProviderConfig {
  query: string;
  model?: string;
  apiKey?: string;
  viewportWidth?: number;
  viewportHeight?: number;
}

export interface ProviderResult {
  result: string;
  provider: ProviderName;
}

export interface CUAProvider {
  name: ProviderName;
  run(config: ProviderConfig, executor: KernelExecutor): Promise<ProviderResult>;
}

const PROVIDER_MAP: Record<ProviderName, () => CUAProvider> = {
  anthropic: () => new AnthropicProvider(),
  openai: () => new OpenAIProvider(),
  gemini: () => new GeminiProvider(),
};

export function createProvider(name: ProviderName): CUAProvider {
  const factory = PROVIDER_MAP[name];
  if (!factory) {
    throw new Error(`Unknown provider: ${name}. Supported: ${Object.keys(PROVIDER_MAP).join(', ')}`);
  }
  return factory();
}

// Errors that indicate a provider-level failure (should trigger fallback)
function isProviderError(error: unknown): boolean {
  if (!(error instanceof Error)) return true;
  const msg = error.message.toLowerCase();
  return (
    msg.includes('rate limit') ||
    msg.includes('429') ||
    msg.includes('503') ||
    msg.includes('502') ||
    msg.includes('500') ||
    msg.includes('overloaded') ||
    msg.includes('capacity') ||
    msg.includes('api key') ||
    msg.includes('authentication') ||
    msg.includes('unauthorized') ||
    msg.includes('forbidden') ||
    msg.includes('quota') ||
    msg.includes('timeout') ||
    msg.includes('econnrefused') ||
    msg.includes('econnreset') ||
    msg.includes('fetch failed')
  );
}

/**
 * Run a CUA task with automatic fallback across providers.
 *
 * Tries the primary provider first. On provider-level errors (rate limits, API errors),
 * falls back to the next provider in the list. Does NOT fall back on task-level failures
 * (the model completed but gave a wrong answer).
 */
export async function runWithFallback(
  providers: ProviderName[],
  config: ProviderConfig,
  executor: KernelExecutor,
): Promise<ProviderResult> {
  const errors: Array<{ provider: ProviderName; error: Error }> = [];

  for (const providerName of providers) {
    const provider = createProvider(providerName);
    try {
      console.log(`[cua] Trying provider: ${providerName}`);
      const result = await provider.run(config, executor);
      console.log(`[cua] Provider ${providerName} succeeded`);
      return result;
    } catch (error) {
      const err = error instanceof Error ? error : new Error(String(error));
      errors.push({ provider: providerName, error: err });
      console.error(`[cua] Provider ${providerName} failed: ${err.message}`);

      if (!isProviderError(error)) {
        // Task-level error, don't fall back
        throw error;
      }

      if (providers.indexOf(providerName) === providers.length - 1) {
        // Last provider, no more fallbacks
        throw new Error(
          `All providers failed:\n${errors.map(e => `  ${e.provider}: ${e.error.message}`).join('\n')}`
        );
      }

      console.log(`[cua] Falling back to next provider...`);
    }
  }

  // Should never reach here
  throw new Error('No providers configured');
}
