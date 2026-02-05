/**
 * GPT-4o (OpenAI) Vision Provider
 */

import OpenAI from "openai";
import type { VisionProvider } from "../types";

export class GPT4oVisionProvider implements VisionProvider {
  readonly name = "GPT-4o (OpenAI)";
  private client: OpenAI;

  constructor(apiKey: string) {
    this.client = new OpenAI({ apiKey });
  }

  async analyzeScreenshot(screenshot: Buffer, prompt: string): Promise<string> {
    const base64Image = screenshot.toString("base64");

    const response = await this.client.chat.completions.create({
      model: "gpt-4o",
      max_tokens: 2048,
      messages: [
        {
          role: "user",
          content: [
            {
              type: "image_url",
              image_url: {
                url: `data:image/png;base64,${base64Image}`,
              },
            },
            {
              type: "text",
              text: prompt,
            },
          ],
        },
      ],
    });

    return response.choices[0]?.message?.content || "";
  }
}
