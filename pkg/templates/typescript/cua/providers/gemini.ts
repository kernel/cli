/**
 * Gemini CUA provider adapter.
 *
 * Uses Google's GenAI SDK with the computer-use-preview model.
 */

import {
  GoogleGenAI,
  Environment,
  type Content,
  type FunctionCall,
  type Part,
} from '@google/genai';
import type { CuaProvider, TaskOptions, TaskResult } from './index';

// Gemini uses a 0-1000 coordinate scale that maps to actual screen pixels.
const COORDINATE_SCALE = 1000;
const DEFAULT_WIDTH = 1200;
const DEFAULT_HEIGHT = 800;

// Gemini reports scroll magnitude in pixels; computer.scroll expects wheel
// notches. Convert with a per-notch pixel budget and clamp to a sane max.
const PX_PER_NOTCH = 60;
const MAX_NOTCHES_PER_ACTION = 17;

const PREDEFINED_ACTIONS = [
  'click_at', 'hover_at', 'type_text_at', 'scroll_document',
  'scroll_at', 'wait_5_seconds', 'go_back', 'go_forward',
  'search', 'navigate', 'key_combination', 'drag_and_drop',
  'open_web_browser',
];

function getSystemPrompt(): string {
  const date = new Date().toLocaleDateString('en-US', {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric',
  });
  return `You are a helpful assistant that can use a web browser.
You are operating a Chrome browser through computer use tools.
The browser is already open and ready for use.
When you need to navigate to a page, use the navigate action with a full URL.
For long pages, prefer PageUp/PageDown style scrolling over repeated mouse-wheel scrolling. Use wheel scrolling mainly for small adjustments.
After each action, carefully evaluate the screenshot to determine your next step.
Current date: ${date}.`;
}

interface GeminiArgs {
  x?: number;
  y?: number;
  text?: string;
  url?: string;
  key_combination?: string;
  direction?: string;
  magnitude?: number;
  start_x?: number;
  start_y?: number;
  end_x?: number;
  end_y?: number;
  safety_decision?: { decision: string; explanation?: string };
  [key: string]: unknown;
}

export class GeminiProvider implements CuaProvider {
  readonly name = 'gemini';
  private apiKey: string;

  constructor() {
    this.apiKey = process.env.GOOGLE_API_KEY ?? '';
  }

  isConfigured(): boolean {
    return this.apiKey.length > 0;
  }

  async runTask(options: TaskOptions): Promise<TaskResult> {
    const { query, kernel, sessionId } = options;
    const width = options.viewportWidth ?? DEFAULT_WIDTH;
    const height = options.viewportHeight ?? DEFAULT_HEIGHT;
    const ai = new GoogleGenAI({ apiKey: this.apiKey });
    const model = options.model || 'gemini-2.5-computer-use-preview-10-2025';

    const contents: Content[] = [{ role: 'user', parts: [{ text: query }] }];
    const maxIterations = 50;

    for (let i = 0; i < maxIterations; i++) {
      const response = await ai.models.generateContent({
        model,
        contents,
        config: {
          temperature: 1,
          topP: 0.95,
          topK: 40,
          maxOutputTokens: 8192,
          systemInstruction: getSystemPrompt(),
          tools: [{ computerUse: { environment: Environment.ENVIRONMENT_BROWSER } }],
          thinkingConfig: { includeThoughts: true },
        },
      });

      const candidateContent = response.candidates?.[0]?.content;
      if (!candidateContent) break;
      contents.push(candidateContent);

      // Extract text and function calls
      const textParts = (candidateContent.parts ?? [])
        .filter(p => 'text' in p && p.text)
        .map(p => (p as { text: string }).text);
      const functionCalls = (candidateContent.parts ?? [])
        .filter(p => 'functionCall' in p)
        .map(p => (p as { functionCall: FunctionCall }).functionCall);

      if (functionCalls.length === 0) {
        return { result: textParts.join(' ') || '(no response)', provider: this.name };
      }

      // Execute function calls
      const responses: Part[] = [];
      for (const fc of functionCalls) {
        if (!fc.name) continue;
        const args = (fc.args ?? {}) as GeminiArgs;

        if (args.safety_decision?.decision === 'require_confirmation') {
          console.log('Safety check:', args.safety_decision.explanation);
        }

        const result = await this.executeAction(kernel, sessionId, fc.name, args, width, height);

        const responseData: Record<string, unknown> = { url: result.url || 'about:blank' };
        const part: Part = {
          functionResponse: {
            name: fc.name,
            response: result.error ? { error: result.error, url: 'about:blank' } : responseData,
            ...(result.screenshot && PREDEFINED_ACTIONS.includes(fc.name) ? {
              parts: [{ inlineData: { mimeType: 'image/png', data: result.screenshot } }],
            } : {}),
          },
        };
        responses.push(part);
      }

      contents.push({ role: 'user', parts: responses });
    }

    return { result: '(max iterations reached)', provider: this.name };
  }

