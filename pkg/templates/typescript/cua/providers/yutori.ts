/**
 * Yutori CUA provider adapter.
 *
 * Uses Yutori's n1-latest model via an OpenAI-compatible API with tool_calls.
 * Coordinates are returned in 1000x1000 space and scaled to viewport dimensions.
 * Screenshots are converted to WebP for better compression.
 *
 * @see https://docs.yutori.com/reference/n1
 */

import OpenAI from 'openai';
import sharp from 'sharp';
import type { CuaProvider, TaskOptions, TaskResult } from './index';

const DEFAULT_MODEL = 'n1-latest';
const TYPING_DELAY_MS = 12;
const SCREENSHOT_DELAY_MS = 300;
const ACTION_DELAY_MS = 300;

const KEY_MAP: Record<string, string> = {
  'Enter': 'Return', 'Escape': 'Escape', 'Backspace': 'BackSpace',
  'Tab': 'Tab', 'Delete': 'Delete',
  'ArrowUp': 'Up', 'ArrowDown': 'Down', 'ArrowLeft': 'Left', 'ArrowRight': 'Right',
  'Home': 'Home', 'End': 'End', 'PageUp': 'Page_Up', 'PageDown': 'Page_Down',
  ...Object.fromEntries(Array.from({ length: 12 }, (_, i) => [`F${i + 1}`, `F${i + 1}`])),
};

const MODIFIER_MAP: Record<string, string> = {
  control: 'ctrl', ctrl: 'ctrl', alt: 'alt', shift: 'shift',
  meta: 'super', command: 'super', cmd: 'super',
};

function mapKey(key: string): string {
  if (key.includes('+')) {
    return key.split('+').map(part => {
      const trimmed = part.trim();
      const lower = trimmed.toLowerCase();
      return MODIFIER_MAP[lower] ?? KEY_MAP[trimmed] ?? trimmed;
    }).join('+');
  }
  return KEY_MAP[key] ?? key;
}

type N1ActionType =
  | 'left_click' | 'double_click' | 'triple_click' | 'right_click'
  | 'scroll' | 'type' | 'key_press' | 'hover' | 'drag'
  | 'wait' | 'refresh' | 'go_back' | 'goto_url';

interface N1Action {
  action_type: N1ActionType;
  coordinates?: [number, number];
  start_coordinates?: [number, number];
  direction?: 'up' | 'down' | 'left' | 'right';
  amount?: number;
  text?: string;
  press_enter_after?: boolean;
  clear_before_typing?: boolean;
  key_comb?: string;
  url?: string;
}

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

export class YutoriProvider implements CuaProvider {
  readonly name = 'yutori';
  private apiKey: string;

  constructor() {
    this.apiKey = process.env.YUTORI_API_KEY ?? '';
  }

  isConfigured(): boolean {
    return this.apiKey.length > 0;
  }

