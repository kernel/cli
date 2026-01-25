import type { Kernel } from '@onkernel/sdk';
import { runAnthropicLoop, type AnthropicLoopOptions } from './anthropic/loop';
import { runGeminiLoop, type GeminiLoopOptions } from './gemini/loop';
import type { ProviderType } from '../shared/config';
import { PROVIDER_MODELS, DEFAULT_VIEWPORTS } from '../shared/config';

export interface ProviderResult {
  result: string;
  error?: string;
}

export interface RunProviderOptions {
  provider: ProviderType;
  query: string;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  systemPromptSuffix?: string;
  maxIterations?: number;
}

export async function runProvider(options: RunProviderOptions): Promise<ProviderResult> {
  const { provider, query, apiKey, kernel, sessionId, systemPromptSuffix, maxIterations } = options;

  const model = PROVIDER_MODELS[provider];
  const viewport = DEFAULT_VIEWPORTS[provider];

  switch (provider) {
    case 'anthropic': {
      return runAnthropicLoop({
        query,
        apiKey,
        kernel,
        sessionId,
        model,
        systemPromptSuffix,
      });
    }

    case 'gemini': {
      return runGeminiLoop({
        query,
        apiKey,
        kernel,
        sessionId,
        model,
        maxIterations,
        systemPromptSuffix,
        screenSize: viewport,
      });
    }

    default:
      return {
        result: '',
        error: `Unknown provider: ${provider}`,
      };
  }
}

export { type ProviderType } from '../shared/config';
