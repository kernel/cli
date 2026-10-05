/**
 * Moondream computer-use agent loop.
 */

import type { Kernel } from '@onkernel/sdk';
import { ComputerTool } from './tools/computer';
import {
  ComputerAction,
  COORDINATE_SCALE,
  DEFAULT_SCREEN_SIZE,
  type ScreenSize,
} from './tools/types/computer';
import type { MoondreamClient, MoondreamPoint } from './moondream';

const URL_RE = /https?:\/\/[^\s)]+/i;

export interface StepInput {
  action: string;
  url?: string;
  target?: string;
  text?: string;
  question?: string;
  direction?: 'up' | 'down' | 'left' | 'right';
  magnitude?: number;
  x?: number;
  y?: number;
  keys?: string;
  seconds?: number;
  retries?: number;
  retry_delay_ms?: number;
  pre_wait_ms?: number;
  press_enter?: boolean;
  clear_before_typing?: boolean;
  length?: 'short' | 'normal' | 'long';
}

export interface AgentOptions {
  maxRetries?: number;
  retryDelayMs?: number;
  strict?: boolean;
}

export interface AgentResult {
  finalResponse: string;
  error?: string;
}

interface StepLog {
  step: number;
  action: string;
  status: 'success' | 'failed';
  detail: string;
  output?: string;
}

export async function runAgent({
  query,
  steps,
  moondream,
  kernel,
  sessionId,
  options,
}: {
  query?: string;
  steps?: StepInput[];
  moondream: MoondreamClient;
  kernel: Kernel;
  sessionId: string;
  options: AgentOptions;
}): Promise<AgentResult> {
  const computer = new ComputerTool(kernel, sessionId);
  const parsedSteps = steps?.length ? steps : parseSteps(query || '');

  if (!parsedSteps.length) {
    throw new Error('No steps could be derived from the query. Provide steps or a query.');
  }

  const logs: StepLog[] = [];
  const answers: string[] = [];
  let lastScreenshot: string | undefined;
  let error: string | undefined;

  for (const [index, step] of parsedSteps.entries()) {
    const stepNumber = index + 1;
    const action = (step.action || '').trim().toLowerCase();

    if (!action) {
      logs.push({ step: stepNumber, action: 'unknown', status: 'failed', detail: 'Missing action' });
      if (options.strict) {
        error = 'Missing action in step';
        break;
      }
      continue;
    }

    try {
      if (step.pre_wait_ms) {
        await sleep(step.pre_wait_ms);
      }

      if (action === 'open_web_browser' || action === 'open') {
        const result = await computer.executeAction(ComputerAction.OPEN_WEB_BROWSER, {});
        lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
        logs.push({ step: stepNumber, action, status: status(result.error), detail: 'Opened browser' });
      } else if (action === 'navigate') {
        const url = step.url || findUrl(query || '');
        if (!url) throw new Error('navigate requires url');
        const result = await computer.executeAction(ComputerAction.NAVIGATE, { url });
        lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
        logs.push({ step: stepNumber, action, status: status(result.error), detail: `Navigated to ${url}` });
      } else if (action === 'go_back') {
        const result = await computer.executeAction(ComputerAction.GO_BACK, {});
        lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
        logs.push({ step: stepNumber, action, status: status(result.error), detail: 'Went back' });
      } else if (action === 'go_forward') {
        const result = await computer.executeAction(ComputerAction.GO_FORWARD, {});
        lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
        logs.push({ step: stepNumber, action, status: status(result.error), detail: 'Went forward' });
      } else if (action === 'search') {
        const result = await computer.executeAction(ComputerAction.SEARCH, {});
        lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
        logs.push({ step: stepNumber, action, status: status(result.error), detail: 'Focused address bar' });
      } else if (action === 'wait') {
        const seconds = step.seconds ?? 1;
        await sleep(seconds * 1000);
        logs.push({ step: stepNumber, action, status: 'success', detail: `Waited ${seconds.toFixed(2)}s` });
      } else if (action === 'key') {
        if (!step.keys) throw new Error('key action requires keys');
        const result = await computer.executeAction(ComputerAction.KEY_COMBINATION, { keys: step.keys });
        lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
        logs.push({ step: stepNumber, action, status: status(result.error), detail: `Pressed ${step.keys}` });
      } else if (action === 'scroll') {
        const direction = step.direction ?? 'down';
        const magnitude = step.magnitude;
        if (step.x !== undefined && step.y !== undefined) {
          const [xNorm, yNorm] = normalizePoint(step.x, step.y, computer.getScreenSize());
          const args: Record<string, number | string> = { x: xNorm, y: yNorm, direction };
          if (magnitude !== undefined) args.magnitude = magnitude;
          const result = await computer.executeAction(ComputerAction.SCROLL_AT, args);
          lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
        } else {
          const args: Record<string, number | string> = { direction };
          if (magnitude !== undefined) args.magnitude = magnitude;
          const result = await computer.executeAction(ComputerAction.SCROLL_DOCUMENT, args);
          lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
        }
        logs.push({ step: stepNumber, action, status: 'success', detail: `Scrolled ${direction}` });
      } else if (action === 'click' || action === 'type') {
        const target = step.target;
        const retries = step.retries ?? options.maxRetries ?? 3;
        const delayMs = step.retry_delay_ms ?? options.retryDelayMs ?? 1000;

        const coords = await resolveTargetCoords({
          step,
          target,
          moondream,
          computer,
          lastScreenshot,
          retries,
          delayMs,
        });

        if (!coords) throw new Error(`Unable to locate target: ${target}`);

        const [xNorm, yNorm] = coords;
        if (action === 'click') {
          const result = await computer.executeAction(ComputerAction.CLICK_AT, { x: xNorm, y: yNorm });
          lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
          logs.push({ step: stepNumber, action, status: status(result.error), detail: `Clicked ${target}` });
        } else {
          if (step.text === undefined) throw new Error('type action requires text');
          const result = await computer.executeAction(ComputerAction.TYPE_TEXT_AT, {
            x: xNorm,
            y: yNorm,
            text: String(step.text),
            press_enter: Boolean(step.press_enter),
            clear_before_typing: step.clear_before_typing !== false,
          });
          lastScreenshot = updateScreenshot(result.base64Image, lastScreenshot);
          logs.push({ step: stepNumber, action, status: status(result.error), detail: `Typed into ${target}` });
        }
      } else if (action === 'query') {
        const question = step.question || query;
        if (!question) throw new Error('query action requires question');
        const screenshot = await ensureScreenshot(computer, lastScreenshot);
        lastScreenshot = screenshot;
        const answer = await moondream.query(screenshot, String(question));
        answers.push(answer);
        logs.push({ step: stepNumber, action, status: 'success', detail: 'Answered question', output: answer });
      } else if (action === 'caption') {
        const length = step.length ?? 'normal';
        const screenshot = await ensureScreenshot(computer, lastScreenshot);
        lastScreenshot = screenshot;
        const caption = await moondream.caption(screenshot, length);
        answers.push(caption);
        logs.push({ step: stepNumber, action, status: 'success', detail: 'Generated caption', output: caption });
      } else {
        throw new Error(`Unknown action: ${action}`);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      logs.push({ step: stepNumber, action, status: 'failed', detail: message });
      error = message;
      if (options.strict) break;
    }
  }

  const summary = `Completed ${logs.filter(l => l.status === 'success').length}/${logs.length} steps`;
  const resultPayload = {
    summary,
    steps: logs,
    answers,
  };

  return {
    finalResponse: JSON.stringify(resultPayload, null, 2),
    error,
  };
}

export function parseSteps(query: string): StepInput[] {
  const trimmed = query.trim();
  if (!trimmed) return [];

  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    try {
      const data = JSON.parse(trimmed);
      if (Array.isArray(data)) return data as StepInput[];
      if (data && typeof data === 'object' && Array.isArray((data as { steps?: StepInput[] }).steps)) {
        return (data as { steps: StepInput[] }).steps;
      }
    } catch {
      // fall through
    }
  }

  const steps: StepInput[] = [];
  const url = findUrl(trimmed);
  if (url) steps.push({ action: 'navigate', url });

  const question = stripUrlAndNavigation(trimmed);
  const wantsCaption = /\bdescribe|caption\b/i.test(trimmed);

  if (wantsCaption) {
    steps.push({ action: 'caption' });
  } else if (question) {
    steps.push({ action: 'query', question });
  } else if (url) {
    steps.push({ action: 'caption' });
  } else {
    steps.push({ action: 'query', question: trimmed });
  }

  return steps;
}

