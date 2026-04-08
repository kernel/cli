/**
 * Gemini CUA provider.
 *
 * Uses Google's Gemini computer use API with native browser environment.
 * Coordinates are normalized (0-1000 scale) by the Gemini API.
 */

import { GoogleGenAI, Environment, type Content, type FunctionCall, type Part } from '@google/genai';
import type { KernelExecutor, CommonAction } from '../tools';
import type { CUAProvider, ProviderConfig, ProviderResult } from './index';

const COORDINATE_SCALE = 1000;

function getSystemPrompt(): string {
  const currentDate = new Date().toLocaleDateString('en-US', {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric',
  });
  return `You are a helpful assistant that can use a web browser.
You are operating a Chrome browser through computer use tools.
The browser is already open and ready for use.

When you need to navigate to a page, use the navigate action with a full URL.
When you need to interact with elements, use click_at, type_text_at, etc.
After each action, carefully evaluate the screenshot to determine your next step.

Current date: ${currentDate}.`;
}

const MAX_RECENT_SCREENSHOTS = 3;
const PX_PER_NOTCH = 60;
const MAX_NOTCHES = 17;

// Map Gemini function calls to CommonAction
function toCommonAction(
  name: string,
  args: Record<string, unknown>,
  viewportWidth: number,
  viewportHeight: number,
): CommonAction {
  const denormX = (x: number) => Math.round((x / COORDINATE_SCALE) * viewportWidth);
  const denormY = (y: number) => Math.round((y / COORDINATE_SCALE) * viewportHeight);

  switch (name) {
    case 'open_web_browser': return { type: 'screenshot' };
    case 'click_at': return { type: 'click', x: denormX(args.x as number), y: denormY(args.y as number) };
    case 'hover_at': return { type: 'mouse_move', x: denormX(args.x as number), y: denormY(args.y as number) };
    case 'type_text_at': {
      // Gemini type_text_at: click at coords, optionally clear, type, optionally press enter
      // We return the primary action; the provider handles the full sequence
      return {
        type: 'click', // Will be handled specially in the loop
        x: denormX(args.x as number),
        y: denormY(args.y as number),
        text: args.text as string,
      };
    }
    case 'scroll_document':
    case 'scroll_at': {
      const dir = args.direction as string;
      const mag = (args.magnitude as number) ?? 400;
      const notches = Math.min(MAX_NOTCHES, Math.max(1, Math.round(mag / PX_PER_NOTCH)));
      let scrollX = 0, scrollY = 0;
      if (dir === 'down') scrollY = notches;
      else if (dir === 'up') scrollY = -notches;
      else if (dir === 'right') scrollX = notches;
      else if (dir === 'left') scrollX = -notches;
      const x = name === 'scroll_at' ? denormX(args.x as number) : Math.round(viewportWidth / 2);
      const y = name === 'scroll_at' ? denormY(args.y as number) : Math.round(viewportHeight / 2);
      return { type: 'scroll', x, y, scrollX, scrollY };
    }
    case 'wait_5_seconds': return { type: 'wait', duration: 5000 };
    case 'go_back': return { type: 'back' };
    case 'go_forward': return { type: 'key', keys: 'alt+Right' };
    case 'search': return { type: 'key', keys: 'ctrl+l' };
    case 'navigate': return { type: 'goto', url: args.url as string };
    case 'key_combination': return { type: 'key', keys: args.keys as string };
    case 'drag_and_drop': return {
      type: 'drag',
      startX: denormX(args.x as number), startY: denormY(args.y as number),
      endX: denormX(args.destination_x as number), endY: denormY(args.destination_y as number),
    };
    default: return { type: 'screenshot' };
  }
}

const PREDEFINED_FUNCTIONS = new Set([
  'open_web_browser', 'click_at', 'hover_at', 'type_text_at',
  'scroll_document', 'scroll_at', 'wait_5_seconds', 'go_back', 'go_forward',
  'search', 'navigate', 'key_combination', 'drag_and_drop',
]);

