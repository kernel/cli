import { Kernel } from '@onkernel/sdk';

// CUA model key names -> X11 keysym names for the Kernel computer API
const KEYSYM_MAP: Record<string, string> = {
  ENTER: 'Return',
  Enter: 'Return',
  RETURN: 'Return',
  BACKSPACE: 'BackSpace',
  Backspace: 'BackSpace',
  DELETE: 'Delete',
  TAB: 'Tab',
  ESCAPE: 'Escape',
  Escape: 'Escape',
  ESC: 'Escape',
  SPACE: 'space',
  Space: 'space',
  UP: 'Up',
  DOWN: 'Down',
  LEFT: 'Left',
  RIGHT: 'Right',
  HOME: 'Home',
  END: 'End',
  PAGEUP: 'Prior',
  PAGE_UP: 'Prior',
  PageUp: 'Prior',
  PAGEDOWN: 'Next',
  PAGE_DOWN: 'Next',
  PageDown: 'Next',
  CAPS_LOCK: 'Caps_Lock',
  CapsLock: 'Caps_Lock',
  CTRL: 'Control_L',
  Ctrl: 'Control_L',
  CONTROL: 'Control_L',
  Control: 'Control_L',
  ALT: 'Alt_L',
  Alt: 'Alt_L',
  SHIFT: 'Shift_L',
  Shift: 'Shift_L',
  META: 'Super_L',
  Meta: 'Super_L',
  SUPER: 'Super_L',
  Super: 'Super_L',
  CMD: 'Super_L',
  COMMAND: 'Super_L',
  F1: 'F1',
  F2: 'F2',
  F3: 'F3',
  F4: 'F4',
  F5: 'F5',
  F6: 'F6',
  F7: 'F7',
  F8: 'F8',
  F9: 'F9',
  F10: 'F10',
  F11: 'F11',
  F12: 'F12',
  INSERT: 'Insert',
  Insert: 'Insert',
  PRINT: 'Print',
  SCROLLLOCK: 'Scroll_Lock',
  PAUSE: 'Pause',
  NUMLOCK: 'Num_Lock',
};

function translateKeys(keys: string[]): string[] {
  return keys.map((k) => KEYSYM_MAP[k] ?? k);
}

interface CuaAction {
  type: string;
  x?: number;
  y?: number;
  text?: string;
  keys?: string[];
  button?: string | number;
  scroll_x?: number;
  scroll_y?: number;
  ms?: number;
  path?: Array<{ x: number; y: number }>;
}

type BatchAction = {
  type: 'click_mouse' | 'move_mouse' | 'type_text' | 'press_key' | 'scroll' | 'drag_mouse' | 'sleep';
  click_mouse?: { x: number; y: number; button?: string; num_clicks?: number };
  move_mouse?: { x: number; y: number };
  type_text?: { text: string };
  press_key?: { keys: string[] };
  scroll?: { x: number; y: number; delta_x?: number; delta_y?: number };
  drag_mouse?: { path: number[][] };
  sleep?: { duration_ms: number };
};

function normalizeButton(button?: string | number): string {
  if (button === undefined || button === null) return 'left';
  if (typeof button === 'number') {
    switch (button) {
      case 1: return 'left';
      case 2: return 'middle';
      case 3: return 'right';
      default: return 'left';
    }
  }
  return button;
}

