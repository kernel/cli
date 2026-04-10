/**
 * Tzafon Northstar Sampling Loop
 *
 * Runs the Northstar CUA model via the Lightcone Responses API using explicit
 * function tools (click, type, key, scroll, drag, done). Full conversation
 * history is maintained in the input array — each tool result includes a fresh
 * screenshot so the model always sees the current screen state.
 *
 * @see https://docs.lightcone.ai
 */

import type { Kernel } from '@onkernel/sdk';
import Lightcone from '@tzafon/lightcone';
import { ComputerTool } from './tools/computer';

const MODEL = 'tzafon.northstar-cua-fast';

const INSTRUCTIONS = [
  'Use a mouse and keyboard to interact with a Chromium browser and take screenshots.',
  '* Chromium is already open on a Kernel cloud browser. If a startup wizard appears, ignore it.',
  "* The screen's coordinate space is a 0-999 grid.",
  "* To navigate to a URL, use point_and_type on the address bar, or key('ctrl+l') to focus it first.",
  '* Some pages may take time to load. Wait and take successive screenshots to confirm the result.',
  '* Whenever you click on an element, consult the screenshot to determine coordinates first.',
  '* Click buttons, links, and icons in the center of the element, not on edges.',
  "* If a click didn't work, try adjusting the coordinates slightly.",
  "* For full-page scrolling, prefer key('PageDown') / key('PageUp') over the scroll tool.",
  '* After each action, evaluate the screenshot to confirm it succeeded before moving on.',
  '* When the task is complete, call done() with a summary of what you found or accomplished.',
].join('\n');

interface FunctionTool {
  type: 'function';
  name: string;
  description: string;
  parameters: Record<string, unknown>;
}

const TOOLS: FunctionTool[] = [
  {
    type: 'function', name: 'click',
    description: 'Single click at (x, y) in 0-999 grid.',
    parameters: {
      type: 'object',
      properties: {
        x: { type: 'integer', description: 'X in 0-999 grid' },
        y: { type: 'integer', description: 'Y in 0-999 grid' },
        button: { type: 'string', enum: ['left', 'right'] },
      },
      required: ['x', 'y'],
    },
  },
  {
    type: 'function', name: 'double_click',
    description: 'Double click at (x, y) in 0-999 grid.',
    parameters: {
      type: 'object',
      properties: {
        x: { type: 'integer', description: 'X in 0-999 grid' },
        y: { type: 'integer', description: 'Y in 0-999 grid' },
      },
      required: ['x', 'y'],
    },
  },
  {
    type: 'function', name: 'point_and_type',
    description: 'Click at position then type text. For input fields, search bars, address bars.',
    parameters: {
      type: 'object',
      properties: {
        x: { type: 'integer', description: 'X in 0-999 grid' },
        y: { type: 'integer', description: 'Y in 0-999 grid' },
        text: { type: 'string' },
        press_enter: { type: 'boolean', description: 'Press Enter after typing' },
      },
      required: ['x', 'y', 'text'],
    },
  },
  {
    type: 'function', name: 'key',
    description: "Press key combo (e.g. 'Enter', 'ctrl+a', 'Tab').",
    parameters: {
      type: 'object',
      properties: { keys: { type: 'string' } },
      required: ['keys'],
    },
  },
  {
    type: 'function', name: 'scroll',
    description: 'Scroll at (x, y) in 0-999 grid. Positive dy = down, negative = up.',
    parameters: {
      type: 'object',
      properties: {
        x: { type: 'integer', description: 'X in 0-999 grid' },
        y: { type: 'integer', description: 'Y in 0-999 grid' },
        dy: { type: 'integer', description: 'Scroll notches. 3=down, -3=up.' },
      },
      required: ['x', 'y', 'dy'],
    },
  },
  {
    type: 'function', name: 'drag',
    description: 'Drag from (x1, y1) to (x2, y2) in 0-999 grid.',
    parameters: {
      type: 'object',
      properties: {
        x1: { type: 'integer', description: 'Start X in 0-999 grid' },
        y1: { type: 'integer', description: 'Start Y in 0-999 grid' },
        x2: { type: 'integer', description: 'End X in 0-999 grid' },
        y2: { type: 'integer', description: 'End Y in 0-999 grid' },
      },
      required: ['x1', 'y1', 'x2', 'y2'],
    },
  },
  {
    type: 'function', name: 'done',
    description: 'Task complete. Report findings.',
    parameters: {
      type: 'object',
      properties: { result: { type: 'string' } },
      required: ['result'],
    },
  },
];

