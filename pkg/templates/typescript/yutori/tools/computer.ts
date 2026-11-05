/**
 * Yutori n2 Computer Tool
 *
 * Maps n2's `computer_batch` action list onto Kernel's Computer Controls API.
 * n2 plans every action in a batch against the screenshot taken *before* the
 * batch ran, so the batch executes sequentially, stops at the first error, and
 * answers with a single screenshot taken after the last action that ran.
 *
 * @see https://docs.yutori.com/reference/n2
 */

import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';
import sharp from 'sharp';

const TYPING_DELAY_MS = 12;
// Let the UI settle between actions in a batch, and again before the screenshot
// that the model will plan its next batch against.
const INTER_ACTION_DELAY_MS = 80;
const SETTLE_DELAY_MS = 400;

const NAVIGATOR_COORDINATE_SCALE = 1000;

// n2's scroll `amount` is a wheel notch count (1-50), which is exactly what
// Kernel's delta_x/delta_y take ("xdotool wheel units"), so it passes through.
const DEFAULT_SCROLL_AMOUNT = 3;

// WebP quality for screenshots. Kernel returns PNGs, which are crisp and
// tolerate aggressive WebP compression with no visible degradation — matches
// Yutori SDK's DEFAULT_WEBP_QUALITY_FOR_PNG=30 (yutori-sdk-python/yutori/
// navigator/images.py). Requests are capped at 10 MB and a trajectory carries
// several full-screen captures, so the compression matters.
const WEBP_QUALITY = 30;

export class ToolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ToolError';
  }
}

export type N2ActionName =
  | 'left_click'
  | 'double_click'
  | 'triple_click'
  | 'middle_click'
  | 'right_click'
  | 'scroll'
  | 'type'
  | 'key_press'
  | 'drag'
  | 'mouse_move'
  | 'mouse_down'
  | 'mouse_up'
  | 'hold_key'
  | 'wait'
  | 'screenshot';

export interface N2ActionArgs {
  coordinates?: [number, number];
  start_coordinates?: [number, number];
  direction?: 'up' | 'down';
  amount?: number;
  text?: string;
  key?: string;
  modifier?: string;
  duration?: number;
}

/** One `{name, arguments}` member of a `computer_batch` call. */
export interface N2Action {
  name: N2ActionName;
  arguments?: N2ActionArgs;
}

export interface BatchOutcome {
  executed: number;
  total: number;
  /** Set when an action threw; the remaining actions were skipped. */
  failure?: { index: number; name: string; message: string };
}

// n2 emits lowercase key names (e.g. `enter`, `ctrl+c`, `down down down enter`).
// Kernel's press_key expects XKeysym names (e.g. `Return`, `Ctrl`, `Page_Up`).
// This map covers every key Yutori documents at
// https://docs.yutori.com/reference/n2#key-space — keys not in the map pass
// through unchanged (printable characters like `a` and `1` are already XKeysym).
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
  // Punctuation — n2 sends word forms, most of which are already XKeysym names
  minus: 'minus',
  plus: 'plus',
  equal: 'equal',
  comma: 'comma',
  period: 'period',
  slash: 'slash',
  backslash: 'backslash',
  semicolon: 'semicolon',
  quote: 'apostrophe',
  backquote: 'grave',
  bracketleft: 'bracketleft',
  bracketright: 'bracketright',
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