function translateCuaAction(action: CuaAction): BatchAction {
  switch (action.type) {
    case 'click':
      return {
        type: 'click_mouse',
        click_mouse: { x: action.x ?? 0, y: action.y ?? 0, button: normalizeButton(action.button) },
      };
    case 'double_click':
      return {
        type: 'click_mouse',
        click_mouse: { x: action.x ?? 0, y: action.y ?? 0, num_clicks: 2 },
      };
    case 'type':
      return { type: 'type_text', type_text: { text: action.text ?? '' } };
    case 'keypress':
      return { type: 'press_key', press_key: { keys: translateKeys(action.keys ?? []) } };
    case 'scroll':
      return {
        type: 'scroll',
        scroll: {
          x: action.x ?? 0,
          y: action.y ?? 0,
          delta_x: action.scroll_x ?? 0,
          delta_y: action.scroll_y ?? 0,
        },
      };
    case 'move':
      return { type: 'move_mouse', move_mouse: { x: action.x ?? 0, y: action.y ?? 0 } };
    case 'drag': {
      const path = (action.path ?? []).map((p) => [p.x, p.y]);
      return { type: 'drag_mouse', drag_mouse: { path } };
    }
    case 'wait':
      return { type: 'sleep', sleep: { duration_ms: action.ms ?? 1000 } };
    default:
      throw new Error(`Unknown CUA action type: ${action.type}`);
  }
}

export class KernelComputer {
  private client: Kernel;
  private sessionId: string;
  private width = 1024;
  private height = 768;

  constructor(client: Kernel, sessionId: string) {
    this.client = client;
    this.sessionId = sessionId;
  }

  getEnvironment(): 'browser' {
    return 'browser';
  }

  getDimensions(): [number, number] {
    return [this.width, this.height];
  }

  async screenshot(): Promise<string> {
    const resp = await this.client.browsers.computer.captureScreenshot(this.sessionId);
    const buf = Buffer.from(await resp.arrayBuffer());
    return buf.toString('base64');
  }

  async click(x: number, y: number, button: string | number = 'left'): Promise<void> {
    await this.client.browsers.computer.clickMouse(this.sessionId, {
      x,
      y,
      button: normalizeButton(button) as 'left' | 'right' | 'middle',
    });
  }

  async doubleClick(x: number, y: number): Promise<void> {
    await this.client.browsers.computer.clickMouse(this.sessionId, { x, y, num_clicks: 2 });
  }

  async type(text: string): Promise<void> {
    await this.client.browsers.computer.typeText(this.sessionId, { text });
  }

  async keypress(keys: string[]): Promise<void> {
    await this.client.browsers.computer.pressKey(this.sessionId, { keys: translateKeys(keys) });
  }

  async scroll(x: number, y: number, scrollX: number, scrollY: number): Promise<void> {
    await this.client.browsers.computer.scroll(this.sessionId, {
      x,
      y,
      delta_x: scrollX,
      delta_y: scrollY,
    });
  }

  async move(x: number, y: number): Promise<void> {
    await this.client.browsers.computer.moveMouse(this.sessionId, { x, y });
  }

  async drag(path: Array<{ x: number; y: number }>): Promise<void> {
    const p = path.map((pt) => [pt.x, pt.y]);
    await this.client.browsers.computer.dragMouse(this.sessionId, { path: p });
  }

  async wait(ms = 1000): Promise<void> {
    await new Promise((resolve) => setTimeout(resolve, ms));
  }

  async batchActions(actions: CuaAction[]): Promise<void> {
    const translated = actions.map(translateCuaAction);
    await this.client.browsers.computer.batch(this.sessionId, {
      actions: translated as Parameters<typeof this.client.browsers.computer.batch>[1]['actions'],
    });
  }

  async goto(url: string): Promise<void> {
    await this.client.browsers.playwright.execute(this.sessionId, {
      code: `await page.goto(${JSON.stringify(url)})`,
    });
  }

  async back(): Promise<void> {
    await this.client.browsers.playwright.execute(this.sessionId, {
      code: 'await page.goBack()',
    });
  }

  async forward(): Promise<void> {
    await this.client.browsers.playwright.execute(this.sessionId, {
      code: 'await page.goForward()',
    });
  }

  async getCurrentUrl(): Promise<string> {
    const result = await this.client.browsers.playwright.execute(this.sessionId, {
      code: 'return page.url()',
    });
    return (result.result as string) ?? '';
  }
}