  async runTask(options: TaskOptions): Promise<TaskResult> {
    const { query, kernel, sessionId, viewportWidth = 1280, viewportHeight = 800 } = options;
    const model = options.model || DEFAULT_MODEL;
    const client = new OpenAI({ apiKey: this.apiKey, baseURL: 'https://api.yutori.com/v1' });
    const computer = kernel.browsers.computer;
    const maxIterations = 50;

    const captureScreenshot = async (): Promise<string> => {
      const res = await computer.captureScreenshot(sessionId);
      const buf = Buffer.from(await res.arrayBuffer());
      const webpBuf = await sharp(buf).webp({ quality: 80 }).toBuffer();
      return webpBuf.toString('base64');
    };

    const scaleCoords = (coords: [number, number]): [number, number] => [
      Math.round((coords[0] / 1000) * viewportWidth),
      Math.round((coords[1] / 1000) * viewportHeight),
    ];

    const getCoords = (coords?: [number, number]): { x: number; y: number } => {
      if (!coords || coords.length !== 2) return { x: viewportWidth / 2, y: viewportHeight / 2 };
      return { x: coords[0], y: coords[1] };
    };

    const executeAction = async (action: N1Action): Promise<string | undefined> => {
      switch (action.action_type) {
        case 'left_click':
        case 'double_click':
        case 'triple_click':
        case 'right_click': {
          const { x, y } = getCoords(action.coordinates);
          const button = action.action_type === 'right_click' ? 'right' : 'left';
          const numClicks = action.action_type === 'double_click' ? 2
            : action.action_type === 'triple_click' ? 3 : 1;
          await computer.clickMouse(sessionId, { x, y, button, click_type: 'click', num_clicks: numClicks });
          await sleep(SCREENSHOT_DELAY_MS);
          return captureScreenshot();
        }
        case 'scroll': {
          const { x, y } = getCoords(action.coordinates);
          const notches = Math.max(action.amount ?? 3, 1);
          let delta_x = 0, delta_y = 0;
          if (action.direction === 'up') delta_y = -notches;
          else if (action.direction === 'down') delta_y = notches;
          else if (action.direction === 'left') delta_x = -notches;
          else if (action.direction === 'right') delta_x = notches;
          await computer.scroll(sessionId, { x, y, delta_x, delta_y });
          await sleep(SCREENSHOT_DELAY_MS);
          return captureScreenshot();
        }
        case 'type': {
          if (!action.text) throw new Error('text is required for type action');
          if (action.clear_before_typing) {
            await computer.pressKey(sessionId, { keys: ['ctrl+a'] });
            await sleep(100);
            await computer.pressKey(sessionId, { keys: ['BackSpace'] });
            await sleep(100);
          }
          await computer.typeText(sessionId, { text: action.text, delay: TYPING_DELAY_MS });
          if (action.press_enter_after) {
            await sleep(100);
            await computer.pressKey(sessionId, { keys: ['Return'] });
          }
          await sleep(SCREENSHOT_DELAY_MS);
          return captureScreenshot();
        }
        case 'key_press': {
          if (!action.key_comb) throw new Error('key_comb is required for key_press action');
          await computer.pressKey(sessionId, { keys: [mapKey(action.key_comb)] });
          await sleep(SCREENSHOT_DELAY_MS);
          return captureScreenshot();
        }
        case 'hover': {
          const { x, y } = getCoords(action.coordinates);
          await computer.moveMouse(sessionId, { x, y });
          await sleep(SCREENSHOT_DELAY_MS);
          return captureScreenshot();
        }
        case 'drag': {
          const start = getCoords(action.start_coordinates);
          const end = getCoords(action.coordinates);
          await computer.dragMouse(sessionId, {
            path: [[start.x, start.y], [end.x, end.y]], button: 'left',
          });
          await sleep(SCREENSHOT_DELAY_MS);
          return captureScreenshot();
        }
        case 'wait':
          await sleep(2000);
          return captureScreenshot();
        case 'refresh':
          await computer.pressKey(sessionId, { keys: ['F5'] });
          await sleep(2000);
          return captureScreenshot();
        case 'go_back':
          await computer.pressKey(sessionId, { keys: ['alt+Left'] });
          await sleep(1500);
          return captureScreenshot();
        case 'goto_url': {
          if (!action.url) throw new Error('url is required for goto_url action');
          await computer.pressKey(sessionId, { keys: ['ctrl+l'] });
          await sleep(ACTION_DELAY_MS);
          await computer.pressKey(sessionId, { keys: ['ctrl+a'] });
          await sleep(100);
          await computer.typeText(sessionId, { text: action.url, delay: TYPING_DELAY_MS });
          await sleep(ACTION_DELAY_MS);
          await computer.pressKey(sessionId, { keys: ['Return'] });
          await sleep(2000);
          return captureScreenshot();
        }
        default:
          throw new Error(`Unknown action type: ${action.action_type}`);
      }
    };

    // Take initial screenshot
    const initialScreenshot = await captureScreenshot();
    const conversationMessages: OpenAI.ChatCompletionMessageParam[] = [
      {
        role: 'user',
        content: [
          { type: 'text', text: query },
          { type: 'image_url', image_url: { url: `data:image/webp;base64,${initialScreenshot}` } },
        ],
      },
    ];

    for (let iteration = 0; iteration < maxIterations; iteration++) {
      const response = await client.chat.completions.create({
        model,
        messages: conversationMessages,
        max_completion_tokens: 4096,
        temperature: 0.3,
      });

      const choice = response.choices[0];
      if (!choice?.message) throw new Error('No response from model');

      conversationMessages.push(choice.message);

      const toolCalls = choice.message.tool_calls;
      if (!toolCalls || toolCalls.length === 0) {
        return { result: choice.message.content || '(no response)', provider: this.name };
      }

      for (const toolCall of toolCalls) {
        let args: Record<string, unknown>;
        try {
          args = JSON.parse(toolCall.function.arguments);
        } catch {
          conversationMessages.push({
            role: 'tool', tool_call_id: toolCall.id,
            content: 'Error: failed to parse arguments',
          });
          continue;
        }

        const action: N1Action = { action_type: toolCall.function.name as N1ActionType, ...args };

        // Scale coordinates from 1000x1000 to viewport
        if (action.coordinates) {
          action.coordinates = scaleCoords(action.coordinates);
        }
        if (action.start_coordinates) {
          action.start_coordinates = scaleCoords(action.start_coordinates);
        }

        try {
          const screenshot = await executeAction(action);
          if (screenshot) {
            conversationMessages.push({
              role: 'tool', tool_call_id: toolCall.id,
              content: [{
                type: 'image_url',
                image_url: { url: `data:image/webp;base64,${screenshot}` },
              }] as unknown as string,
            });
          } else {
            conversationMessages.push({
              role: 'tool', tool_call_id: toolCall.id, content: 'OK',
            });
          }
        } catch (error) {
          conversationMessages.push({
            role: 'tool', tool_call_id: toolCall.id,
            content: `Action failed: ${error instanceof Error ? error.message : String(error)}`,
          });
        }
      }
    }

    return { result: '(max iterations reached)', provider: this.name };
  }
}