function findUrl(query: string): string | undefined {
  const match = query.match(URL_RE);
  return match?.[0];
}

function stripUrlAndNavigation(query: string): string {
  let cleaned = query.replace(URL_RE, '');
  cleaned = cleaned.replace(/\b(navigate|open|go|visit)\b/gi, '');
  cleaned = cleaned.replace(/\bto\b/gi, ' ');
  cleaned = cleaned.replace(/\s+/g, ' ').trim();
  return cleaned.replace(/^[,.;:-]+|[,.;:-]+$/g, '');
}

function normalizePoint(x: number, y: number, screenSize?: ScreenSize): [number, number] {
  if (x >= 0 && x <= 1 && y >= 0 && y <= 1) {
    return [Math.round(x * COORDINATE_SCALE), Math.round(y * COORDINATE_SCALE)];
  }
  const width = screenSize?.width ?? DEFAULT_SCREEN_SIZE.width;
  const height = screenSize?.height ?? DEFAULT_SCREEN_SIZE.height;
  return [
    Math.round((x / width) * COORDINATE_SCALE),
    Math.round((y / height) * COORDINATE_SCALE),
  ];
}

function updateScreenshot(current?: string, fallback?: string): string | undefined {
  return current ?? fallback;
}

function status(error?: string): 'success' | 'failed' {
  return error ? 'failed' : 'success';
}

async function ensureScreenshot(computer: ComputerTool, lastScreenshot?: string): Promise<string> {
  if (lastScreenshot) return lastScreenshot;
  const result = await computer.screenshot();
  if (result.error || !result.base64Image) {
    throw new Error(result.error || 'Failed to capture screenshot');
  }
  return result.base64Image;
}

async function resolveTargetCoords({
  step,
  target,
  moondream,
  computer,
  lastScreenshot,
  retries,
  delayMs,
}: {
  step: StepInput;
  target?: string;
  moondream: MoondreamClient;
  computer: ComputerTool;
  lastScreenshot?: string;
  retries: number;
  delayMs: number;
}): Promise<[number, number] | undefined> {
  if (step.x !== undefined && step.y !== undefined) {
    return normalizePoint(step.x, step.y, computer.getScreenSize());
  }

  if (!target) return undefined;

  const attempts = Math.max(1, retries);
  let currentScreenshot = lastScreenshot;

  for (let attempt = 0; attempt < attempts; attempt++) {
    const screenshot = await ensureScreenshot(computer, currentScreenshot);
    const point = await moondream.point(screenshot, String(target));
    if (point) {
      return normalizePoint(point.x, point.y, computer.getScreenSize());
    }
    if (attempt < attempts - 1) {
      await sleep(delayMs);
      currentScreenshot = undefined;
    }
  }

  return undefined;
}

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}
