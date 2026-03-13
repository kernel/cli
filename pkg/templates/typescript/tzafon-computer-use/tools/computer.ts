/**
 * Tzafon Northstar Computer Tool
 * 
 * Maps Tzafon Northstar CUA action format to Kernel's Computer Controls API.
 * Northstar returns actions via computer_call outputs with properties like
 * .type, .x, .y, .text, .keys, .scroll_x, .scroll_y, .url, etc.
 */

import type { Kernel } from '@onkernel/sdk';

export class ToolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ToolError';
  }
}

const MODIFIER_MAP: Record<string, string> = {
  Control: 'Ctrl',
  Enter: 'Return',
};

const MODIFIER_NAMES = new Set(['Ctrl', 'Shift', 'Alt', 'Meta', 'Super']);

function clamp(val: number, max: number): number {
  return Math.max(0, Math.min(Math.round(val), max - 1));
}

function formatKeys(keys: string[]): string[] {
  const modifiers: string[] = [];
  const regular: string[] = [];

  for (const key of keys) {
    const mapped = MODIFIER_MAP[key] ?? key;
    if (MODIFIER_NAMES.has(mapped)) {
      modifiers.push(mapped);
    } else {
      regular.push(mapped);
    }
  }

  if (regular.length === 0 && modifiers.length > 0) {
    return [modifiers.join('+')];
  }
  return regular.map((k) => [...modifiers, k].join('+'));
}

export class ComputerTool {
  private kernel: Kernel;
  private sessionId: string;
  private width: number;
  private height: number;

  constructor(kernel: Kernel, sessionId: string, width = 1280, height = 800) {
    this.kernel = kernel;
    this.sessionId = sessionId;
    this.width = width;
    this.height = height;
  }

  private x(action: any): number {
    return clamp(action.x, this.width);
  }

  private y(action: any): number {
    return clamp(action.y, this.height);
  }

  private get cx(): number {
    return this.width / 2;
  }

  private get cy(): number {
    return this.height / 2;
  }

  async execute(action: any): Promise<void> {
    switch (action.type) {
      case 'click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: this.x(action), y: this.y(action),
        });
        break;

      case 'double_click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: this.x(action), y: this.y(action), num_clicks: 2,
        });
        break;

      case 'triple_click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: this.x(action), y: this.y(action), num_clicks: 3,
        });
        break;

      case 'right_click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: this.x(action), y: this.y(action), button: 'right',
        });
        break;

      case 'type':
        await this.kernel.browsers.computer.typeText(this.sessionId, {
          text: action.text,
        });
        break;

      case 'key':
      case 'keypress':
        await this.kernel.browsers.computer.pressKey(this.sessionId, {
          keys: formatKeys(action.keys),
        });
        break;

      case 'key_down':
        await this.kernel.browsers.computer.pressKey(this.sessionId, {
          keys: formatKeys(action.keys),
          duration: 5000,
        });
        break;

      case 'key_up':
        break;

      case 'scroll':
        await this.kernel.browsers.computer.scroll(this.sessionId, {
          x: clamp(action.x ?? this.cx, this.width),
          y: clamp(action.y ?? this.cy, this.height),
          delta_x: action.scroll_x ?? 0,
          delta_y: action.scroll_y ?? 0,
        });
        break;

      case 'hscroll':
        await this.kernel.browsers.computer.scroll(this.sessionId, {
          x: clamp(action.x ?? this.cx, this.width),
          y: clamp(action.y ?? this.cy, this.height),
          delta_x: action.scroll_x ?? 0,
        });
        break;

      case 'navigate':
        await this.kernel.browsers.playwright.execute(this.sessionId, {
          code: `await page.goto(${JSON.stringify(action.url)})`,
        });
        break;

      case 'drag':
        await this.kernel.browsers.computer.dragMouse(this.sessionId, {
          path: [
            [
              clamp(action.x ?? action.x1, this.width),
              clamp(action.y ?? action.y1, this.height),
            ],
            [
              clamp(action.end_x ?? action.x2, this.width),
              clamp(action.end_y ?? action.y2, this.height),
            ],
          ],
        });
        break;

      case 'wait':
        await new Promise((r) => setTimeout(r, 2000));
        break;

      default:
        throw new ToolError(`Unknown action type: ${action.type}`);
    }
  }

  async captureScreenshot(): Promise<string> {
    const res = await this.kernel.browsers.computer.captureScreenshot(this.sessionId);
    const buf = Buffer.from(await res.arrayBuffer());
    return `data:image/png;base64,${buf.toString('base64')}`;
  }
}
