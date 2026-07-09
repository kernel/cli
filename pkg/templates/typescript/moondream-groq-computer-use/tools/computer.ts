/**
 * Computer Tool - Maps high-level actions to Kernel's Computer Controls API.
 */

import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';
import {
  ComputerAction,
  PREDEFINED_COMPUTER_USE_FUNCTIONS,
  DEFAULT_SCREEN_SIZE,
  COORDINATE_SCALE,
  type ComputerFunctionArgs,
  type ToolResult,
  type ScreenSize,
} from './types/computer';

const TYPING_DELAY_MS = 12;
const SCREENSHOT_DELAY_MS = 500;

export class ComputerTool {
  private kernel: Kernel;
  private sessionId: string;
  private screenSize: ScreenSize;

  constructor(kernel: Kernel, sessionId: string, screenSize: ScreenSize = DEFAULT_SCREEN_SIZE) {
    this.kernel = kernel;
    this.sessionId = sessionId;
    this.screenSize = screenSize;
  }

  getScreenSize(): ScreenSize {
    return this.screenSize;
  }

  getKernel(): Kernel {
    return this.kernel;
  }

  getSessionId(): string {
    return this.sessionId;
  }

  private denormalizeX(x: number): number {
    return Math.round((x / COORDINATE_SCALE) * this.screenSize.width);
  }

  private denormalizeY(y: number): number {
    return Math.round((y / COORDINATE_SCALE) * this.screenSize.height);
  }

  async screenshot(): Promise<ToolResult> {
    try {
      await this.sleep(SCREENSHOT_DELAY_MS);
      const response = await this.kernel.browsers.computer.captureScreenshot(this.sessionId);
      const blob = await response.blob();
      const arrayBuffer = await blob.arrayBuffer();
      const buffer = Buffer.from(arrayBuffer);
      const dimensions = parsePngDimensions(buffer);
      if (dimensions) {
        this.screenSize = dimensions;
      }

      return {
        base64Image: buffer.toString('base64'),
        url: 'about:blank',
        width: dimensions?.width,
        height: dimensions?.height,
      };
    } catch (error) {
      return {
        error: `Failed to take screenshot: ${error}`,
        url: 'about:blank',
      };
    }
  }

