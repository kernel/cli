/**
 * Gemini (Google) Vision Provider
 */

import { GoogleGenerativeAI } from "@google/generative-ai";
import type { VisionProvider } from "../types";

export class GeminiVisionProvider implements VisionProvider {
  readonly name = "Gemini (Google)";
  private client: GoogleGenerativeAI;

  constructor(apiKey: string) {
    this.client = new GoogleGenerativeAI(apiKey);
  }

  async analyzeScreenshot(screenshot: Buffer, prompt: string): Promise<string> {
    const model = this.client.getGenerativeModel({ model: "gemini-2.0-flash-exp" });

    const result = await model.generateContent([
      {
        inlineData: {
          mimeType: "image/png",
          data: screenshot.toString("base64"),
        },
      },
      prompt,
    ]);

    return result.response.text();
  }
}
