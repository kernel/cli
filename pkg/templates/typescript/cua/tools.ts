/**
 * Unified tool mapping layer.
 *
 * Normalizes provider-specific action formats into Kernel Computer Controls API calls.
 * Each provider adapter converts its native actions into CommonAction[], then this
 * module executes them against the Kernel API.
 */

import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';

// ---------- Common action types (provider-agnostic) ----------

export interface CommonAction {
  type: 'click' | 'double_click' | 'triple_click' | 'right_click' | 'middle_click'
    | 'mouse_move' | 'mouse_down' | 'mouse_up'
    | 'type' | 'key' | 'scroll' | 'drag' | 'wait' | 'screenshot'
    | 'goto' | 'back';
  x?: number;
  y?: number;
  text?: string;
  keys?: string;
  url?: string;
  scrollX?: number;
  scrollY?: number;
  duration?: number;
  startX?: number;
  startY?: number;
  endX?: number;
  endY?: number;
  path?: Array<[number, number]>;
}

export interface ToolResult {
  output?: string;
  error?: string;
  base64Image?: string;
}

// ---------- Key mappings (provider key names -> X11 keysym for Kernel API) ----------

const KEY_MAP: Record<string, string> = {
  'return': 'Return', 'enter': 'Return', 'Enter': 'Return', 'ENTER': 'Return',
  'left': 'Left', 'right': 'Right', 'up': 'Up', 'down': 'Down',
  'ArrowLeft': 'Left', 'ArrowRight': 'Right', 'ArrowUp': 'Up', 'ArrowDown': 'Down',
  'LEFT': 'Left', 'RIGHT': 'Right', 'UP': 'Up', 'DOWN': 'Down',
  'home': 'Home', 'end': 'End', 'Home': 'Home', 'End': 'End', 'HOME': 'Home', 'END': 'End',
  'pageup': 'Page_Up', 'page_up': 'Page_Up', 'PageUp': 'Page_Up', 'PAGEUP': 'Prior', 'PAGE_UP': 'Prior',
  'pagedown': 'Page_Down', 'page_down': 'Page_Down', 'PageDown': 'Page_Down', 'PAGEDOWN': 'Next', 'PAGE_DOWN': 'Next',
  'delete': 'Delete', 'Delete': 'Delete', 'DELETE': 'Delete',
  'backspace': 'BackSpace', 'Backspace': 'BackSpace', 'BACKSPACE': 'BackSpace',
  'tab': 'Tab', 'Tab': 'Tab', 'TAB': 'Tab',
  'insert': 'Insert', 'Insert': 'Insert', 'INSERT': 'Insert',
  'esc': 'Escape', 'escape': 'Escape', 'Escape': 'Escape', 'ESC': 'Escape', 'ESCAPE': 'Escape',
  'space': 'space', 'Space': 'space', 'SPACE': 'space',
  'caps_lock': 'Caps_Lock', 'CapsLock': 'Caps_Lock', 'CAPS_LOCK': 'Caps_Lock',
  'f1': 'F1', 'f2': 'F2', 'f3': 'F3', 'f4': 'F4', 'f5': 'F5', 'f6': 'F6',
  'f7': 'F7', 'f8': 'F8', 'f9': 'F9', 'f10': 'F10', 'f11': 'F11', 'f12': 'F12',
  'F1': 'F1', 'F2': 'F2', 'F3': 'F3', 'F4': 'F4', 'F5': 'F5', 'F6': 'F6',
  'F7': 'F7', 'F8': 'F8', 'F9': 'F9', 'F10': 'F10', 'F11': 'F11', 'F12': 'F12',
};

const MODIFIER_MAP: Record<string, string> = {
  'ctrl': 'ctrl', 'control': 'ctrl', 'Control': 'ctrl', 'Ctrl': 'ctrl', 'CTRL': 'ctrl', 'CONTROL': 'ctrl',
  'alt': 'alt', 'Alt': 'alt', 'ALT': 'alt',
  'shift': 'shift', 'Shift': 'shift', 'SHIFT': 'shift',
  'meta': 'super', 'Meta': 'super', 'cmd': 'super', 'command': 'super',
  'win': 'super', 'super': 'super', 'Super': 'super',
  'CMD': 'super', 'COMMAND': 'super', 'META': 'super', 'SUPER': 'super',
};

export function translateKey(key: string): string {
  return MODIFIER_MAP[key] || KEY_MAP[key] || key;
}

