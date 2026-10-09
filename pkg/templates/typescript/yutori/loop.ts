/**
 * Yutori n2 Sampling Loop
 *
 * Implements the agent loop for Yutori's n2 computer use model.
 * n2 uses an OpenAI-compatible API with tool_calls:
 * - GUI actions arrive as a single `computer_batch` call holding an action list
 * - `bash`, `read`, `write`, and `edit` arrive as their own calls
 * - Every call needs one tool result with a matching tool_call_id
 * - The model stops by returning content without tool_calls
 * - Coordinates are returned in 1000x1000 space and need scaling
 *
 * @see https://docs.yutori.com/reference/n2
 */

import OpenAI from 'openai';
import type { Kernel } from '@onkernel/sdk';
import { ComputerTool, type BatchOutcome, type N2Action } from './tools/computer';
import { SystemTools } from './tools/system';

// Pin the tool set so a change to the server default can't change which tools
// this run exposes. n2 rejects `disable_tools`, so the set is served whole.
const TOOL_SET = 'computer_use_tools-20260825';

export type ReasoningEffort = 'none' | 'low' | 'medium' | 'xhigh';

// Requests are capped at 10 MB. n2 also only reads images from the last 2
// image-bearing messages, so anything older is dropped before sending rather
// than paying to upload screenshots the model will discard server-side.
const MAX_REQUEST_BYTES = 9_500_000;
const KEEP_IMAGE_MESSAGES = 2;

interface YutoriExtras {
  tool_set: string;
  reasoning_effort?: ReasoningEffort;
  prev_request_id?: string;
}

interface SamplingLoopOptions {
  model?: string;
  task: string;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  maxCompletionTokens?: number;
  maxIterations?: number;
  screenWidth?: number;
  screenHeight?: number;
  reasoningEffort?: ReasoningEffort;
  userTimezone?: string;
  userLocation?: string;
}

export interface SamplingLoopResult {
  messages: OpenAI.ChatCompletionMessageParam[];
  finalAnswer?: string;
}

