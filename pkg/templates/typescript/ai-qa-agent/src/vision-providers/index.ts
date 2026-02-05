/**
 * Vision Provider Factory
 *
 * Creates the appropriate vision provider based on the model type.
 */

import type { ModelType, VisionProvider } from "../types";
import { ClaudeVisionProvider } from "./claude";
import { GeminiVisionProvider } from "./gemini";
import { GPT4oVisionProvider } from "./gpt4o";

export { ClaudeVisionProvider } from "./claude";
export { GeminiVisionProvider } from "./gemini";
export { GPT4oVisionProvider } from "./gpt4o";

/**
 * Creates a vision provider instance based on the model type.
 * Reads API keys from environment variables.
 *
 * @throws Error if the required API key is not set
 */
export function createVisionProvider(model: ModelType): VisionProvider {
  switch (model) {
    case "claude": {
      const apiKey = process.env.ANTHROPIC_API_KEY;
      if (!apiKey) {
        throw new Error("ANTHROPIC_API_KEY is required for Claude model");
      }
      return new ClaudeVisionProvider(apiKey);
    }

    case "gpt4o": {
      const apiKey = process.env.OPENAI_API_KEY;
      if (!apiKey) {
        throw new Error("OPENAI_API_KEY is required for GPT-4o model");
      }
      return new GPT4oVisionProvider(apiKey);
    }

    case "gemini": {
      const apiKey = process.env.GOOGLE_API_KEY;
      if (!apiKey) {
        throw new Error("GOOGLE_API_KEY is required for Gemini model");
      }
      return new GeminiVisionProvider(apiKey);
    }

    default:
      throw new Error(`Unknown model: ${model}`);
  }
}
