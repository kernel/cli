/**
 * Yutori n1.5 Sampling Loop
 *
 * Implements the agent loop for Yutori's n1.5-latest computer use model.
 * n1.5-latest uses an OpenAI-compatible API with tool_calls:
 * - Actions are returned via tool_calls in the assistant message
 * - Tool results use role: "tool" with matching tool_call_id
 * - The model stops by returning content without tool_calls
 * - Coordinates are returned in 1000x1000 space and need scaling
 *
 * @see https://docs.yutori.com/reference/n1-5
 */

import OpenAI from 'openai';
import type { Kernel } from '@onkernel/sdk';
import { ComputerTool, type N15Action, type ToolResult } from './tools/computer';

// Tools that require a Playwright page / DOM access. The default core tool set
// already excludes them, but we also list them in `disable_tools` so the
// exclusion is explicit and survives if the default ever changes.
const DISABLED_TOOLS = ['extract_elements', 'find', 'set_element_value', 'execute_js'];
const TOOL_SET = 'browser_tools_core-20260403';

interface SamplingLoopOptions {
  model?: string;
  task: string;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  maxCompletionTokens?: number;
  maxIterations?: number;
  viewportWidth?: number;
  viewportHeight?: number;
  kioskMode?: boolean;
}

interface SamplingLoopResult {
  messages: OpenAI.ChatCompletionMessageParam[];
  finalAnswer?: string;
}

export async function samplingLoop({
  model = 'n1.5-latest',
  task,
  apiKey,
  kernel,
  sessionId,
  maxCompletionTokens = 4096,
  maxIterations = 50,
  viewportWidth = 1280,
  viewportHeight = 800,
  kioskMode = false,
}: SamplingLoopOptions): Promise<SamplingLoopResult> {
  const client = new OpenAI({
    apiKey,
    baseURL: 'https://api.yutori.com/v1',
  });

  const computerTool = new ComputerTool(kernel, sessionId, viewportWidth, viewportHeight, kioskMode);

  const initialScreenshot = await computerTool.screenshot();

  const conversationMessages: OpenAI.ChatCompletionMessageParam[] = [
    {
      role: 'user',
      content: [
        { type: 'text', text: task },
        ...(initialScreenshot.base64Image
          ? [{
              type: 'image_url' as const,
              image_url: {
                url: `data:image/webp;base64,${initialScreenshot.base64Image}`,
              },
            }]
          : []),
      ],
    },
  ];

  let iteration = 0;
  let finalAnswer: string | undefined;

  while (iteration < maxIterations) {
    iteration++;
    console.log(`\n=== Iteration ${iteration} ===`);

    let response;
    try {
      response = await client.chat.completions.create({
        model,
        messages: conversationMessages,
        max_completion_tokens: maxCompletionTokens,
        temperature: 0.3,
        // n1.5-specific knobs go in extra_body (not yet in OpenAI SDK types).
        // tool_set selects the core (coordinate-based) tools.
        // disable_tools is a defense-in-depth exclusion of DOM/Playwright tools.
        // @ts-expect-error extra_body is a Yutori extension
        extra_body: {
          tool_set: TOOL_SET,
          disable_tools: DISABLED_TOOLS,
        },
      });
    } catch (apiError) {
      console.error('API call failed:', apiError);
      throw apiError;
    }

    if (!response.choices || response.choices.length === 0) {
      console.error('No choices in response:', JSON.stringify(response, null, 2));
      throw new Error('No choices in API response');
    }

    const choice = response.choices[0];
    const assistantMessage = choice.message;
    if (!assistantMessage) {
      throw new Error('No response from model');
    }

    console.log('Assistant content:', assistantMessage.content || '(none)');

    // Preserve full assistant message (including tool_calls) in history
    conversationMessages.push(assistantMessage);

    const toolCalls = assistantMessage.tool_calls;

    // No tool_calls means the model is done
    if (!toolCalls || toolCalls.length === 0) {
      finalAnswer = assistantMessage.content || undefined;
      console.log('No tool_calls, model is done. Final answer:', finalAnswer);
      break;
    }

    for (const toolCall of toolCalls) {
      const actionName = toolCall.function.name;
      let args: Record<string, unknown>;
      try {
        args = JSON.parse(toolCall.function.arguments);
      } catch {
        console.error('Failed to parse tool_call arguments:', toolCall.function.arguments);
        conversationMessages.push({
          role: 'tool',
          tool_call_id: toolCall.id,
          content: 'Error: failed to parse arguments',
        });
        continue;
      }

      const action: N15Action = {
        action_type: actionName as N15Action['action_type'],
        ...args,
      };

      console.log('Executing action:', actionName, args);

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

      if (result.base64Image) {
        conversationMessages.push({
          role: 'tool',
          tool_call_id: toolCall.id,
          // Yutori n1 accepts image content arrays in tool messages (not yet in OpenAI SDK types)
          content: [
            {
              type: 'image_url',
              image_url: {
                url: `data:image/webp;base64,${result.base64Image}`,
              },
            },
          ] as unknown as string,
        });
      } else if (result.error) {
        conversationMessages.push({
          role: 'tool',
          tool_call_id: toolCall.id,
          content: `Action failed: ${result.error}`,
        });
      } else {
        conversationMessages.push({
          role: 'tool',
          tool_call_id: toolCall.id,
          content: result.output || 'OK',
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

function scaleCoordinates(action: N15Action, viewportWidth: number, viewportHeight: number): N15Action {
  const scaled = { ...action };

  if (scaled.coordinates) {
    scaled.coordinates = [
      Math.round((scaled.coordinates[0] / 1000) * viewportWidth),
      Math.round((scaled.coordinates[1] / 1000) * viewportHeight),
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
