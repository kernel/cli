/**
 * Yutori n1.5 Computer Tool
 *
 * Maps n1.5-latest action format to Kernel's Computer Controls API.
 * Screenshots are converted to WebP for better compression across multi-step trajectories.
 *
 * @see https://docs.yutori.com/reference/n1-5
 */

import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';
import sharp from 'sharp';

const TYPING_DELAY_MS = 12;
const SCREENSHOT_DELAY_MS = 150;
const ACTION_DELAY_MS = 300;

// n1.5 scroll `amount` is in "wheel units" where 1 unit ≈ 10% of the viewport
// height (~80px at 800px tall). Kernel's `delta_y` is a wheel-event repeat
// count where each tick is much smaller in practice, so we multiply.
const SCROLL_NOTCHES_PER_AMOUNT = 4;

// WebP quality for screenshots. Kernel returns PNGs, which are crisp and
// tolerate aggressive WebP compression with no visible degradation — matches
// Yutori SDK's DEFAULT_WEBP_QUALITY_FOR_PNG=30 (yutori-sdk-python/yutori/
// navigator/images.py). Lower values cut payload size substantially on long
// multi-step trajectories.
const WEBP_QUALITY = 30;

export interface ToolResult {
  base64Image?: string;
  output?: string;
  error?: string;
}

export class ToolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ToolError';
  }
}

export type N15ActionType =
  | 'left_click'
  | 'double_click'
  | 'triple_click'
  | 'middle_click'
  | 'right_click'
  | 'mouse_move'
  | 'mouse_down'
  | 'mouse_up'
  | 'scroll'
  | 'type'
  | 'key_press'
  | 'hold_key'
  | 'drag'
  | 'wait'
  | 'refresh'
  | 'go_back'
  | 'go_forward'
  | 'goto_url';

export interface N15Action {
  action_type: N15ActionType;
  coordinates?: [number, number];
  start_coordinates?: [number, number];
  direction?: 'up' | 'down' | 'left' | 'right';
  amount?: number;
  text?: string;
  key?: string;
  modifier?: string;
  duration?: number;
  url?: string;
}

// n1.5 emits lowercase key names (e.g. `enter`, `ctrl+c`, `down down down enter`).
// Kernel's press_key expects XKeysym names (e.g. `Return`, `Ctrl`, `Page_Up`).
// This map covers every key Yutori documents at
// https://docs.yutori.com/reference/n1-5#key-space — keys not in the map pass
// through unchanged (printable characters like `a`, `1`, `,` are already XKeysym).
//
// Sister implementation (Playwright target instead of XKeysym):
// https://github.com/yutori-ai/yutori-sdk-python/blob/main/yutori/navigator/keys.py
const KEY_MAP: Record<string, string> = {
  // Modifiers
  ctrl: 'Ctrl',
  control: 'Ctrl',
  shift: 'Shift',
  alt: 'Alt',
  meta: 'Super_L',
  command: 'Super_L',
  cmd: 'Super_L',
  super: 'Super_L',
  option: 'Alt',
  // Enter
  enter: 'Return',
  return: 'Return',
  // Navigation
  tab: 'Tab',
  backspace: 'BackSpace',
  delete: 'Delete',
  escape: 'Escape',
  esc: 'Escape',
  space: 'space',
  // Arrows
  up: 'Up',
  down: 'Down',
  left: 'Left',
  right: 'Right',
  arrowup: 'Up',
  arrowdown: 'Down',
  arrowleft: 'Left',
  arrowright: 'Right',
  // Page nav
  home: 'Home',
  end: 'End',
  pageup: 'Page_Up',
  pagedown: 'Page_Down',
  // Function keys
  f1: 'F1', f2: 'F2', f3: 'F3', f4: 'F4', f5: 'F5', f6: 'F6',
  f7: 'F7', f8: 'F8', f9: 'F9', f10: 'F10', f11: 'F11', f12: 'F12',
  // Locks / special
  capslock: 'Caps_Lock',
  numlock: 'Num_Lock',
  scrolllock: 'Scroll_Lock',
  insert: 'Insert',
  pause: 'Pause',
  printscreen: 'Print',
};

function mapToken(token: string): string {
  const lower = token.trim().toLowerCase();
  return KEY_MAP[lower] ?? token.trim();
}

