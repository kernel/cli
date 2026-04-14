/**
 * Tzafon CUA provider adapter.
 *
 * Uses the Tzafon Lightcone Responses API with function tools (click, type,
 * key, scroll, drag, done). Coordinates arrive in a normalised 0-999 grid.
 *
 * @see https://docs.lightcone.ai
 */

import Lightcone from '@tzafon/lightcone';
import type { CuaProvider, TaskOptions, TaskResult } from './index';

const DEFAULT_MODEL = 'tzafon.northstar-cua-fast';

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

const KEY_MAP: Record<string, string> = {
  return: 'Return', enter: 'Return',
  space: 'space', tab: 'Tab',
  backspace: 'BackSpace', delete: 'Delete',
  escape: 'Escape', esc: 'Escape', insert: 'Insert',
  up: 'Up', down: 'Down', left: 'Left', right: 'Right',
  home: 'Home', end: 'End',
  pageup: 'Page_Up', page_up: 'Page_Up',
  pagedown: 'Page_Down', page_down: 'Page_Down',
  ...Object.fromEntries(Array.from({ length: 12 }, (_, i) => [`f${i + 1}`, `F${i + 1}`])),
};

const MODIFIER_MAP: Record<string, string> = {
  ctrl: 'ctrl', control: 'ctrl',
  alt: 'alt', shift: 'shift',
  meta: 'super', cmd: 'super', command: 'super', win: 'super',
};

function mapKey(keyCombo: string): string {
  const parts = keyCombo.includes('+') ? keyCombo.split('+') : [keyCombo];
  return parts
    .map((p) => {
      const k = p.trim().toLowerCase();
      return MODIFIER_MAP[k] ?? KEY_MAP[k] ?? p.trim();
    })
    .join('+');
}

/** Parse a coordinate value. Handles the model's occasional '470,77' format. */
function coord(val: unknown): number {
  if (val == null) return 0;
  let s = String(val);
  if (s.includes(',')) s = s.split(',')[0].trim();
  return Math.trunc(Number(s));
}

function get(obj: any, key: string, fallback?: any): any {
  if (obj && typeof obj === 'object' && key in obj) return obj[key];
  return fallback;
}

export class TzafonProvider implements CuaProvider {
  readonly name = 'tzafon';
  private apiKey: string;

  constructor() {
    this.apiKey = process.env.TZAFON_API_KEY ?? '';
  }

  isConfigured(): boolean {
    return this.apiKey.length > 0;
  }

  async runTask(options: TaskOptions): Promise<TaskResult> {
    const { query, kernel, sessionId, viewportWidth = 1280, viewportHeight = 800 } = options;
    const model = options.model || DEFAULT_MODEL;
    const tzafon = new Lightcone({ apiKey: this.apiKey });
    const computer = kernel.browsers.computer;
    const maxSteps = 50;

    const captureScreenshot = async (): Promise<string> => {
      const res = await computer.captureScreenshot(sessionId);
      const buf = Buffer.from(await res.arrayBuffer());
      return `data:image/png;base64,${buf.toString('base64')}`;
    };

    const scale = (x: unknown, y: unknown): [number, number] => {
      const cx = coord(x);
      const cy = coord(y);
      const px = Math.max(0, Math.min(Math.trunc(cx * (viewportWidth - 1) / 999), viewportWidth - 1));
      const py = Math.max(0, Math.min(Math.trunc(cy * (viewportHeight - 1) / 999), viewportHeight - 1));
      return [px, py];
    };

    const executeFunction = async (name: string, args: Record<string, any>): Promise<void> => {
      switch (name) {
        case 'click': {
          const [px, py] = scale(args.x, args.y);
          await computer.clickMouse(sessionId, { x: px, y: py, button: args.button ?? 'left' });
          break;
        }
        case 'double_click': {
          const [px, py] = scale(args.x, args.y);
          await computer.clickMouse(sessionId, { x: px, y: py, num_clicks: 2 });
          break;
        }
        case 'point_and_type': {
          const [px, py] = scale(args.x, args.y);
          await computer.clickMouse(sessionId, { x: px, y: py });
          await new Promise(r => setTimeout(r, 300));
          await computer.typeText(sessionId, { text: args.text });
          if (args.press_enter) {
            await new Promise(r => setTimeout(r, 100));
            await computer.pressKey(sessionId, { keys: ['Return'] });
          }
          break;
        }
        case 'key': {
          await computer.pressKey(sessionId, { keys: [mapKey(args.keys)] });
          break;
        }
        case 'scroll': {
          const [px, py] = scale(args.x ?? 500, args.y ?? 500);
          const dy = Math.max(-10, Math.min(10, args.dy ?? 3));
          await computer.scroll(sessionId, { x: px, y: py, delta_x: 0, delta_y: dy });
          break;
        }
        case 'drag': {
          const [px1, py1] = scale(args.x1, args.y1);
          const [px2, py2] = scale(args.x2, args.y2);
          await computer.dragMouse(sessionId, { path: [[px1, py1], [px2, py2]] });
          break;
        }
        default:
          throw new Error(`Unknown function: ${name}`);
      }
    };

    const img = (screenshotUrl: string, text = 'screenshot') => ({
      role: 'user',
      content: [
        { type: 'input_text', text },
        { type: 'input_image', image_url: screenshotUrl, detail: 'auto' },
      ],
    });

    let screenshotUrl = await captureScreenshot();
    const items: any[] = [img(screenshotUrl, `${query}\n\nCurrent screenshot:`)];
    let resp: any;

    for (let step = 0; step < maxSteps; step++) {
      // Prevent unbounded payload growth
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
            }
          }
        } else if (itemType === 'function_call') {
          const callId = get(item, 'call_id');
          const fnName = get(item, 'name');
          const rawArgs = get(item, 'arguments', '{}');
          let args: Record<string, any>;
          try {
            args = typeof rawArgs === 'string' ? JSON.parse(rawArgs) : rawArgs;
          } catch {
            args = {};
          }
          calls.push({ callId, name: fnName, args });
          items.push({
            type: 'function_call', call_id: callId, name: fnName,
            arguments: typeof rawArgs === 'string' ? rawArgs : JSON.stringify(rawArgs),
          });
        }
      }

      if (calls.length === 0) continue;

      for (const { callId, name, args } of calls) {
        if (name === 'done') {
          return { result: args.result ?? '', provider: this.name };
        }

        try {
          await executeFunction(name, args);
        } catch (e: any) {
          items.push({ type: 'function_call_output', call_id: callId, output: `Error: ${e.message}` });
          continue;
        }

        await new Promise(r => setTimeout(r, 500));
        screenshotUrl = await captureScreenshot();

        // Replace old screenshots with placeholders
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

    // Extract any final text from the last response
    const messages = (get(resp, 'output') ?? [])
      .filter((o: any) => get(o, 'type') === 'message')
      .flatMap((o: any) => (get(o, 'content') ?? []).map((c: any) => get(c, 'text')))
      .filter(Boolean);

    return { result: messages.join(' ') || '(max iterations reached)', provider: this.name };
  }
}