export async function samplingLoop({
  model = 'n2',
  task,
  apiKey,
  kernel,
  sessionId,
  // Shared by the reasoning trace and the tool call, so a low value truncates
  // the call.
  maxCompletionTokens = 16384,
  maxIterations = 100,
  screenWidth = 1280,
  screenHeight = 800,
  reasoningEffort,
  userTimezone = 'America/Los_Angeles',
  userLocation = 'San Francisco, CA, US',
}: SamplingLoopOptions): Promise<SamplingLoopResult> {
  const client = new OpenAI({
    apiKey,
    baseURL: 'https://api.yutori.com/v1',
  });

  const computerTool = new ComputerTool(kernel, sessionId, screenWidth, screenHeight);
  const systemTools = new SystemTools(kernel, sessionId);

  // Append location/timezone/current-date context to the task — mirrors Yutori's
  // format_task_with_context helper and helps the model with date-sensitive
  // judgments. https://github.com/yutori-ai/yutori-sdk-python/blob/main/yutori/navigator/context.py
  const conversationMessages: OpenAI.ChatCompletionMessageParam[] = [
    {
      role: 'user',
      content: [
        { type: 'text', text: formatTaskWithContext(task, userTimezone, userLocation) },
        imagePart(await computerTool.screenshot()),
      ],
    },
  ];

  let iteration = 0;
  let finalAnswer: string | undefined;
  let prevRequestId: string | undefined;

  while (iteration < maxIterations) {
    iteration++;
    console.log(`\n=== Iteration ${iteration} ===`);

    const requestMessages = trimmedForRequest(conversationMessages);

    // n2-specific knobs (not in OpenAI SDK types). The openai-node SDK
    // serializes the body as-is, so these go at the top level via a spread —
    // unlike the Python SDK, there is no `extra_body` kwarg here.
    const yutoriExtras: YutoriExtras = {
      tool_set: TOOL_SET,
      ...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
      // Groups the trajectory into one conversation for usage reporting.
      ...(prevRequestId ? { prev_request_id: prevRequestId } : {}),
    };

    let response;
    try {
      response = await client.chat.completions.create(
        requestBody(model, requestMessages, maxCompletionTokens, yutoriExtras),
      );
    } catch (apiError) {
      console.error('API call failed:', apiError);
      throw apiError;
    }

    prevRequestId = (response as { _request_id?: string })._request_id ?? prevRequestId;

    const assistantMessage = response.choices?.[0]?.message;
    if (!assistantMessage) {
      console.error('No choices in response:', JSON.stringify(response, null, 2));
      throw new Error('No response from model');
    }

    console.log('Assistant content:', assistantMessage.content || '(none)');

    // Push the assistant message unchanged — `reasoning_content` rides along on
    // it, and n2 needs it echoed back to keep its reasoning across turns.
    conversationMessages.push(assistantMessage);

    const toolCalls = assistantMessage.tool_calls;

    // No tool_calls means the model is done
    if (!toolCalls || toolCalls.length === 0) {
      finalAnswer = assistantMessage.content || undefined;
      console.log('No tool_calls, model is done. Final answer:', finalAnswer);
      break;
    }

    for (const [index, toolCall] of toolCalls.entries()) {
      // n2 usually answers with one call, but `parallel_tool_calls` is pinned on
      // server-side so a turn can carry several. Only the first one was planned
      // against the screenshot the model actually saw, so run it and make the
      // rest re-plan.
      if (index > 0) {
        conversationMessages.push({
          role: 'tool',
          tool_call_id: toolCall.id,
          content: 'Not executed: planned against a screenshot the previous call already changed. Re-plan from the latest screenshot.',
        });
        continue;
      }

      conversationMessages.push(await runToolCall(toolCall, computerTool, systemTools));
    }
  }

  // If the loop exhausted iterations, prompt the model for a final summary so
  // the caller gets a usable answer instead of empty content. Mirrors Yutori's
  // format_stop_and_summarize helper.
  if (iteration >= maxIterations && !finalAnswer) {
    console.log('Max iterations reached — requesting summary');
    try {
      conversationMessages.push({
        role: 'user',
        content: [
          { type: 'text', text: formatStopAndSummarize(task) },
          imagePart(await computerTool.screenshot()),
        ],
      });

      const summaryResponse = await client.chat.completions.create(
        requestBody(model, trimmedForRequest(conversationMessages), maxCompletionTokens, {
          tool_set: TOOL_SET,
        }),
      );

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

// n2's knobs are not in the OpenAI SDK types — the openai-node SDK serializes
// the body as-is, so they ride at the top level. `reasoning_effort` collides
// with OpenAI's own narrower enum, hence the cast on the way out.
function requestBody(
  model: string,
  messages: OpenAI.ChatCompletionMessageParam[],
  maxCompletionTokens: number,
  extras: YutoriExtras,
): OpenAI.ChatCompletionCreateParamsNonStreaming {
  return {
    model,
    messages,
    max_completion_tokens: maxCompletionTokens,
    temperature: 0.6,
    ...extras,
  } as OpenAI.ChatCompletionCreateParamsNonStreaming;
}

async function runToolCall(
  toolCall: OpenAI.ChatCompletionMessageToolCall,
  computerTool: ComputerTool,
  systemTools: SystemTools,
): Promise<OpenAI.ChatCompletionMessageParam> {
  const name = toolCall.function.name;

  let args: Record<string, unknown>;
  try {
    args = JSON.parse(toolCall.function.arguments);
  } catch {
    console.error('Failed to parse tool_call arguments:', toolCall.function.arguments);
    return { role: 'tool', tool_call_id: toolCall.id, content: 'Error: failed to parse arguments' };
  }

  console.log('Executing tool:', name, JSON.stringify(args));

  if (name === 'computer_batch') {
    const outcome = await computerTool.runBatch((args.actions ?? []) as N2Action[]);

    return {
      role: 'tool',
      tool_call_id: toolCall.id,
      // n2 accepts image content arrays in tool messages (not yet in OpenAI SDK types)
      content: [
        { type: 'text', text: describeOutcome(outcome) },
        imagePart(await computerTool.screenshot()),
      ] as unknown as string,
    };
  }

  try {
    return { role: 'tool', tool_call_id: toolCall.id, content: await systemTools.execute(name, args) };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`${name} failed:`, message);
    return { role: 'tool', tool_call_id: toolCall.id, content: `${name} failed: ${message}` };
  }
}

function describeOutcome({ executed, total, failure }: BatchOutcome): string {
  if (!failure) {
    return `Executed ${executed} of ${total} actions.`;
  }

  return (
    `Executed ${executed} of ${total} actions. ` +
    `Action ${failure.index + 1} (${failure.name}) failed: ${failure.message}. ` +
    `The remaining actions were skipped.`
  );
}

function imagePart(base64Image: string): OpenAI.ChatCompletionContentPartImage {
  return { type: 'image_url', image_url: { url: `data:image/webp;base64,${base64Image}` } };
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

function stripImages(msg: OpenAI.ChatCompletionMessageParam): void {
  const content = (msg as { content?: unknown }).content;
  if (!Array.isArray(content)) return;

  const next = (content as ContentPart[]).filter(
    (p) => !(typeof p === 'object' && p !== null && (p as { type?: unknown }).type === 'image_url'),
  );

  const hasText = next.some((p) => typeof p === 'object' && p !== null && (p as { type?: unknown }).type === 'text');
  if (!hasText) {
    next.push({ type: 'text', text: 'Screenshot omitted — superseded by a newer one.' });
  }

  (msg as { content: unknown }).content = next;
}

/**
 * Drop the screenshots n2 will not look at, keeping the text of every turn.
 *
 * The model reads images from the last KEEP_IMAGE_MESSAGES image-bearing
 * messages and ignores the rest, so those are stripped every request. If the
 * payload is still over the cap after that, the older of the kept screenshots
 * goes too — the latest one is always kept, since a request with no screenshot
 * badly degrades grounding.
 */
function trimmedForRequest(
  messages: OpenAI.ChatCompletionMessageParam[],
): OpenAI.ChatCompletionMessageParam[] {
  // Deep-copy so the caller's full history is preserved unchanged.
  const trimmed = JSON.parse(JSON.stringify(messages)) as OpenAI.ChatCompletionMessageParam[];

  const imageIndices: number[] = [];
  for (let i = 0; i < trimmed.length; i++) {
    if (messageHasImage(trimmed[i]!)) imageIndices.push(i);
  }

  for (const idx of imageIndices.slice(0, -KEEP_IMAGE_MESSAGES)) {
    stripImages(trimmed[idx]!);
  }

  // The latest screenshot is never dropped — a request without one badly
  // degrades grounding — so the previous one is the only thing left to give up.
  const kept = imageIndices.slice(-KEEP_IMAGE_MESSAGES);
  for (const idx of kept.slice(0, -1)) {
    if (estimateSize(trimmed) <= MAX_REQUEST_BYTES) break;
    console.warn('Payload still over the 10 MB cap — dropping the previous screenshot too');
    stripImages(trimmed[idx]!);
  }

  return trimmed;
}
