/**
 * Provider factory with automatic fallback.
 *
 * Resolution order:
 *   1. CUA_PROVIDER env var (required)
 *   2. CUA_FALLBACK_PROVIDERS env var (optional, comma-separated)
 *
 * A provider is "available" when its API key env var is set.
 */

import type { Kernel } from '@onkernel/sdk';
import { AnthropicProvider } from './anthropic';
import { OpenAIProvider } from './openai';
import { GeminiProvider } from './gemini';
import { TzafonProvider } from './tzafon';
import { YutoriProvider } from './yutori';

// Shared interface every provider adapter must implement.
export interface TaskOptions {
  query: string;
  model?: string;
  kernel: Kernel;
  sessionId: string;
  viewportWidth?: number;
  viewportHeight?: number;
}

export interface TaskResult {
  result: string;
  provider: string;
}

export interface CuaProvider {
  readonly name: string;
  isConfigured(): boolean;
  runTask(options: TaskOptions): Promise<TaskResult>;
}

export type ProviderName = 'anthropic' | 'openai' | 'gemini' | 'tzafon' | 'yutori';

const PROVIDERS: Record<ProviderName, () => CuaProvider> = {
  anthropic: () => new AnthropicProvider(),
  openai: () => new OpenAIProvider(),
  gemini: () => new GeminiProvider(),
  tzafon: () => new TzafonProvider(),
  yutori: () => new YutoriProvider(),
};

/**
 * Build the ordered list of providers to try.
 * Throws if no configured provider is found.
 */
export function resolveProviders(): CuaProvider[] {
  const primaryName = (process.env.CUA_PROVIDER ?? '').trim().toLowerCase();
  const fallbackNames = (process.env.CUA_FALLBACK_PROVIDERS ?? '')
    .split(',')
    .map(s => s.trim().toLowerCase())
    .filter(Boolean);

  const order = primaryName ? [primaryName, ...fallbackNames] : fallbackNames;

  // Deduplicate while preserving order
  const seen = new Set<string>();
  const providers: CuaProvider[] = [];

  for (const name of order) {
    if (seen.has(name)) continue;
    seen.add(name);

    const factory = PROVIDERS[name];
    if (!factory) {
      console.warn(`Unknown provider "${name}", skipping.`);
      continue;
    }

    const provider = factory();
    if (provider.isConfigured()) {
      providers.push(provider);
    } else {
      console.warn(`Provider "${name}" is not configured (missing API key), skipping.`);
    }
  }

  if (providers.length === 0) {
    const available = Object.keys(PROVIDERS).join(', ');
    throw new Error(
      'No CUA provider is configured. ' +
      `Set CUA_PROVIDER to one of: ${available}, and provide the matching API key.`,
    );
  }

  return providers;
}

/**
 * Run a CUA task, trying each provider in order until one succeeds.
 */
export async function runWithFallback(
  providers: CuaProvider[],
  options: TaskOptions,
): Promise<TaskResult> {
  const errors: Array<{ provider: string; error: unknown }> = [];

  for (const provider of providers) {
    try {
      console.log(`Attempting provider: ${provider.name}`);
      return await provider.runTask(options);
    } catch (error) {
      console.error(`Provider "${provider.name}" failed:`, error);
      errors.push({ provider: provider.name, error });
    }
  }

  const summary = errors
    .map(e => `  ${e.provider}: ${e.error instanceof Error ? e.error.message : String(e.error)}`)
    .join('\n');
  throw new Error(`All providers failed:\n${summary}`);
}