interface SamplingLoopOptions {
  task: string;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  model?: string;
  maxSteps?: number;
  viewportWidth?: number;
  viewportHeight?: number;
}

interface SamplingLoopResult {
  messages: string[];
  finalResult?: string;
}

function get(obj: any, key: string, fallback?: any): any {
  if (obj && typeof obj === 'object' && key in obj) return obj[key];
  return fallback;
}

function img(screenshotUrl: string, text = 'screenshot') {
  return {
    role: 'user',
    content: [
      { type: 'input_text', text },
      { type: 'input_image', image_url: screenshotUrl, detail: 'auto' },
    ],
  };
}

export async function samplingLoop({
  task,
  apiKey,
  kernel,
  sessionId,
  model = MODEL,
  maxSteps = 50,
  viewportWidth = 1280,
  viewportHeight = 800,
}: SamplingLoopOptions): Promise<SamplingLoopResult> {
  const tzafon = new Lightcone({ apiKey });
  const computer = new ComputerTool(kernel, sessionId, viewportWidth, viewportHeight);

  let screenshotUrl = await computer.captureScreenshot();
  const items: any[] = [img(screenshotUrl, `${task}\n\nCurrent screenshot:`)];

  let resp: any;

  for (let step = 0; step < maxSteps; step++) {
    console.log(`\n=== Step ${step + 1}/${maxSteps} ===`);

    // Prevent unbounded payload growth — keep the task prompt + recent history
    if (items.length > 30) {
      items.splice(2, items.length - 22);
    }

    resp = await tzafon.responses.create({
      model,
      input: items,
      tools: TOOLS,
      instructions: INSTRUCTIONS,
      temperature: 0,
      max_output_tokens: 4096,
    });

    const calls: Array<{ callId: string; name: string; args: Record<string, any> }> = [];

    for (const item of get(resp, 'output') ?? []) {
      const itemType = get(item, 'type');

      if (itemType === 'message') {
        for (const block of get(item, 'content') ?? []) {
          const text = get(block, 'text', '');
          if (text) {
            items.push({ role: 'assistant', content: text });
            console.log(`  Model: ${text.slice(0, 150)}`);
          }
        }
      } else if (itemType === 'function_call') {
        const callId = get(item, 'call_id');
        const name = get(item, 'name');
        const rawArgs = get(item, 'arguments', '{}');
        let args: Record<string, any>;
        try {
          args = typeof rawArgs === 'string' ? JSON.parse(rawArgs) : rawArgs;
        } catch {
          args = {};
        }
        calls.push({ callId, name, args });
        items.push({
          type: 'function_call', call_id: callId, name,
          arguments: typeof rawArgs === 'string' ? rawArgs : JSON.stringify(rawArgs),
        });
      }
    }

    if (calls.length === 0) continue;

    for (const { callId, name, args } of calls) {
      console.log(`  [${step + 1}] ${name}(${JSON.stringify(args).slice(0, 100)})`);

      if (name === 'done') {
        const result = args.result ?? '';
        items.push({ type: 'function_call_output', call_id: callId, output: 'ok' });
        console.log(`  Done: ${result}`);
        return { messages: [], finalResult: result };
      }

      try {
        await computer.executeFunction(name, args);
      } catch (e: any) {
        console.log(`  Action failed: ${e.message}`);
        items.push({ type: 'function_call_output', call_id: callId, output: `Error: ${e.message}` });
        continue;
      }

      await new Promise((r) => setTimeout(r, 500));
      screenshotUrl = await computer.captureScreenshot();

      // Replace old screenshots with placeholders to save payload space
      for (const it of items.slice(0, -1)) {
        const c = it?.content;
        if (Array.isArray(c) && c.some((p: any) => p?.type === 'input_image')) {
          it.content = c.filter((p: any) => p?.type !== 'input_image');
          if (it.content.length === 0) it.content = '(old screenshot)';
        }
      }

      items.push({ type: 'function_call_output', call_id: callId, output: '[screenshot]' });
      items.push(img(screenshotUrl));
    }
  }

  const messages = (get(resp, 'output') ?? [])
    .filter((o: any) => get(o, 'type') === 'message')
    .flatMap((o: any) => (get(o, 'content') ?? []).map((c: any) => get(c, 'text')))
    .filter(Boolean);

  return { messages, finalResult: undefined };
}