  private denormalize(value: number | undefined, dimension: number): number {
    if (value === undefined) return 0;
    return Math.round((value / COORDINATE_SCALE) * dimension);
  }

  private async executeAction(
    kernel: TaskOptions['kernel'],
    sessionId: string,
    name: string,
    args: GeminiArgs,
    width: number,
    height: number,
  ): Promise<{ screenshot?: string; url?: string; error?: string }> {
    const computer = kernel.browsers.computer;

    try {
      switch (name) {
        case 'click_at': {
          const x = this.denormalize(args.x, width);
          const y = this.denormalize(args.y, height);
          await computer.clickMouse(sessionId, { x, y });
          break;
        }
        case 'hover_at': {
          const x = this.denormalize(args.x, width);
          const y = this.denormalize(args.y, height);
          await computer.moveMouse(sessionId, { x, y });
          break;
        }
        case 'type_text_at': {
          const x = this.denormalize(args.x, width);
          const y = this.denormalize(args.y, height);
          await computer.clickMouse(sessionId, { x, y });
          if (args.text) {
            await computer.typeText(sessionId, { text: args.text });
          }
          break;
        }
        case 'scroll_document':
        case 'scroll_at': {
          const x = name === 'scroll_at' ? this.denormalize(args.x, width) : width / 2;
          const y = name === 'scroll_at' ? this.denormalize(args.y, height) : height / 2;
          const magnitudePx = args.magnitude ?? 400;
          const notches = Math.min(
            MAX_NOTCHES_PER_ACTION,
            Math.max(1, Math.round(magnitudePx / PX_PER_NOTCH)),
          );
          const dir = args.direction ?? 'down';
          const deltaY = dir === 'up' ? -notches : dir === 'down' ? notches : 0;
          const deltaX = dir === 'left' ? -notches : dir === 'right' ? notches : 0;
          await computer.scroll(sessionId, { x, y, delta_x: deltaX, delta_y: deltaY });
          break;
        }
        case 'wait_5_seconds':
          await new Promise(r => setTimeout(r, 5000));
          break;
        case 'go_back':
          await computer.pressKey(sessionId, { keys: ['Left'], hold_keys: ['Alt_L'] });
          break;
        case 'go_forward':
          await computer.pressKey(sessionId, { keys: ['Right'], hold_keys: ['Alt_L'] });
          break;
        case 'navigate':
        case 'search': {
          const url = args.url ?? args.text ?? '';
          await computer.batch(sessionId, {
            actions: [
              { type: 'press_key', press_key: { keys: ['l'], hold_keys: ['Control_L'] } },
              { type: 'sleep', sleep: { duration_ms: 200 } },
              { type: 'press_key', press_key: { keys: ['a'], hold_keys: ['Control_L'] } },
              { type: 'type_text', type_text: { text: url } },
              { type: 'press_key', press_key: { keys: ['Return'] } },
            ] as Parameters<typeof computer.batch>[1]['actions'],
          });
          await new Promise(r => setTimeout(r, 1500));
          break;
        }
        case 'key_combination': {
          const combo = args.key_combination ?? '';
          const parts = combo.split('+').map(k => k.trim());
          const holdKeys = parts.slice(0, -1);
          const keys = parts.slice(-1);
          await computer.pressKey(sessionId, {
            keys: keys.length > 0 ? keys : parts,
            ...(holdKeys.length > 0 ? { hold_keys: holdKeys } : {}),
          });
          break;
        }
        case 'drag_and_drop': {
          const sx = this.denormalize(args.start_x, width);
          const sy = this.denormalize(args.start_y, height);
          const ex = this.denormalize(args.end_x, width);
          const ey = this.denormalize(args.end_y, height);
          await computer.dragMouse(sessionId, { path: [[sx, sy], [ex, ey]] });
          break;
        }
        case 'open_web_browser':
          break;
        default:
          return { error: `Unknown action: ${name}` };
      }

      // Take screenshot after action
      await new Promise(r => setTimeout(r, 500));
      const resp = await computer.captureScreenshot(sessionId);
      const buf = Buffer.from(await resp.arrayBuffer());
      return { screenshot: buf.toString('base64'), url: 'about:blank' };
    } catch (error) {
      return { error: error instanceof Error ? error.message : String(error) };
    }
  }
}
