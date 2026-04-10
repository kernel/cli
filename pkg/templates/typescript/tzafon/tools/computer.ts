/**
 * Tzafon Northstar Computer Tool
 *
 * Executes function tool calls from the Northstar model on the browser.
 * Coordinates arrive in a normalised 0-999 grid and are scaled to the
 * browser viewport before dispatch.
 */

import type { Kernel } from '@onkernel/sdk';

export class ToolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ToolError';
  }
}

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

  /** Parse a coordinate value. Handles the model's occasional '470,77' format. */
  private coord(val: unknown): number {
    if (val == null) return 0;
    let s = String(val);
    if (s.includes(',')) s = s.split(',')[0].trim();
    return Math.trunc(Number(s));
  }

  /** Convert 0-999 grid coordinates to pixel coordinates. */
  private scale(x: unknown, y: unknown): [number, number] {
    const cx = this.coord(x);
    const cy = this.coord(y);
    const px = Math.max(0, Math.min(Math.trunc(cx * (this.width - 1) / 999), this.width - 1));
    const py = Math.max(0, Math.min(Math.trunc(cy * (this.height - 1) / 999), this.height - 1));
    return [px, py];
  }

  async executeFunction(name: string, args: Record<string, any>): Promise<void> {
    switch (name) {
      case 'click': {
        const [px, py] = this.scale(args.x, args.y);
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: px, y: py, button: args.button ?? 'left',
        });
        break;
      }

      case 'double_click': {
        const [px, py] = this.scale(args.x, args.y);
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: px, y: py, num_clicks: 2,
        });
        break;
      }

      case 'point_and_type': {
        const [px, py] = this.scale(args.x, args.y);
        await this.kernel.browsers.computer.clickMouse(this.sessionId, { x: px, y: py });
        await sleep(300);
        await this.kernel.browsers.computer.typeText(this.sessionId, { text: args.text });
        if (args.press_enter) {
          await sleep(100);
          await this.kernel.browsers.computer.pressKey(this.sessionId, { keys: ['Return'] });
        }
        break;
      }

      case 'key': {
        await this.kernel.browsers.computer.pressKey(this.sessionId, {
          keys: [mapKey(args.keys)],
        });
        break;
      }

      case 'scroll': {
        const [px, py] = this.scale(args.x ?? 500, args.y ?? 500);
        const dy = Math.max(-10, Math.min(10, args.dy ?? 3));
        await this.kernel.browsers.computer.scroll(this.sessionId, {
          x: px, y: py, delta_x: 0, delta_y: dy,
        });
        break;
      }

      case 'drag': {
        const [px1, py1] = this.scale(args.x1, args.y1);
        const [px2, py2] = this.scale(args.x2, args.y2);
        await this.kernel.browsers.computer.dragMouse(this.sessionId, {
          path: [[px1, py1], [px2, py2]],
        });
        break;
      }

      default:
        throw new ToolError(`Unknown function: ${name}`);
    }
  }

  async captureScreenshot(): Promise<string> {
    const res = await this.kernel.browsers.computer.captureScreenshot(this.sessionId);
    const buf = Buffer.from(await res.arrayBuffer());
    return `data:image/png;base64,${buf.toString('base64')}`;
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
