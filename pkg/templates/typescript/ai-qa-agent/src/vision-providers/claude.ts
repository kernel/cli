/**
 * Claude (Anthropic) Vision Provider
 */

import Anthropic from "@anthropic-ai/sdk";
import type { VisionProvider } from "../types";

export class ClaudeVisionProvider implements VisionProvider {
  readonly name = "Claude (Anthropic)";
  private client: Anthropic;

  constructor(apiKey: string) {
    this.client = new Anthropic({ apiKey });
  }

  async analyzeScreenshot(screenshot: Buffer, prompt: string): Promise<string> {
    const base64Image = screenshot.toString("base64");

    const response = await this.client.messages.create({
      model: "claude-sonnet-4-5-20250929",
      max_tokens: 4096,
      messages: [
        {
          role: "user",
          content: [
            {
              type: "image",
              source: {
                type: "base64",
                media_type: "image/png",
                data: base64Image,
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

    const textContent = response.content.find((block) => block.type === "text");
    return textContent && textContent.type === "text" ? textContent.text : "";
  }
}
