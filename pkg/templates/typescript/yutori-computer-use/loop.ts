/**
 * Yutori n1 Sampling Loop
 * 
 * Implements the agent loop for Yutori's n1 computer use model.
 * n1 uses an OpenAI-compatible API with specific conventions:
 * - Screenshots use role: "observation" (not "user")
 * - Coordinates are returned in 1000x1000 space and need scaling
 * - WebP format recommended for screenshots
 * 
 * Supports two modes:
 * - computer_use: Uses Kernel's Computer Controls API (full VM screenshots)
 * - playwright: Uses Playwright via CDP (viewport-only screenshots, optimized for n1)
 * 
 * @see https://docs.yutori.com/reference/n1
 */

import OpenAI from 'openai';
import type { Kernel } from '@onkernel/sdk';
import { ComputerTool, type N1Action, type ToolResult } from './tools/computer';
import { PlaywrightComputerTool } from './tools/playwright-computer';

/** Mode for browser interaction */
export type BrowserMode = 'computer_use' | 'playwright';

interface N1ComputerTool {
  execute(action: N1Action): Promise<ToolResult>;
  screenshot(): Promise<ToolResult>;
}

// n1 uses its own system prompt - custom prompts may degrade performance
// Per docs: "we generally do not recommend providing custom system prompts"

interface Message {
  role: 'user' | 'assistant' | 'observation';
  content: string | MessageContent[];
}

interface MessageContent {
  type: 'text' | 'image_url';
  text?: string;
  image_url?: {
    url: string;
  };
}

interface SamplingLoopOptions {
  model?: string;
  task: string;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  /** CDP WebSocket URL for playwright mode */
  cdpWsUrl?: string;
  maxTokens?: number;
  maxIterations?: number;
  /** Viewport width for coordinate scaling (default: 1200, closest to Yutori's 1280 recommendation) */
  viewportWidth?: number;
  /** Viewport height for coordinate scaling (default: 800 per Yutori recommendation) */
  viewportHeight?: number;
  /**
   * Browser interaction mode:
   * - computer_use: Uses Kernel's Computer Controls API (full VM screenshots)
   * - playwright: Uses Playwright via CDP (viewport-only screenshots, optimized for n1)
   * @default 'computer_use'
   */
  mode?: BrowserMode;
}

interface SamplingLoopResult {
  messages: Message[];
  finalAnswer?: string;
}

