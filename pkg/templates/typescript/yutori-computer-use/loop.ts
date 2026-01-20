/**
 * Yutori n1 Sampling Loop
 * 
 * Implements the agent loop for Yutori's n1 computer use model.
 * n1 uses an OpenAI-compatible API with specific conventions:
 * - Screenshots use role: "observation" (not "user")
 * - Coordinates are returned in 1000x1000 space and need scaling
 * - WebP format recommended for screenshots
 * 
 * @see https://docs.yutori.com/reference/n1
 */

import OpenAI from 'openai';
import type { Kernel } from '@onkernel/sdk';
import { ComputerTool, type N1Action, type ToolResult } from './tools/computer';

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
  maxTokens?: number;
  maxIterations?: number;
  /** Viewport width for coordinate scaling (default: 1200 to match WXGA) */
  viewportWidth?: number;
  /** Viewport height for coordinate scaling (default: 800 to match WXGA) */
  viewportHeight?: number;
}

interface SamplingLoopResult {
  messages: Message[];
  finalAnswer?: string;
}

/**
 * Run the n1 sampling loop until the model returns a stop action or max iterations.
 */
export async function samplingLoop({
  model = 'n1-preview-2025-11',
  task,
  apiKey,
  kernel,
  sessionId,
  maxTokens = 4096,
  maxIterations = 50,
  // Default viewport matches WXGA (1200x800) - closest to Yutori's recommended 1280x800
  viewportWidth = 1200,
  viewportHeight = 800,
}: SamplingLoopOptions): Promise<SamplingLoopResult> {
  const client = new OpenAI({
    apiKey,
    baseURL: 'https://api.yutori.com/v1',
  });

  const computerTool = new ComputerTool(kernel, sessionId, viewportWidth, viewportHeight);

  // Take initial screenshot
  const initialScreenshot = await computerTool.screenshot();

  // Build conversation per n1 format:
  // 1. User message with task
  // 2. Observation message with screenshot
  const conversationMessages: Message[] = [
    {
      role: 'user',
      content: [{ type: 'text', text: task }],
    },
  ];

  // Add initial screenshot as observation (n1's required format)
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

    // Call the n1 API (no system prompt - n1 uses its own)
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

    // Add assistant message to conversation
    conversationMessages.push({
      role: 'assistant',
      content: responseContent,
    });

    // Parse the action(s) from the response
    // n1 returns JSON with "thoughts" and "actions" array
    const parsed = parseN1Response(responseContent);

    if (!parsed || !parsed.actions || parsed.actions.length === 0) {
      console.log('No actions found in response, ending loop');
      break;
    }

    // Execute each action in the actions array
    for (const action of parsed.actions) {
      console.log('Executing action:', action.action_type, action);

      // Check for stop action
      if (action.action_type === 'stop') {
        finalAnswer = action.answer;
        console.log('Stop action received, final answer:', finalAnswer);
        return { messages: conversationMessages, finalAnswer };
      }

      // Scale coordinates from n1's 1000x1000 space to actual viewport
      const scaledAction = scaleCoordinates(action, viewportWidth, viewportHeight);

      // Execute the action
      let result: ToolResult;
      try {
        result = await computerTool.execute(scaledAction);
      } catch (error) {
        console.error('Action failed:', error);
        result = {
          error: error instanceof Error ? error.message : String(error),
        };
      }

      // After action, add observation with screenshot and optional text output
      if (result.base64Image || result.output) {
        const observationContent: MessageContent[] = [];

        // Add text output first (e.g., from read_texts_and_links)
        if (result.output) {
          observationContent.push({
            type: 'text',
            text: result.output,
          });
        }

        // Add screenshot
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
        // If there was an error, add it as text observation
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
}

/**
 * Parse n1's response format: { "thoughts": "...", "actions": [...] }
 */
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

/**
 * Scale coordinates from n1's 1000x1000 space to actual viewport dimensions.
 * Per docs: "n1-preview-2025-11 outputs relative coordinates in 1000×1000"
 */
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
