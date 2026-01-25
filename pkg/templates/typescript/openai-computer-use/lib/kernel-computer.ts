import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';

const KEY_MAP: Record<string, string> = {
  // Enter/Return
  return: 'Return',
  enter: 'Return',
  Enter: 'Return',
  // Arrow keys
  left: 'Left',
  right: 'Right',
  up: 'Up',
  down: 'Down',
  arrowdown: 'ArrowDown',
  arrowleft: 'ArrowLeft',
  arrowright: 'ArrowRight',
  arrowup: 'ArrowUp',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  // Navigation
  home: 'Home',
  end: 'End',
  pageup: 'Page_Up',
  page_up: 'Page_Up',
  PageUp: 'Page_Up',
  pagedown: 'Page_Down',
  page_down: 'Page_Down',
  PageDown: 'Page_Down',
  // Editing
  delete: 'Delete',
  backspace: 'BackSpace',
  Backspace: 'BackSpace',
  tab: 'Tab',
  insert: 'Insert',
  // Escape
  esc: 'Escape',
  escape: 'Escape',
  // Function keys
  f1: 'F1',
  f2: 'F2',
  f3: 'F3',
  f4: 'F4',
  f5: 'F5',
  f6: 'F6',
  f7: 'F7',
  f8: 'F8',
  f9: 'F9',
  f10: 'F10',
  f11: 'F11',
  f12: 'F12',
  // Misc
  space: 'space',
  minus: 'minus',
  equal: 'equal',
  plus: 'plus',
};

const MODIFIER_MAP: Record<string, string> = {
  ctrl: 'ctrl',
  control: 'ctrl',
  Control: 'ctrl',
  alt: 'alt',
  Alt: 'alt',
  shift: 'shift',
  Shift: 'shift',
  meta: 'super',
  Meta: 'super',
  cmd: 'super',
  command: 'super',
  win: 'super',
  super: 'super',
  option: 'alt',
};

interface Point {
  x: number;
  y: number;
}

const TYPING_DELAY_MS = 12;
const DEFAULT_WIDTH = 1024;
const DEFAULT_HEIGHT = 768;

export class KernelComputer {
  private kernel: Kernel;
  private sessionId: string;
  private width: number;
  private height: number;
  private currentUrl: string = 'about:blank';
  private screenshotDelay = 0.5;

  constructor(kernel: Kernel, sessionId: string, width = DEFAULT_WIDTH, height = DEFAULT_HEIGHT) {
    this.kernel = kernel;
    this.sessionId = sessionId;
    this.width = width;
    this.height = height;
  }

  getEnvironment(): 'browser' {
    return 'browser';
  }

  getDimensions(): [number, number] {
    return [this.width, this.height];
  }

  getCurrentUrl(): string {
    return this.currentUrl;
  }

  private convertToKernelKey(key: string): string {
    // Check modifier keys first
    if (MODIFIER_MAP[key]) {
      return MODIFIER_MAP[key];
    }
    // Check special keys
    if (KEY_MAP[key]) {
      return KEY_MAP[key];
    }
    // Check lowercase version
    if (KEY_MAP[key.toLowerCase()]) {
      return KEY_MAP[key.toLowerCase()];
    }
    // Return as-is if no mapping exists
    return key;
  }

  private convertKeyCombinationToKernel(combo: string): string {
    // Handle key combinations (e.g., "ctrl+a", "Control+t")
    if (combo.includes('+')) {
      const parts = combo.split('+');
      const mappedParts = parts.map((part) => this.convertToKernelKey(part.trim()));
      return mappedParts.join('+');
    }
    // Single key - just convert it
    return this.convertToKernelKey(combo);
  }

  async screenshot(): Promise<string> {
    // Small delay to let the page settle
    await this.sleep(this.screenshotDelay * 1000);

    const response = await this.kernel.browsers.computer.captureScreenshot(this.sessionId);
    const blob = await response.blob();
    const arrayBuffer = await blob.arrayBuffer();
    const buffer = Buffer.from(arrayBuffer);

    return buffer.toString('base64');
  }

  async click(
    button: 'left' | 'right' | 'back' | 'forward' | 'wheel',
    x: number,
    y: number,
  ): Promise<void> {
    switch (button) {
      case 'back':
        await this.back();
        return;
      case 'forward':
        await this.forward();
        return;
      case 'wheel':
        // Wheel button at coordinates - treat as scroll
        await this.kernel.browsers.computer.scroll(this.sessionId, {
          x,
          y,
          delta_x: 0,
          delta_y: 120, // Default scroll amount
        });
        return;
      default: {
        const btn = button === 'right' ? 'right' : 'left';
        await this.kernel.browsers.computer.clickMouse(this.sessionId, {
          x,
          y,
          button: btn,
          click_type: 'click',
        });
      }
    }
  }

  async doubleClick(x: number, y: number): Promise<void> {
    await this.kernel.browsers.computer.clickMouse(this.sessionId, {
      x,
      y,
      button: 'left',
      click_type: 'click',
      num_clicks: 2,
    });
  }

  async scroll(x: number, y: number, scrollX: number, scrollY: number): Promise<void> {
    await this.kernel.browsers.computer.scroll(this.sessionId, {
      x,
      y,
      delta_x: scrollX,
      delta_y: scrollY,
    });
  }

  async type(text: string): Promise<void> {
    await this.kernel.browsers.computer.typeText(this.sessionId, {
      text,
      delay: TYPING_DELAY_MS,
    });
  }

  async keypress(keys: string[]): Promise<void> {
    const mappedKeys = keys.map((k) => this.convertToKernelKey(k));
    const combo = mappedKeys.join('+');

    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: [combo],
    });
  }

  async wait(ms = 1000): Promise<void> {
    await this.sleep(ms);
  }

  async move(x: number, y: number): Promise<void> {
    await this.kernel.browsers.computer.moveMouse(this.sessionId, {
      x,
      y,
    });
  }

  async drag(path: Point[]): Promise<void> {
    if (!path || path.length < 2) return;
    const kernelPath: [number, number][] = path.map((p) => [p.x, p.y]);

    await this.kernel.browsers.computer.dragMouse(this.sessionId, {
      path: kernelPath,
      button: 'left',
    });
  }

  async goto(url: string): Promise<void> {
    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['ctrl+l'],
    });
    await this.sleep(200);
    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['ctrl+a'],
    });
    await this.sleep(100);

    await this.kernel.browsers.computer.typeText(this.sessionId, {
      text: url,
      delay: TYPING_DELAY_MS,
    });
    await this.sleep(100);
    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['Return'],
    });
    await this.sleep(1000);
    this.currentUrl = url;
  }

  async back(): Promise<void> {
    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['alt+Left'],
    });
    await this.sleep(500);
  }

  async forward(): Promise<void> {
    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['alt+Right'],
    });
    await this.sleep(500);
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