export async function samplingLoop({
  model = 'n1-preview-2025-11',
  task,
  apiKey,
  kernel,
  sessionId,
  cdpWsUrl,
  maxTokens = 4096,
  maxIterations = 50,
  viewportWidth = 1280,
  viewportHeight = 800,
  mode = 'computer_use',
}: SamplingLoopOptions): Promise<SamplingLoopResult> {
  const client = new OpenAI({
    apiKey,
    baseURL: 'https://api.yutori.com/v1',
  });

  let computerTool: N1ComputerTool;
  let playwrightTool: PlaywrightComputerTool | null = null;

  console.log(`Mode requested: '${mode}', cdpWsUrl available: ${cdpWsUrl != null}`);

  if (mode === 'playwright') {
    if (!cdpWsUrl) {
      throw new Error('cdpWsUrl is required for playwright mode');
    }
    console.log(`Connecting to CDP WebSocket: ${cdpWsUrl.substring(0, 50)}...`);
    playwrightTool = new PlaywrightComputerTool(cdpWsUrl, viewportWidth, viewportHeight);
    await playwrightTool.connect();
    computerTool = playwrightTool;
    console.log('Using playwright mode (viewport-only screenshots)');
  } else {
    computerTool = new ComputerTool(kernel, sessionId, viewportWidth, viewportHeight);
    console.log('Using computer_use mode (Computer Controls API)');
  }

  try {
    const initialScreenshot = await computerTool.screenshot();

    const conversationMessages: Message[] = [
      {
        role: 'user',
        content: [{ type: 'text', text: task }],
      },
    ];

    if (initialScreenshot.base64Image) {
      conversationMessages.push({
        role: 'observation',
        content: [
          {
            type: 'image_url',
            image_url: {
              url: `data:image/png;base64,${initialScreenshot.base64Image}`,
            },
          },
        ],
      });
    }

    let iteration = 0;
    let finalAnswer: string | undefined;

    while (iteration < maxIterations) {
      iteration++;
      console.log(`\n=== Iteration ${iteration} ===`);

      let response;
      try {
        response = await client.chat.completions.create({
          model,
          messages: conversationMessages as OpenAI.ChatCompletionMessageParam[],
          max_tokens: maxTokens,
          temperature: 0.3,
        });
      } catch (apiError) {
        console.error('API call failed:', apiError);
        throw apiError;
      }

      if (!response.choices || response.choices.length === 0) {
        console.error('No choices in response:', JSON.stringify(response, null, 2));
        throw new Error('No choices in API response');
      }

      const assistantMessage = response.choices[0]?.message;
      if (!assistantMessage) {
        throw new Error('No response from model');
      }

      const responseContent = assistantMessage.content || '';
      console.log('Assistant response:', responseContent);

      conversationMessages.push({
        role: 'assistant',
        content: responseContent,
      });

      const parsed = parseN1Response(responseContent);

      if (!parsed || !parsed.actions || parsed.actions.length === 0) {
        console.log('No actions found in response, ending loop');
        break;
      }

      for (const action of parsed.actions) {
        console.log('Executing action:', action.action_type, action);

        if (action.action_type === 'stop') {
          finalAnswer = action.answer;
          console.log('Stop action received, final answer:', finalAnswer);
          return { messages: conversationMessages, finalAnswer };
        }

        const scaledAction = scaleCoordinates(action, viewportWidth, viewportHeight);

        let result: ToolResult;
        try {
          result = await computerTool.execute(scaledAction);
        } catch (error) {
          console.error('Action failed:', error);
          result = {
            error: error instanceof Error ? error.message : String(error),
          };
        }

        if (result.base64Image || result.output) {
          const observationContent: MessageContent[] = [];

          if (result.output) {
            observationContent.push({
              type: 'text',
              text: result.output,
            });
          }

          if (result.base64Image) {
            observationContent.push({
              type: 'image_url',
              image_url: {
                url: `data:image/png;base64,${result.base64Image}`,
              },
            });
          }

          conversationMessages.push({
            role: 'observation',
            content: observationContent,
          });
        } else if (result.error) {
          conversationMessages.push({
            role: 'observation',
            content: [{ type: 'text', text: `Action failed: ${result.error}` }],
          });
        }
      }
    }

    if (iteration >= maxIterations) {
      console.log('Max iterations reached');
    }

    return {
      messages: conversationMessages,
      finalAnswer,
    };
  } finally {
    if (playwrightTool) {
      await playwrightTool.disconnect();
    }
  }
}

function parseN1Response(content: string): { thoughts?: string; actions?: N1Action[] } | null {
  try {
    // The response should be JSON
    const parsed = JSON.parse(content);
    return parsed;
  } catch {
    // Try to extract JSON from the response if it's wrapped in text
    const jsonMatch = content.match(/\{[\s\S]*\}/);
    if (jsonMatch) {
      try {
        return JSON.parse(jsonMatch[0]);
      } catch {
        console.error('Failed to parse action JSON:', jsonMatch[0]);
      }
    }
    return null;
  }
}

function scaleCoordinates(action: N1Action, viewportWidth: number, viewportHeight: number): N1Action {
  const scaled = { ...action };

  if (scaled.center_coordinates) {
    scaled.center_coordinates = [
      Math.round((scaled.center_coordinates[0] / 1000) * viewportWidth),
      Math.round((scaled.center_coordinates[1] / 1000) * viewportHeight),
    ];
  }

  if (scaled.start_coordinates) {
    scaled.start_coordinates = [
      Math.round((scaled.start_coordinates[0] / 1000) * viewportWidth),
      Math.round((scaled.start_coordinates[1] / 1000) * viewportHeight),
    ];
  }

  return scaled;
}