export function translateKeyCombination(combo: string): string {
  if (!combo.includes('+')) return translateKey(combo);
  return combo.split('+').map(part => translateKey(part.trim())).join('+');
}

// ---------- Kernel Computer Controls executor ----------

const TYPING_DELAY_MS = 12;
const POST_ACTION_DELAY_MS = 500;
const SCREENSHOT_DELAY_MS = 2000;

export class KernelExecutor {
  constructor(
    private kernel: Kernel,
    private sessionId: string,
  ) {}

  async execute(action: CommonAction): Promise<ToolResult> {
    switch (action.type) {
      case 'screenshot':
        return this.screenshot();

      case 'click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0, button: 'left', click_type: 'click', num_clicks: 1,
        });
        return this.delayAndScreenshot();

      case 'double_click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0, button: 'left', click_type: 'click', num_clicks: 2,
        });
        return this.delayAndScreenshot();

      case 'triple_click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0, button: 'left', click_type: 'click', num_clicks: 3,
        });
        return this.delayAndScreenshot();

      case 'right_click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0, button: 'right', click_type: 'click',
        });
        return this.delayAndScreenshot();

      case 'middle_click':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0, button: 'middle', click_type: 'click',
        });
        return this.delayAndScreenshot();

      case 'mouse_move':
        await this.kernel.browsers.computer.moveMouse(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0,
        });
        return this.delayAndScreenshot();

      case 'mouse_down':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0, button: 'left', click_type: 'down',
        });
        return this.delayAndScreenshot();

      case 'mouse_up':
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0, button: 'left', click_type: 'up',
        });
        return this.delayAndScreenshot();

      case 'type':
        await this.kernel.browsers.computer.typeText(this.sessionId, {
          text: action.text ?? '', delay: TYPING_DELAY_MS,
        });
        return this.delayAndScreenshot();

      case 'key': {
        const key = translateKeyCombination(action.keys ?? action.text ?? '');
        await this.kernel.browsers.computer.pressKey(this.sessionId, { keys: [key] });
        return this.delayAndScreenshot();
      }

      case 'scroll': {
        const deltaX = action.scrollX ?? 0;
        const deltaY = action.scrollY ?? 0;
        await this.kernel.browsers.computer.scroll(this.sessionId, {
          x: action.x ?? 0, y: action.y ?? 0, delta_x: deltaX, delta_y: deltaY,
        });
        const result = await this.delayAndScreenshot();
        result.output = `Scrolled (dx=${deltaX}, dy=${deltaY}).`;
        return result;
      }

      case 'drag': {
        const path = action.path ?? (
          action.startX !== undefined && action.endX !== undefined
            ? [[action.startX, action.startY ?? 0], [action.endX, action.endY ?? 0]]
            : [[0, 0], [action.x ?? 0, action.y ?? 0]]
        );
        await this.kernel.browsers.computer.dragMouse(this.sessionId, { path, button: 'left' });
        return this.delayAndScreenshot();
      }

      case 'wait':
        await this.sleep(action.duration ?? 1000);
        return this.screenshot();

      case 'goto':
        // Focus URL bar, select all, type URL, press Enter
        await this.kernel.browsers.computer.pressKey(this.sessionId, { keys: ['ctrl+l'] });
        await this.sleep(200);
        await this.kernel.browsers.computer.pressKey(this.sessionId, { keys: ['ctrl+a'] });
        await this.kernel.browsers.computer.typeText(this.sessionId, { text: action.url ?? '' });
        await this.kernel.browsers.computer.pressKey(this.sessionId, { keys: ['Return'] });
        await this.sleep(1500);
        return this.screenshot();

      case 'back':
        await this.kernel.browsers.computer.pressKey(this.sessionId, { keys: ['alt+Left'] });
        await this.sleep(1000);
        return this.screenshot();

      default:
        return { error: `Unknown action type: ${action.type}` };
    }
  }

  async screenshot(): Promise<ToolResult> {
    await this.sleep(SCREENSHOT_DELAY_MS);
    const response = await this.kernel.browsers.computer.captureScreenshot(this.sessionId);
    const blob = await response.blob();
    const buffer = Buffer.from(await blob.arrayBuffer());
    return { base64Image: buffer.toString('base64') };
  }

  private async delayAndScreenshot(): Promise<ToolResult> {
    await this.sleep(POST_ACTION_DELAY_MS);
    return this.screenshot();
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