function pruneOldScreenshots(contents: Content[]): void {
  let count = 0;
  for (let i = contents.length - 1; i >= 0; i--) {
    const c = contents[i];
    if (!c || c.role !== 'user' || !c.parts) continue;
    const hasScreenshot = c.parts.some(p =>
      'functionResponse' in p && p.functionResponse &&
      PREDEFINED_FUNCTIONS.has(p.functionResponse.name || '') &&
      (p.functionResponse as { parts?: unknown[] }).parts?.length
    );
    if (hasScreenshot) {
      count++;
      if (count > MAX_RECENT_SCREENSHOTS) {
        for (const p of c.parts) {
          if ('functionResponse' in p && p.functionResponse && PREDEFINED_FUNCTIONS.has(p.functionResponse.name || '')) {
            delete (p.functionResponse as { parts?: unknown }).parts;
          }
        }
      }
    }
  }
}

export class GeminiProvider implements CUAProvider {
  name = 'gemini' as const;

  async run(config: ProviderConfig, executor: KernelExecutor): Promise<ProviderResult> {
    const apiKey = config.apiKey || process.env.GOOGLE_API_KEY;
    if (!apiKey) throw new Error('GOOGLE_API_KEY is required for Gemini provider');

    const model = config.model || 'gemini-2.5-computer-use-preview-10-2025';
    const ai = new GoogleGenAI({ apiKey });
    const viewportWidth = config.viewportWidth ?? 1200;
    const viewportHeight = config.viewportHeight ?? 800;

    const contents: Content[] = [
      { role: 'user', parts: [{ text: config.query }] },
    ];

    const maxIterations = 50;
    for (let i = 0; i < maxIterations; i++) {
      console.log(`[gemini] iteration ${i + 1}`);

      const response = await ai.models.generateContent({
        model,
        contents,
        config: {
          temperature: 1, topP: 0.95, topK: 40, maxOutputTokens: 8192,
          systemInstruction: getSystemPrompt(),
          tools: [{ computerUse: { environment: Environment.ENVIRONMENT_BROWSER } }],
          thinkingConfig: { includeThoughts: true },
        },
      });

      if (!response.candidates?.[0]?.content) break;

      const candidate = response.candidates[0];
      contents.push(candidate.content);

      // Extract text and function calls
      const textParts = (candidate.content.parts ?? []).filter(p => 'text' in p && p.text);
      const functionCalls = (candidate.content.parts ?? []).filter(p => 'functionCall' in p && p.functionCall).map(p => p.functionCall as FunctionCall);

      if (functionCalls.length === 0) {
        const text = textParts.map(p => 'text' in p ? p.text : '').join(' ');
        return { result: text || 'Task completed.', provider: 'gemini' };
      }

      // Execute function calls
      const functionResponses: Part[] = [];
      for (const fc of functionCalls) {
        if (!fc.name) continue;
        const args = (fc.args as Record<string, unknown>) || {};
        console.log(`[gemini] action: ${fc.name}`);

        try {
          // Special handling for type_text_at (multi-step action)
          if (fc.name === 'type_text_at') {
            const common = toCommonAction(fc.name, args, viewportWidth, viewportHeight);
            await executor.execute({ type: 'click', x: common.x, y: common.y });
            if (args.clear_before_typing !== false) {
              await executor.execute({ type: 'key', keys: 'ctrl+a' });
            }
            await executor.execute({ type: 'type', text: common.text ?? '' });
            if (args.press_enter) {
              await executor.execute({ type: 'key', keys: 'Return' });
            }
            const screenshot = await executor.screenshot();
            functionResponses.push({
              functionResponse: {
                name: fc.name,
                response: { url: 'about:blank' },
                ...(screenshot.base64Image ? {
                  parts: [{ inlineData: { mimeType: 'image/png', data: screenshot.base64Image } }],
                } : {}),
              },
            } as Part);
          } else {
            const common = toCommonAction(fc.name, args, viewportWidth, viewportHeight);
            const result = await executor.execute(common);
            functionResponses.push({
              functionResponse: {
                name: fc.name,
                response: { url: result.output || 'about:blank' },
                ...(result.base64Image && PREDEFINED_FUNCTIONS.has(fc.name) ? {
                  parts: [{ inlineData: { mimeType: 'image/png', data: result.base64Image } }],
                } : {}),
              },
            } as Part);
          }
        } catch (error) {
          functionResponses.push({
            functionResponse: {
              name: fc.name,
              response: { error: String(error), url: 'about:blank' },
            },
          } as Part);
        }
      }

      contents.push({ role: 'user', parts: functionResponses });
      pruneOldScreenshots(contents);
    }

    return { result: 'Max iterations reached.', provider: 'gemini' };
  }
}