// Parse an n2 key expression into one Kernel combo string per sequential press.
// Spaces separate sequential presses; `+` separates simultaneous tokens within a
// press. Examples:
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
  private screenWidth: number;
  private screenHeight: number;

  constructor(kernel: Kernel, sessionId: string, screenWidth = 1280, screenHeight = 800) {
    this.kernel = kernel;
    this.sessionId = sessionId;
    this.screenWidth = screenWidth;
    this.screenHeight = screenHeight;
  }

  /**
   * Run a `computer_batch` action list in order, stopping at the first failure.
   * The caller reports the outcome and a single post-batch screenshot back to n2.
   */
  async runBatch(actions: N2Action[]): Promise<BatchOutcome> {
    for (let i = 0; i < actions.length; i++) {
      const action = actions[i]!;
      try {
        await this.runAction(action);
      } catch (error) {
        return {
          executed: i,
          total: actions.length,
          failure: {
            index: i,
            name: action.name,
            message: error instanceof Error ? error.message : String(error),
          },
        };
      }
      if (i < actions.length - 1) {
        await this.sleep(INTER_ACTION_DELAY_MS);
      }
    }

    return { executed: actions.length, total: actions.length };
  }

  private async runAction(action: N2Action): Promise<void> {
    const args = action.arguments ?? {};

    switch (action.name) {
      case 'left_click':
        return this.click(args, 'left', 1);
      case 'double_click':
        return this.click(args, 'left', 2);
      case 'triple_click':
        return this.click(args, 'left', 3);
      case 'middle_click':
        return this.click(args, 'middle', 1);
      case 'right_click':
        return this.click(args, 'right', 1);
      case 'scroll':
        return this.scroll(args);
      case 'type':
        return this.type(args);
      case 'key_press':
        return this.keyPress(args);
      case 'drag':
        return this.drag(args);
      case 'mouse_move':
        return this.mouseMove(args);
      case 'mouse_down':
        return this.mouseButton(args, 'down');
      case 'mouse_up':
        return this.mouseButton(args, 'up');
      case 'hold_key':
        return this.holdKey(args);
      case 'wait':
        return this.sleep(durationMs(args.duration, 2000));
      case 'screenshot':
        // A batch already answers with a screenshot taken after its last action,
        // so an explicit `screenshot` member has nothing to do.
        return;
      default:
        throw new ToolError(`Unknown action: ${action.name}`);
    }
  }

  private async click(args: N2ActionArgs, button: 'left' | 'right' | 'middle', numClicks: number): Promise<void> {
    const { x, y } = this.requireCoordinates(args.coordinates);
    const holdKeys = args.modifier ? [mapToken(args.modifier)] : undefined;

    await this.kernel.browsers.computer.clickMouse(this.sessionId, {
      x,
      y,
      button,
      click_type: 'click',
      num_clicks: numClicks,
      ...(holdKeys ? { hold_keys: holdKeys } : {}),
    });
  }

  private async mouseMove(args: N2ActionArgs): Promise<void> {
    const { x, y } = this.requireCoordinates(args.coordinates);
    await this.kernel.browsers.computer.moveMouse(this.sessionId, { x, y });
  }

  private async mouseButton(args: N2ActionArgs, clickType: 'down' | 'up'): Promise<void> {
    // Coordinates are optional here — without them the button is pressed or
    // released wherever the cursor already is, which is what a manual
    // mouse_move -> mouse_down -> mouse_move -> mouse_up drag relies on.
    const { x, y } = args.coordinates
      ? this.requireCoordinates(args.coordinates)
      : await this.kernel.browsers.computer.getMousePosition(this.sessionId);

    await this.kernel.browsers.computer.clickMouse(this.sessionId, {
      x,
      y,
      button: 'left',
      click_type: clickType,
    });
  }

  private async scroll(args: N2ActionArgs): Promise<void> {
    const { x, y } = this.requireCoordinates(args.coordinates);
    const direction = args.direction;

    // n2 only scrolls vertically.
    if (direction !== 'up' && direction !== 'down') {
      throw new ToolError(`Invalid scroll direction: ${direction}`);
    }

    const notches = Math.max(1, Math.round(args.amount ?? DEFAULT_SCROLL_AMOUNT));
    const holdKeys = args.modifier ? [mapToken(args.modifier)] : undefined;

    await this.kernel.browsers.computer.scroll(this.sessionId, {
      x,
      y,
      delta_x: 0,
      delta_y: direction === 'up' ? -notches : notches,
      ...(holdKeys ? { hold_keys: holdKeys } : {}),
    });
  }

  private async type(args: N2ActionArgs): Promise<void> {
    if (!args.text) {
      throw new ToolError('text is required for type');
    }

    await this.kernel.browsers.computer.typeText(this.sessionId, {
      text: args.text,
      delay: TYPING_DELAY_MS,
    });
  }

  private async keyPress(args: N2ActionArgs): Promise<void> {
    if (!args.key) {
      throw new ToolError('key is required for key_press');
    }

    // n2 supports sequential presses ("down down down enter") — issue each combo
    // as its own pressKey so they're seen as separate keystrokes.
    for (const combo of parseKeyExpression(args.key)) {
      await this.kernel.browsers.computer.pressKey(this.sessionId, { keys: [combo] });
    }
  }

  private async holdKey(args: N2ActionArgs): Promise<void> {
    if (!args.key) {
      throw new ToolError('key is required for hold_key');
    }

    for (const combo of parseKeyExpression(args.key)) {
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: [combo],
        duration: durationMs(args.duration, 1000),
      });
    }
  }

  private async drag(args: N2ActionArgs): Promise<void> {
    const start = this.requireCoordinates(args.start_coordinates);
    const end = this.requireCoordinates(args.coordinates);

    await this.kernel.browsers.computer.dragMouse(this.sessionId, {
      path: [[start.x, start.y], [end.x, end.y]],
      button: 'left',
    });
  }

  /** Capture the whole screen — browser chrome included — as base64 WebP. */
  async screenshot(): Promise<string> {
    await this.sleep(SETTLE_DELAY_MS);

    const response = await this.kernel.browsers.computer.captureScreenshot(this.sessionId);
    const pngBuffer = Buffer.from(await (await response.blob()).arrayBuffer());
    const webpBuffer = await sharp(pngBuffer).webp({ quality: WEBP_QUALITY }).toBuffer();

    return webpBuffer.toString('base64');
  }

  // Map [0, 1000] coordinates into screen pixels and clamp to [0, dim-1] so a
  // boundary value like 1000 doesn't land one pixel outside the screen.
  private requireCoordinates(coords?: [number, number]): { x: number; y: number } {
    if (!coords || coords.length !== 2) {
      throw new ToolError(`coordinates are required, got ${JSON.stringify(coords)}`);
    }

    const [nx, ny] = coords;
    if (typeof nx !== 'number' || typeof ny !== 'number') {
      throw new ToolError(`Invalid coordinates: ${JSON.stringify(coords)}`);
    }

    return {
      x: clamp(Math.round((nx / NAVIGATOR_COORDINATE_SCALE) * this.screenWidth), this.screenWidth),
      y: clamp(Math.round((ny / NAVIGATOR_COORDINATE_SCALE) * this.screenHeight), this.screenHeight),
    };
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}

function clamp(value: number, dimension: number): number {
  return Math.max(0, Math.min(dimension - 1, value));
}

// n2 emits `duration` in seconds; Kernel and setTimeout take milliseconds.
function durationMs(duration: number | undefined, fallbackMs: number): number {
  return duration && duration > 0 ? Math.round(duration * 1000) : fallbackMs;
}
