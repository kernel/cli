/**
 * OpenAI CUA provider adapter.
 *
 * Uses the OpenAI Responses API with computer use tool.
 */

import OpenAI from 'openai';
import type {
  ResponseInputItem,
  ResponseItem,
  ResponseComputerToolCall,
  ResponseOutputMessage,
} from 'openai/resources/responses/responses';
import type { CuaProvider, TaskOptions, TaskResult } from './index';

const KEYSYM_MAP: Record<string, string> = {
  ENTER: 'Return', Enter: 'Return', RETURN: 'Return',
  BACKSPACE: 'BackSpace', Backspace: 'BackSpace',
  DELETE: 'Delete', TAB: 'Tab', ESCAPE: 'Escape', Escape: 'Escape',
  SPACE: 'space', Space: 'space',
  UP: 'Up', DOWN: 'Down', LEFT: 'Left', RIGHT: 'Right',
  HOME: 'Home', END: 'End',
  PAGEUP: 'Prior', PAGE_UP: 'Prior', PageUp: 'Prior',
  PAGEDOWN: 'Next', PAGE_DOWN: 'Next', PageDown: 'Next',
  CTRL: 'Control_L', Ctrl: 'Control_L', CONTROL: 'Control_L', Control: 'Control_L',
  ALT: 'Alt_L', Alt: 'Alt_L',
  SHIFT: 'Shift_L', Shift: 'Shift_L',
  META: 'Super_L', Meta: 'Super_L', CMD: 'Super_L', COMMAND: 'Super_L',
  F1: 'F1', F2: 'F2', F3: 'F3', F4: 'F4', F5: 'F5', F6: 'F6',
  F7: 'F7', F8: 'F8', F9: 'F9', F10: 'F10', F11: 'F11', F12: 'F12',
};

const MODIFIER_KEYSYMS = new Set([
  'Control_L', 'Control_R', 'Alt_L', 'Alt_R',
  'Shift_L', 'Shift_R', 'Super_L', 'Super_R',
]);

function translateKeys(keys: string[]): string[] {
  return keys.map(k => KEYSYM_MAP[k] ?? k);
}

function expandAndTranslateKeys(keys: string[], holdKeys: string[]): { keys: string[]; holdKeys: string[] } {
  const expanded: string[] = [];
  for (const raw of keys) {
    const parts = raw.includes('+') ? raw.split('+') : [raw];
    for (const part of parts) {
      const trimmed = part.trim();
      if (trimmed) expanded.push(trimmed);
    }
  }

  const translated = translateKeys(expanded);
  const translatedHold = translateKeys(holdKeys);

  const holdFromKeys: string[] = [];
  const primaryKeys: string[] = [];
  for (const key of translated) {
    if (MODIFIER_KEYSYMS.has(key)) holdFromKeys.push(key);
    else primaryKeys.push(key);
  }

  if (primaryKeys.length === 0) return { keys: translated, holdKeys: translatedHold };

  const merged = [...new Set([...translatedHold, ...holdFromKeys])];
  return { keys: primaryKeys, holdKeys: merged };
}

interface CuaAction {
  type: string;
  x?: number;
  y?: number;
  text?: string;
  url?: string;
  keys?: string[];
  hold_keys?: string[];
  button?: string | number;
  scroll_x?: number;
  scroll_y?: number;
  ms?: number;
  path?: Array<{ x: number; y: number }>;
  [key: string]: unknown;
}

type BatchAction = {
  type: string;
  click_mouse?: { x: number; y: number; button?: string; num_clicks?: number };
  move_mouse?: { x: number; y: number };
  type_text?: { text: string };
  press_key?: { keys: string[]; hold_keys?: string[] };
  scroll?: { x: number; y: number; delta_x?: number; delta_y?: number };
  drag_mouse?: { path: number[][] };
  sleep?: { duration_ms: number };
};

function normalizeButton(button?: string | number): string {
  if (button === undefined || button === null) return 'left';
  if (typeof button === 'number') return button === 2 ? 'middle' : button === 3 ? 'right' : 'left';
  return button;
}

