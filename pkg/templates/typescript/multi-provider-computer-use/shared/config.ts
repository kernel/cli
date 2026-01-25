export type ProviderType = 'anthropic' | 'gemini';

export interface ProviderConfig {
  provider: ProviderType;
  apiKey?: string;
  maxIterations?: number;
  systemPromptSuffix?: string;
}

export const PROVIDER_MODELS: Record<ProviderType, string> = {
  anthropic: 'claude-sonnet-4-5-20250929',
  gemini: 'gemini-2.5-computer-use-preview-10-2025',
};

export const DEFAULT_VIEWPORTS: Record<ProviderType, { width: number; height: number }> = {
  anthropic: { width: 1024, height: 768 },
  gemini: { width: 1200, height: 800 },
};

export const API_KEY_ENV_VARS: Record<ProviderType, string> = {
  anthropic: 'ANTHROPIC_API_KEY',
  gemini: 'GOOGLE_API_KEY',
};

export function validateApiKey(provider: ProviderType): string {
  const envVar = API_KEY_ENV_VARS[provider];
  const apiKey = process.env[envVar];

  if (!apiKey) {
    throw new Error(`${envVar} environment variable is required for the ${provider} provider`);
  }

  return apiKey;
}