  async executeAction(actionName: string, args: ComputerFunctionArgs): Promise<ToolResult> {
    if (!PREDEFINED_COMPUTER_USE_FUNCTIONS.includes(actionName as ComputerAction)) {
      return { error: `Unknown action: ${actionName}` };
    }

    try {
      switch (actionName) {
        case ComputerAction.OPEN_WEB_BROWSER:
          break;

        case ComputerAction.CLICK_AT: {
          if (args.x === undefined || args.y === undefined) {
            return { error: 'click_at requires x and y coordinates' };
          }
          const x = this.denormalizeX(args.x);
          const y = this.denormalizeY(args.y);
          const numClicks = typeof args.clicks === 'number' ? args.clicks : 1;
          await this.kernel.browsers.computer.clickMouse(this.sessionId, {
            x,
            y,
            button: 'left',
            click_type: 'click',
            num_clicks: numClicks,
          });
          break;
        }

        case ComputerAction.HOVER_AT: {
          if (args.x === undefined || args.y === undefined) {
            return { error: 'hover_at requires x and y coordinates' };
          }
          const x = this.denormalizeX(args.x);
          const y = this.denormalizeY(args.y);
          await this.kernel.browsers.computer.moveMouse(this.sessionId, { x, y });
          break;
        }

        case ComputerAction.TYPE_TEXT_AT: {
          if (args.x === undefined || args.y === undefined) {
            return { error: 'type_text_at requires x and y coordinates' };
          }
          if (!args.text) {
            return { error: 'type_text_at requires text' };
          }

          const x = this.denormalizeX(args.x);
          const y = this.denormalizeY(args.y);

          await this.kernel.browsers.computer.clickMouse(this.sessionId, {
            x,
            y,
            button: 'left',
            click_type: 'click',
            num_clicks: 1,
          });

          if (args.clear_before_typing !== false) {
            await this.kernel.browsers.computer.pressKey(this.sessionId, {
              keys: ['ctrl+a'],
            });
            await this.sleep(50);
          }

          await this.kernel.browsers.computer.typeText(this.sessionId, {
            text: args.text,
            delay: TYPING_DELAY_MS,
          });

          if (args.press_enter) {
            await this.sleep(100);
            await this.kernel.browsers.computer.pressKey(this.sessionId, {
              keys: ['Return'],
            });
          }
          break;
        }

        case ComputerAction.SCROLL_DOCUMENT: {
          if (!args.direction) {
            return { error: 'scroll_document requires direction' };
          }
          const centerX = Math.round(this.screenSize.width / 2);
          const centerY = Math.round(this.screenSize.height / 2);
          const scrollDelta = 500;

          let deltaX = 0;
          let deltaY = 0;
          if (args.direction === 'down') deltaY = scrollDelta;
          else if (args.direction === 'up') deltaY = -scrollDelta;
          else if (args.direction === 'right') deltaX = scrollDelta;
          else if (args.direction === 'left') deltaX = -scrollDelta;

          await this.kernel.browsers.computer.scroll(this.sessionId, {
            x: centerX,
            y: centerY,
            delta_x: deltaX,
            delta_y: deltaY,
          });
          break;
        }

        case ComputerAction.SCROLL_AT: {
          if (args.x === undefined || args.y === undefined) {
            return { error: 'scroll_at requires x and y coordinates' };
          }
          if (!args.direction) {
            return { error: 'scroll_at requires direction' };
          }

          const x = this.denormalizeX(args.x);
          const y = this.denormalizeY(args.y);

          let magnitude = args.magnitude ?? 800;
          if (args.direction === 'up' || args.direction === 'down') {
            magnitude = this.denormalizeY(magnitude);
          } else {
            magnitude = this.denormalizeX(magnitude);
          }

          let deltaX = 0;
          let deltaY = 0;
          if (args.direction === 'down') deltaY = magnitude;
          else if (args.direction === 'up') deltaY = -magnitude;
          else if (args.direction === 'right') deltaX = magnitude;
          else if (args.direction === 'left') deltaX = -magnitude;

          await this.kernel.browsers.computer.scroll(this.sessionId, {
            x,
            y,
            delta_x: deltaX,
            delta_y: deltaY,
          });
          break;
        }

        case ComputerAction.WAIT_5_SECONDS:
          await this.sleep(5000);
          break;

        case ComputerAction.GO_BACK:
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: ['alt+Left'],
          });
          await this.sleep(1000);
          break;

        case ComputerAction.GO_FORWARD:
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: ['alt+Right'],
          });
          await this.sleep(1000);
          break;

        case ComputerAction.SEARCH:
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: ['ctrl+l'],
          });
          break;

        case ComputerAction.NAVIGATE: {
          if (!args.url) {
            return { error: 'navigate requires url' };
          }
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: ['ctrl+l'],
          });
          await this.sleep(100);
          await this.kernel.browsers.computer.typeText(this.sessionId, {
            text: args.url,
            delay: TYPING_DELAY_MS,
          });
          await this.sleep(100);
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: ['Return'],
          });
          await this.sleep(1500);
          break;
        }

        case ComputerAction.KEY_COMBINATION: {
          if (!args.keys) {
            return { error: 'key_combination requires keys' };
          }
          const keyValue = String(args.keys).toLowerCase() === 'enter' ? 'Return' : args.keys;
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: [keyValue],
          });
          break;
        }

        case ComputerAction.DRAG_AND_DROP: {
          if (args.x === undefined || args.y === undefined ||
              args.destination_x === undefined || args.destination_y === undefined) {
            return { error: 'drag_and_drop requires x, y, destination_x, and destination_y' };
          }

          const startX = this.denormalizeX(args.x);
          const startY = this.denormalizeY(args.y);
          const endX = this.denormalizeX(args.destination_x);
          const endY = this.denormalizeY(args.destination_y);

          await this.kernel.browsers.computer.dragMouse(this.sessionId, {
            path: [[startX, startY], [endX, endY]],
            button: 'left',
          });
          break;
        }

        default:
          return { error: `Unhandled action: ${actionName}` };
      }

      await this.sleep(SCREENSHOT_DELAY_MS);
      return await this.screenshot();

    } catch (error) {
      return { error: `Action failed: ${error}`, url: 'about:blank' };
    }
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}

function parsePngDimensions(buffer: Buffer): ScreenSize | null {
  if (buffer.length < 24) return null;
  const signature = buffer.subarray(0, 8);
  const expected = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  if (!signature.equals(expected)) return null;
  const width = buffer.readUInt32BE(16);
  const height = buffer.readUInt32BE(20);
  if (!width || !height) return null;
  return { width, height };
}
