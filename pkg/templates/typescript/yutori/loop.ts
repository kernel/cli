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

const NAVIGATOR_COORDINATE_SCALE = 1000;

// Screenshot-trimming defaults mirror Yutori's reference loop:
// https://github.com/yutori-ai/yutori-sdk-python/blob/main/yutori/navigator/payload.py
// Trimming is size-triggered — we only drop old screenshots when the payload
// exceeds MAX_REQUEST_BYTES, and we always keep at least KEEP_RECENT_SCREENSHOTS.
const MAX_REQUEST_BYTES = 9_500_000;
const KEEP_RECENT_SCREENSHOTS = 6;

interface YutoriExtras {
  tool_set: string;
  disable_tools: string[];
}

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
  userTimezone?: string;
  userLocation?: string;
}

export interface SamplingLoopResult {
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
  maxIterations = 100,
  viewportWidth = 1280,
  viewportHeight = 800,
  kioskMode = false,
  userTimezone = 'America/Los_Angeles',
  userLocation = 'San Francisco, CA, US',
}: SamplingLoopOptions): Promise<SamplingLoopResult> {
  const client = new OpenAI({
    apiKey,
    baseURL: 'https://api.yutori.com/v1',
  });

  const computerTool = new ComputerTool(kernel, sessionId, viewportWidth, viewportHeight, kioskMode);

  const initialScreenshot = await computerTool.screenshot();

  // Append location/timezone/current-date context to the task — mirrors Yutori's
  // format_task_with_context helper and helps the model with date-sensitive
  // judgments. https://github.com/yutori-ai/yutori-sdk-python/blob/main/yutori/navigator/context.py
  const taskWithContext = formatTaskWithContext(task, userTimezone, userLocation);

  const conversationMessages: OpenAI.ChatCompletionMessageParam[] = [
    {
      role: 'user',
      content: [
        { type: 'text', text: taskWithContext },
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

    const { messages: requestMessages, removed } = trimmedForRequest(conversationMessages);
    if (removed > 0) {
      console.log(`Trimmed ${removed} old screenshot(s) to fit request size limit`);
    }

    let response;
    try {
      // n1.5-specific knobs (not in OpenAI SDK types). The openai-node SDK
      // serializes the body as-is, so these go at the top level via a spread —
      // unlike the Python SDK, there is no `extra_body` kwarg here.
      // tool_set selects the core (coordinate-based) tools.
      // disable_tools is a defense-in-depth exclusion of DOM/Playwright tools.
      const yutoriExtras: YutoriExtras = {
        tool_set: TOOL_SET,
        disable_tools: DISABLED_TOOLS,
      };
      response = await client.chat.completions.create({
        model,
        messages: requestMessages,
        max_completion_tokens: maxCompletionTokens,
        temperature: 0.3,
        ...yutoriExtras,
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

  // If the loop exhausted iterations, prompt the model for a final summary so
  // the caller gets a usable answer instead of empty content. Mirrors Yutori's
  // format_stop_and_summarize helper.
  if (iteration >= maxIterations && !finalAnswer) {
    console.log('Max iterations reached — requesting summary');
    try {
      const finalScreenshot = await computerTool.screenshot();
      conversationMessages.push({
        role: 'user',
        content: [
          { type: 'text', text: formatStopAndSummarize(task) },
          ...(finalScreenshot.base64Image
            ? [{
                type: 'image_url' as const,
                image_url: { url: `data:image/webp;base64,${finalScreenshot.base64Image}` },
              }]
            : []),
        ],
      });
      const { messages: summaryMessages } = trimmedForRequest(conversationMessages);
      const summaryResponse = await client.chat.completions.create({
        model,
        messages: summaryMessages,
        max_completion_tokens: maxCompletionTokens,
        temperature: 0.3,
        ...({ tool_set: TOOL_SET, disable_tools: DISABLED_TOOLS } satisfies YutoriExtras),
      });
      const summary = summaryResponse.choices[0]?.message;
      if (summary) {
        conversationMessages.push(summary);
        finalAnswer = summary.content || undefined;
      }
    } catch (error) {
      console.error('Stop-and-summarize call failed:', error);
    }
  }

  return {
    messages: conversationMessages,
    finalAnswer,
  };
}

function formatTaskWithContext(task: string, userTimezone: string, userLocation: string): string {
  const now = new Date();
  const tzLabel = resolveTimezone(userTimezone);
  const timeFormatter = new Intl.DateTimeFormat('en-US', {
    timeZone: tzLabel,
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    timeZoneName: 'short',
  });
  const dateFormatter = new Intl.DateTimeFormat('en-US', {
    timeZone: tzLabel,
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });
  const weekdayFormatter = new Intl.DateTimeFormat('en-US', { timeZone: tzLabel, weekday: 'long' });

  const context = [
    `User's location: ${userLocation}`,
    `User's timezone: ${tzLabel}`,
    `Current Date: ${dateFormatter.format(now)}`,
    `Current Time: ${timeFormatter.format(now)}`,
    `Today is: ${weekdayFormatter.format(now)}`,
  ].join('\n');

  return `${task}\n\n${context}`;
}

function resolveTimezone(userTimezone: string): string {
  for (const timeZone of [userTimezone, 'America/Los_Angeles', 'UTC']) {
    try {
      new Intl.DateTimeFormat('en-US', { timeZone }).format(new Date());
      return timeZone;
    } catch {
      // Try the next fallback.
    }
  }
  return 'UTC';
}

function formatStopAndSummarize(task: string): string {
  return (
    `Stop here. ` +
    `Summarize your current progress and list in detail all the findings ` +
    `relevant to the given task:\n${task}\n` +
    `Provide URLs for all relevant results you find and return them in your response. ` +
    `If there is no specific URL for a result, ` +
    `cite the page URL that the information was found on.`
  );
}

function scaleCoordinates(action: N15Action, viewportWidth: number, viewportHeight: number): N15Action {
  const scaled = { ...action };

  if (scaled.coordinates) {
    scaled.coordinates = denormalize(scaled.coordinates, viewportWidth, viewportHeight);
  }

  if (scaled.start_coordinates) {
    scaled.start_coordinates = denormalize(scaled.start_coordinates, viewportWidth, viewportHeight);
  }

  return scaled;
}

// Map [0, 1000] coordinates into viewport pixels and clamp to [0, dim-1] so a
// boundary value like 1000 doesn't land one pixel outside the viewport.
function denormalize(coords: [number, number], width: number, height: number): [number, number] {
  const rawX = Math.round((coords[0] / NAVIGATOR_COORDINATE_SCALE) * width);
  const rawY = Math.round((coords[1] / NAVIGATOR_COORDINATE_SCALE) * height);
  const x = Math.max(0, Math.min(width - 1, rawX));
  const y = Math.max(0, Math.min(height - 1, rawY));
  return [x, y];
}

interface ImagePart {
  type: 'image_url';
  image_url: { url: string };
}

interface TextPart {
  type: 'text';
  text: string;
}

type ContentPart = ImagePart | TextPart | Record<string, unknown>;

function estimateSize(messages: OpenAI.ChatCompletionMessageParam[]): number {
  return Buffer.byteLength(JSON.stringify(messages), 'utf-8');
}

function messageHasImage(msg: OpenAI.ChatCompletionMessageParam): boolean {
  const content = (msg as { content?: unknown }).content;
  if (!Array.isArray(content)) return false;
  return content.some((p) => typeof p === 'object' && p !== null && (p as { type?: unknown }).type === 'image_url');
}

function stripOneImage(msg: OpenAI.ChatCompletionMessageParam): boolean {
  const content = (msg as { content?: unknown }).content;
  if (!Array.isArray(content)) return false;

  let removed = false;
  const next: ContentPart[] = [];
  for (const part of content as ContentPart[]) {
    if (!removed && typeof part === 'object' && part !== null && (part as { type?: unknown }).type === 'image_url') {
      removed = true;
      continue;
    }
    next.push(part);
  }
  if (!removed) return false;

  const hasText = next.some((p) => typeof p === 'object' && p !== null && (p as { type?: unknown }).type === 'text');
  if (!hasText) {
    next.push({ type: 'text', text: 'Screenshot omitted to stay under request size limit.' });
  }

  (msg as { content: unknown }).content = next;
  return true;
}

function trimmedForRequest(
  messages: OpenAI.ChatCompletionMessageParam[],
): { messages: OpenAI.ChatCompletionMessageParam[]; removed: number } {
  // Deep-copy so the caller's full history is preserved unchanged.
  const trimmed = JSON.parse(JSON.stringify(messages)) as OpenAI.ChatCompletionMessageParam[];

  let size = estimateSize(trimmed);
  if (size <= MAX_REQUEST_BYTES) return { messages: trimmed, removed: 0 };

  const imageIndices: number[] = [];
  for (let i = 0; i < trimmed.length; i++) {
    if (messageHasImage(trimmed[i]!)) imageIndices.push(i);
  }
  if (imageIndices.length === 0) return { messages: trimmed, removed: 0 };

  const keep = Math.max(1, KEEP_RECENT_SCREENSHOTS);
  const protectedIdx = new Set(imageIndices.slice(-keep));
  let removed = 0;

  for (const idx of imageIndices) {
    if (size <= MAX_REQUEST_BYTES) break;
    if (protectedIdx.has(idx)) continue;
    if (stripOneImage(trimmed[idx]!)) {
      removed++;
      size = estimateSize(trimmed);
    }
  }

  // If still over, strip from the protected window too — but always keep the latest.
  if (size > MAX_REQUEST_BYTES) {
    const lastIdx = imageIndices[imageIndices.length - 1]!;
    for (const idx of imageIndices) {
      if (size <= MAX_REQUEST_BYTES) break;
      if (idx === lastIdx) continue;
      if (stripOneImage(trimmed[idx]!)) {
        removed++;
        size = estimateSize(trimmed);
      }
    }
  }

  return { messages: trimmed, removed };
}
