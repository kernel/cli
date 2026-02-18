/**
 * Yutori n1 Computer Tool
 * 
 * Maps n1-latest action format to Kernel's Computer Controls API.
 * Screenshots are converted to WebP for better compression across multi-step trajectories.
 */

import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';
import sharp from 'sharp';

const TYPING_DELAY_MS = 12;
const SCREENSHOT_DELAY_MS = 300;
const ACTION_DELAY_MS = 300;

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

export type N1ActionType =
  | 'left_click'
  | 'double_click'
  | 'triple_click'
  | 'right_click'
  | 'scroll'
  | 'type'
  | 'key_press'
  | 'hover'
  | 'drag'
  | 'wait'
  | 'refresh'
  | 'go_back'
  | 'goto_url';

export interface N1Action {
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

const KEY_MAP: Record<string, string> = {
  'Enter': 'Return',
  'Escape': 'Escape',
  'Backspace': 'BackSpace',
  'Tab': 'Tab',
  'Delete': 'Delete',
  'ArrowUp': 'Up',
  'ArrowDown': 'Down',
  'ArrowLeft': 'Left',
  'ArrowRight': 'Right',
  'Home': 'Home',
  'End': 'End',
  'PageUp': 'Page_Up',
  'PageDown': 'Page_Down',
  'F1': 'F1',
  'F2': 'F2',
  'F3': 'F3',
  'F4': 'F4',
  'F5': 'F5',
  'F6': 'F6',
  'F7': 'F7',
  'F8': 'F8',
  'F9': 'F9',
  'F10': 'F10',
  'F11': 'F11',
  'F12': 'F12',
};

const MODIFIER_MAP: Record<string, string> = {
  'control': 'ctrl',
  'ctrl': 'ctrl',
  'alt': 'alt',
  'shift': 'shift',
  'meta': 'super',
  'command': 'super',
  'cmd': 'super',
};

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

  async execute(action: N1Action): Promise<ToolResult> {
    const { action_type } = action;

    switch (action_type) {
      case 'left_click':
        return this.handleClick(action, 'left', 1);
      case 'double_click':
        return this.handleClick(action, 'left', 2);
      case 'triple_click':
        return this.handleClick(action, 'left', 3);
      case 'right_click':
        return this.handleClick(action, 'right', 1);
      case 'scroll':
        return this.handleScroll(action);
      case 'type':
        return this.handleType(action);
      case 'key_press':
        return this.handleKeyPress(action);
      case 'hover':
        return this.handleHover(action);
      case 'drag':
        return this.handleDrag(action);
      case 'wait':
        return this.handleWait();
      case 'refresh':
        return this.handleRefresh();
      case 'go_back':
        return this.handleGoBack();
      case 'goto_url':
        return this.handleGotoUrl(action);
      default:
        throw new ToolError(`Unknown action type: ${action_type}`);
    }
  }

  private async handleClick(action: N1Action, button: 'left' | 'right', numClicks: number): Promise<ToolResult> {
    const coords = this.getCoordinates(action.coordinates);
    
    await this.kernel.browsers.computer.clickMouse(this.sessionId, {
      x: coords.x,
      y: coords.y,
      button,
      click_type: 'click',
      num_clicks: numClicks,
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleScroll(action: N1Action): Promise<ToolResult> {
    const coords = this.getCoordinates(action.coordinates);
    const direction = action.direction;
    const amount = action.amount ?? 3;

    if (!direction || !['up', 'down', 'left', 'right'].includes(direction)) {
      throw new ToolError(`Invalid scroll direction: ${direction}`);
    }

    const scrollDelta = amount * 100;

    let delta_x = 0;
    let delta_y = 0;

    switch (direction) {
      case 'up':
        delta_y = -scrollDelta;
        break;
      case 'down':
        delta_y = scrollDelta;
        break;
      case 'left':
        delta_x = -scrollDelta;
        break;
      case 'right':
        delta_x = scrollDelta;
        break;
    }

    await this.kernel.browsers.computer.scroll(this.sessionId, {
      x: coords.x,
      y: coords.y,
      delta_x,
      delta_y,
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleType(action: N1Action): Promise<ToolResult> {
    const text = action.text;
    if (!text) {
      throw new ToolError('text is required for type action');
    }

    if (action.clear_before_typing) {
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: ['ctrl+a'],
      });
      await this.sleep(100);
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: ['BackSpace'],
      });
      await this.sleep(100);
    }

    await this.kernel.browsers.computer.typeText(this.sessionId, {
      text,
      delay: TYPING_DELAY_MS,
    });

    if (action.press_enter_after) {
      await this.sleep(100);
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: ['Return'],
      });
    }

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleKeyPress(action: N1Action): Promise<ToolResult> {
    const keyComb = action.key_comb;
    if (!keyComb) {
      throw new ToolError('key_comb is required for key_press action');
    }

    const mappedKey = this.mapKey(keyComb);

    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: [mappedKey],
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleHover(action: N1Action): Promise<ToolResult> {
    const coords = this.getCoordinates(action.coordinates);

    await this.kernel.browsers.computer.moveMouse(this.sessionId, {
      x: coords.x,
      y: coords.y,
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleDrag(action: N1Action): Promise<ToolResult> {
    const startCoords = this.getCoordinates(action.start_coordinates);
    const endCoords = this.getCoordinates(action.coordinates);

    await this.kernel.browsers.computer.dragMouse(this.sessionId, {
      path: [[startCoords.x, startCoords.y], [endCoords.x, endCoords.y]],
      button: 'left',
    });

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleWait(): Promise<ToolResult> {
    await this.sleep(2000);
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
      keys: ['alt+Left'],
    });

    await this.sleep(1500);
    return this.screenshot();
  }

  private async handleGotoUrl(action: N1Action): Promise<ToolResult> {
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
        return { error: response.error ?? 'Playwright goto failed' };
      }
      await this.sleep(ACTION_DELAY_MS);
      return this.screenshot();
    }

    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['ctrl+l'],
    });
    await this.sleep(ACTION_DELAY_MS);

    await this.kernel.browsers.computer.pressKey(this.sessionId, {
      keys: ['ctrl+a'],
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
      const webpBuffer = await sharp(pngBuffer).webp({ quality: 80 }).toBuffer();

      return {
        base64Image: webpBuffer.toString('base64'),
      };
    } catch (error) {
      throw new ToolError(`Failed to take screenshot: ${error}`);
    }
  }

  private getCoordinates(coords?: [number, number]): { x: number; y: number } {
    if (!coords || coords.length !== 2) {
      return { x: this.width / 2, y: this.height / 2 };
    }

    const [x, y] = coords;
    if (typeof x !== 'number' || typeof y !== 'number' || x < 0 || y < 0) {
      throw new ToolError(`Invalid coordinates: ${JSON.stringify(coords)}`);
    }

    return { x, y };
  }

  private mapKey(key: string): string {
    if (key.includes('+')) {
      const parts = key.split('+');
      const mappedParts = parts.map(part => {
        const trimmed = part.trim();
        const lower = trimmed.toLowerCase();
        
        if (MODIFIER_MAP[lower]) {
          return MODIFIER_MAP[lower];
        }
        
        return KEY_MAP[trimmed] || trimmed;
      });
      return mappedParts.join('+');
    }

    return KEY_MAP[key] || key;
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