function translateCuaAction(action: CuaAction): BatchAction[] {
  switch (action.type) {
    case 'click': {
      if (action.button === 'back') return [{ type: 'press_key', press_key: { hold_keys: ['Alt_L'], keys: ['Left'] } }];
      if (action.button === 'forward') return [{ type: 'press_key', press_key: { hold_keys: ['Alt_L'], keys: ['Right'] } }];
      if (action.button === 'wheel') {
        return [{ type: 'scroll', scroll: { x: action.x ?? 0, y: action.y ?? 0, delta_x: action.scroll_x ?? 0, delta_y: action.scroll_y ?? 0 } }];
      }
      return [{ type: 'click_mouse', click_mouse: { x: action.x ?? 0, y: action.y ?? 0, button: normalizeButton(action.button) } }];
    }
    case 'double_click':
      return [{ type: 'click_mouse', click_mouse: { x: action.x ?? 0, y: action.y ?? 0, num_clicks: 2 } }];
    case 'type':
      return [{ type: 'type_text', type_text: { text: action.text ?? '' } }];
    case 'keypress': {
      const n = expandAndTranslateKeys(action.keys ?? [], action.hold_keys ?? []);
      return [{ type: 'press_key', press_key: { keys: n.keys, ...(n.holdKeys.length ? { hold_keys: n.holdKeys } : {}) } }];
    }
    case 'scroll':
      return [{ type: 'scroll', scroll: { x: action.x ?? 0, y: action.y ?? 0, delta_x: action.scroll_x ?? 0, delta_y: action.scroll_y ?? 0 } }];
    case 'move':
      return [{ type: 'move_mouse', move_mouse: { x: action.x ?? 0, y: action.y ?? 0 } }];
    case 'drag': {
      const points = (action.path ?? []).map(p => [p.x, p.y]);
      if (points.length < 2) throw new Error('drag requires at least 2 path points');
      return [{ type: 'drag_mouse', drag_mouse: { path: points } }];
    }
    case 'wait':
      return [{ type: 'sleep', sleep: { duration_ms: action.ms ?? 1000 } }];
    case 'goto':
      return [
        { type: 'press_key', press_key: { keys: ['l'], hold_keys: ['Control_L'] } },
        { type: 'sleep', sleep: { duration_ms: 200 } },
        { type: 'press_key', press_key: { keys: ['a'], hold_keys: ['Control_L'] } },
        { type: 'type_text', type_text: { text: action.url ?? '' } },
        { type: 'press_key', press_key: { keys: ['Return'] } },
      ];
    case 'back':
      return [{ type: 'press_key', press_key: { keys: ['Left'], hold_keys: ['Alt_L'] } }];
    case 'screenshot':
      return [];
    default:
      throw new Error(`Unknown CUA action: ${action.type}`);
  }
}

export class OpenAIProvider implements CuaProvider {
  readonly name = 'openai';
  private apiKey: string;

  constructor() {
    this.apiKey = process.env.OPENAI_API_KEY ?? '';
  }

  isConfigured(): boolean {
    return this.apiKey.length > 0;
  }

  async runTask(options: TaskOptions): Promise<TaskResult> {
    const { query, kernel, sessionId } = options;
    const client = new OpenAI({ apiKey: this.apiKey });

    // Navigate to a neutral starting page
    await kernel.browsers.computer.batch(sessionId, {
      actions: translateCuaAction({ type: 'goto', url: 'https://duckduckgo.com' }) as Parameters<typeof kernel.browsers.computer.batch>[1]['actions'],
    });

    const input: ResponseInputItem[] = [
      {
        role: 'system',
        content: `Current date: ${new Date().toISOString()}. For long pages, prefer PageUp/PageDown style scrolling over repeated mouse-wheel scrolling. Use wheel scrolling mainly for small adjustments.`,
      } as unknown as ResponseInputItem,
      {
        type: 'message',
        role: 'user',
        content: [{ type: 'input_text', text: query }],
      },
    ];

    const items: ResponseItem[] = [];
    const maxTurns = 50;

    for (let turn = 0; turn < maxTurns; turn++) {
      const response = await client.responses.create({
        model: options.model || 'gpt-5.4',
        input: [...input, ...items] as ResponseInputItem[],
        tools: [{ type: 'computer' } as unknown as OpenAI.Responses.Tool],
        truncation: 'auto',
        reasoning: { effort: 'low', summary: 'concise' },
      });

      if (!response.output) throw new Error('No output from model');

      for (const item of response.output as ResponseItem[]) {
        items.push(item);

        if (item.type === 'computer_call') {
          const cc = item as ResponseComputerToolCall & {
            action?: CuaAction;
            actions?: CuaAction[];
          };
          const actionList: CuaAction[] = Array.isArray(cc.actions)
            ? cc.actions
            : cc.action ? [cc.action] : [];

          // Execute actions
          const batch: BatchAction[] = [];
          for (const a of actionList) {
            batch.push(...translateCuaAction(a));
          }
          if (batch.length > 0) {
            await kernel.browsers.computer.batch(sessionId, {
              actions: batch as Parameters<typeof kernel.browsers.computer.batch>[1]['actions'],
            });
          }

          // Acknowledge safety checks
          const pending = cc.pending_safety_checks ?? [];
          for (const check of pending) {
            console.log(`Safety check: ${check.message ?? ''}`);
          }

          // Take screenshot
          await new Promise(r => setTimeout(r, 300));
          const screenshotResp = await kernel.browsers.computer.captureScreenshot(sessionId);
          const buf = Buffer.from(await screenshotResp.arrayBuffer());
          const screenshot = buf.toString('base64');

          items.push({
            type: 'computer_call_output',
            call_id: cc.call_id,
            acknowledged_safety_checks: pending,
            output: {
              type: 'computer_screenshot',
              image_url: `data:image/png;base64,${screenshot}`,
            },
          } as unknown as ResponseItem);
        }
      }

      // Check if the model produced a final assistant message
      const lastItem = response.output[response.output.length - 1] as ResponseItem & { role?: string };
      if (lastItem?.role === 'assistant') {
        const msg = lastItem as ResponseOutputMessage;
        const text = msg.content
          ?.filter(c => c && 'text' in c)
          .map(c => (c as { text: string }).text)
          .join('') ?? '';
        return { result: text || '(no response)', provider: this.name };
      }
    }

    return { result: '(max turns reached)', provider: this.name };
  }
}