// Parse an n1.5 key expression into one Kernel combo string per sequential
// press. Spaces separate sequential presses; `+` separates simultaneous tokens
// within a press. Examples:
//   "enter"             -> ["Return"]
//   "ctrl+c"            -> ["Ctrl+c"]
//   "down down enter"   -> ["Down", "Down", "Return"]
//   "ctrl+shift+t"      -> ["Ctrl+Shift+t"]
function parseKeyExpression(expr: string): string[] {
  return expr
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map((combo) => combo.split('+').map(mapToken).join('+'));
}

export class ComputerTool {
  private kernel: Kernel;
  private sessionId: string;
  private width: number;
  private height: number;
  private kioskMode: boolean;

  constructor(kernel: Kernel, sessionId: string, width = 1280, height = 800, kioskMode = false) {
    this.kernel = kernel;
    this.sessionId = sessionId;
    this.width = width;
    this.height = height;
    this.kioskMode = kioskMode;
  }

  async execute(action: N15Action): Promise<ToolResult> {
    const { action_type } = action;

    switch (action_type) {
      case 'left_click':
        return this.handleClick(action, 'left', 1);
      case 'double_click':
        return this.handleClick(action, 'left', 2);
      case 'triple_click':
        return this.handleClick(action, 'left', 3);
      case 'middle_click':
        return this.handleClick(action, 'middle', 1);
      case 'right_click':
        return this.handleClick(action, 'right', 1);
      case 'mouse_move':
        return this.handleMouseMove(action);
      case 'mouse_down':
        return this.handleMouseButton(action, 'down');
      case 'mouse_up':
        return this.handleMouseButton(action, 'up');
      case 'scroll':
        return this.handleScroll(action);
      case 'type':
        return this.handleType(action);
      case 'key_press':
        return this.handleKeyPress(action);
      case 'hold_key':
        return this.handleHoldKey(action);
      case 'drag':
        return this.handleDrag(action);
      case 'wait':
        return this.handleWait(action);
      case 'refresh':
        return this.handleRefresh();
      case 'go_back':
        return this.handleGoBack();
      case 'go_forward':
        return this.handleGoForward();
      case 'goto_url':
        return this.handleGotoUrl(action);
      default:
        throw new ToolError(`Unknown action type: ${action_type}`);
    }
  }

  private async handleClick(action: N15Action, button: 'left' | 'right' | 'middle', numClicks: number): Promise<ToolResult> {
    const coords = this.getCoordinates(action.coordinates);
    const holdKeys = action.modifier ? [mapToken(action.modifier)] : undefined;

    await this.kernel.browsers.computer.clickMouse(this.sessionId, {
      x: coords.x,
      y: coords.y,
      button,
      click_type: 'click',
      num_clicks: numClicks,
      ...(holdKeys ? { hold_keys: holdKeys } : {}),
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleMouseMove(action: N15Action): Promise<ToolResult> {
    const coords = this.getCoordinates(action.coordinates);

    await this.kernel.browsers.computer.moveMouse(this.sessionId, {
      x: coords.x,
      y: coords.y,
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleMouseButton(action: N15Action, clickType: 'down' | 'up'): Promise<ToolResult> {
    const coords = this.getCoordinates(action.coordinates);

    await this.kernel.browsers.computer.clickMouse(this.sessionId, {
      x: coords.x,
      y: coords.y,
      button: 'left',
      click_type: clickType,
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleScroll(action: N15Action): Promise<ToolResult> {
    const coords = this.getCoordinates(action.coordinates);
    const direction = action.direction;
    const amount = Math.max(action.amount ?? 3, 1);

    if (!direction || !['up', 'down', 'left', 'right'].includes(direction)) {
      throw new ToolError(`Invalid scroll direction: ${direction}`);
    }

    // Yutori 1 unit ≈ 10% of viewport height; scale into Kernel wheel-event ticks.
    const ticks = amount * SCROLL_NOTCHES_PER_AMOUNT;

    let delta_x = 0;
    let delta_y = 0;

    switch (direction) {
      case 'up':
        delta_y = -ticks;
        break;
      case 'down':
        delta_y = ticks;
        break;
      case 'left':
        delta_x = -ticks;
        break;
      case 'right':
        delta_x = ticks;
        break;
    }

    const holdKeys = action.modifier ? [mapToken(action.modifier)] : undefined;

    await this.kernel.browsers.computer.scroll(this.sessionId, {
      x: coords.x,
      y: coords.y,
      delta_x,
      delta_y,
      ...(holdKeys ? { hold_keys: holdKeys } : {}),
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    const screenshotResult = await this.screenshot();
    return {
      ...screenshotResult,
      output: `Scrolled ${amount} unit(s) ${direction}.`,
    };
  }

  private async handleType(action: N15Action): Promise<ToolResult> {
    const text = action.text;
    if (!text) {
      throw new ToolError('text is required for type action');
    }

    await this.kernel.browsers.computer.typeText(this.sessionId, {
      text,
      delay: TYPING_DELAY_MS,
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleKeyPress(action: N15Action): Promise<ToolResult> {
    const key = action.key;
    if (!key) {
      throw new ToolError('key is required for key_press action');
    }

    // n1.5 supports sequential presses ("down down down enter") — issue each
    // combo as its own pressKey so they're seen as separate keystrokes.
    const combos = parseKeyExpression(key);
    for (const combo of combos) {
      await this.kernel.browsers.computer.pressKey(this.sessionId, { keys: [combo] });
    }

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleHoldKey(action: N15Action): Promise<ToolResult> {
    const key = action.key;
    if (!key) {
      throw new ToolError('key is required for hold_key action');
    }

    // Yutori emits `duration` in seconds; Kernel SDK's pressKey takes ms.
    const durationMs = action.duration && action.duration > 0 ? Math.round(action.duration * 1000) : 1000;

    const combos = parseKeyExpression(key);
    for (const combo of combos) {
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: [combo],
        duration: durationMs,
      });
    }

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleDrag(action: N15Action): Promise<ToolResult> {
    const startCoords = this.getCoordinates(action.start_coordinates);
    const endCoords = this.getCoordinates(action.coordinates);

    await this.kernel.browsers.computer.dragMouse(this.sessionId, {
      path: [[startCoords.x, startCoords.y], [endCoords.x, endCoords.y]],
      button: 'left',
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleWait(action: N15Action): Promise<ToolResult> {
    // Yutori emits `duration` in seconds (matches reference impl).
    const durationMs = action.duration && action.duration > 0 ? Math.round(action.duration * 1000) : 2000;
    await this.sleep(durationMs);
    return this.screenshot();
  }

  private async handleRefresh(): Promise<ToolResult> {
    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['F5'],
    });

    await this.sleep(2000);
    return this.screenshot();
  }

  private async handleGoBack(): Promise<ToolResult> {
    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['Alt+Left'],
    });

    await this.sleep(1500);
    return this.screenshot();
  }

  private async handleGoForward(): Promise<ToolResult> {
    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['Alt+Right'],
    });

    await this.sleep(1500);
    return this.screenshot();
  }

  private async handleGotoUrl(action: N15Action): Promise<ToolResult> {
    const url = action.url;
    if (!url) {
      throw new ToolError('url is required for goto_url action');
    }

    if (this.kioskMode) {
      const response = await this.kernel.browsers.playwright.execute(this.sessionId, {
        code: `await page.goto(${JSON.stringify(url)});`,
        timeout_sec: 60,
      });
      if (!response.success) {
        throw new ToolError(response.error ?? 'Playwright goto failed');
      }
      await this.sleep(ACTION_DELAY_MS);
      return this.screenshot();
    }

    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['Ctrl+l'],
    });
    await this.sleep(ACTION_DELAY_MS);

    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['Ctrl+a'],
    });
    await this.sleep(100);

    await this.kernel.browsers.computer.typeText(this.sessionId, {
      text: url,
      delay: TYPING_DELAY_MS,
    });
    await this.sleep(ACTION_DELAY_MS);

    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['Return'],
    });

    await this.sleep(2000);
    return this.screenshot();
  }

  async screenshot(): Promise<ToolResult> {
    try {
      const response = await this.kernel.browsers.computer.captureScreenshot(this.sessionId);
      const blob = await response.blob();
      const arrayBuffer = await blob.arrayBuffer();
      const pngBuffer = Buffer.from(arrayBuffer);
      const webpBuffer = await sharp(pngBuffer).webp({ quality: WEBP_QUALITY }).toBuffer();

      return {
        base64Image: webpBuffer.toString('base64'),
      };
    } catch (error) {
      throw new ToolError(`Failed to take screenshot: ${error}`);
    }
  }

  private getCoordinates(coords?: [number, number]): { x: number; y: number } {
    if (!coords || coords.length !== 2) {
      return { x: Math.floor(this.width / 2), y: Math.floor(this.height / 2) };
    }

    const [x, y] = coords;
    if (typeof x !== 'number' || typeof y !== 'number' || x < 0 || y < 0) {
      throw new ToolError(`Invalid coordinates: ${JSON.stringify(coords)}`);
    }

    return { x, y };
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
