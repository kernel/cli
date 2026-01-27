/**
 * Gemini Computer Tool - Maps Gemini actions to Kernel Computer Controls API.
 * Based on Google's computer-use-preview reference implementation.
 */

import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';
import {
  GeminiAction,
  PREDEFINED_COMPUTER_USE_FUNCTIONS,
  DEFAULT_SCREEN_SIZE,
  COORDINATE_SCALE,
  type GeminiFunctionArgs,
  type ToolResult,
  type ScreenSize,
} from './types';

const TYPING_DELAY_MS = 12;
const SCREENSHOT_DELAY_MS = 500;

/**
 * Computer tool that maps Gemini actions to Kernel's Computer Controls API.
 */
export class ComputerTool {
  private kernel: Kernel;
  private sessionId: string;
  private screenSize: ScreenSize;

  constructor(kernel: Kernel, sessionId: string, screenSize: ScreenSize = DEFAULT_SCREEN_SIZE) {
    this.kernel = kernel;
    this.sessionId = sessionId;
    this.screenSize = screenSize;
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

      // Get current URL
      let url = '';
      try {
        const state = await this.kernel.browsers.computer.getState(this.sessionId);
        url = state.url || '';
      } catch {
        // Ignore URL fetch errors
      }

      return {
        base64Image: buffer.toString('base64'),
        url,
      };
    } catch (error) {
      return {
        error: `Failed to take screenshot: ${error}`,
      };
    }
  }

  async executeAction(actionName: string, args: GeminiFunctionArgs): Promise<ToolResult> {
    // Check if this is a known computer use function
    if (!PREDEFINED_COMPUTER_USE_FUNCTIONS.includes(actionName as GeminiAction)) {
      return { error: `Unknown action: ${actionName}` };
    }

    try {
      switch (actionName) {
        case GeminiAction.OPEN_WEB_BROWSER:
          // Browser is already open in Kernel, just return screenshot
          break;

        case GeminiAction.CLICK_AT: {
          if (args.x === undefined || args.y === undefined) {
            return { error: 'click_at requires x and y coordinates' };
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
          break;
        }

        case GeminiAction.HOVER_AT: {
          if (args.x === undefined || args.y === undefined) {
            return { error: 'hover_at requires x and y coordinates' };
          }
          const x = this.denormalizeX(args.x);
          const y = this.denormalizeY(args.y);
          await this.kernel.browsers.computer.moveMouse(this.sessionId, { x, y });
          break;
        }

        case GeminiAction.TYPE_TEXT_AT: {
          if (args.x === undefined || args.y === undefined) {
            return { error: 'type_text_at requires x and y coordinates' };
          }
          if (!args.text) {
            return { error: 'type_text_at requires text' };
          }

          const x = this.denormalizeX(args.x);
          const y = this.denormalizeY(args.y);

          // Click at the location first
          await this.kernel.browsers.computer.clickMouse(this.sessionId, {
            x,
            y,
            button: 'left',
            click_type: 'click',
            num_clicks: 1,
          });

          // Clear existing text if requested (default: true)
          if (args.clear_before_typing !== false) {
            await this.kernel.browsers.computer.pressKey(this.sessionId, {
              keys: ['ctrl+a'],
            });
            await this.sleep(50);
          }

          // Type the text
          await this.kernel.browsers.computer.typeText(this.sessionId, {
            text: args.text,
            delay: TYPING_DELAY_MS,
          });

          // Press enter if requested
          if (args.press_enter) {
            await this.sleep(100);
            await this.kernel.browsers.computer.pressKey(this.sessionId, {
              keys: ['Return'],
            });
          }
          break;
        }

        case GeminiAction.SCROLL_DOCUMENT: {
          if (!args.direction) {
            return { error: 'scroll_document requires direction' };
          }
          // Scroll at center of viewport
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

        case GeminiAction.SCROLL_AT: {
          if (args.x === undefined || args.y === undefined) {
            return { error: 'scroll_at requires x and y coordinates' };
          }
          if (!args.direction) {
            return { error: 'scroll_at requires direction' };
          }

          const x = this.denormalizeX(args.x);
          const y = this.denormalizeY(args.y);

          // Denormalize magnitude if provided
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

        case GeminiAction.WAIT_5_SECONDS:
          await this.sleep(5000);
          break;

        case GeminiAction.GO_BACK:
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: ['alt+Left'],
          });
          await this.sleep(1000);
          break;

        case GeminiAction.GO_FORWARD:
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: ['alt+Right'],
          });
          await this.sleep(1000);
          break;

        case GeminiAction.SEARCH:
          // Focus URL bar (Ctrl+L) - equivalent to clicking search
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: ['ctrl+l'],
          });
          break;

        case GeminiAction.NAVIGATE: {
          if (!args.url) {
            return { error: 'navigate requires url' };
          }
          // Focus URL bar and type the URL
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
          await this.sleep(1500); // Wait for navigation
          break;
        }

        case GeminiAction.KEY_COMBINATION: {
          if (!args.keys) {
            return { error: 'key_combination requires keys' };
          }
          // Gemini sends keys as "key1+key2+key3"
          await this.kernel.browsers.computer.pressKey(this.sessionId, {
            keys: [args.keys],
          });
          break;
        }

        case GeminiAction.DRAG_AND_DROP: {
          if (
            args.x === undefined ||
            args.y === undefined ||
            args.destination_x === undefined ||
            args.destination_y === undefined
          ) {
            return { error: 'drag_and_drop requires x, y, destination_x, and destination_y' };
          }

          const startX = this.denormalizeX(args.x);
          const startY = this.denormalizeY(args.y);
          const endX = this.denormalizeX(args.destination_x);
          const endY = this.denormalizeY(args.destination_y);

          await this.kernel.browsers.computer.dragMouse(this.sessionId, {
            path: [
              [startX, startY],
              [endX, endY],
            ],
            button: 'left',
          });
          break;
        }

        default:
          return { error: `Unhandled action: ${actionName}` };
      }

      // Wait a moment for the action to complete, then take a screenshot
      await this.sleep(SCREENSHOT_DELAY_MS);
      return await this.screenshot();
    } catch (error) {
      // Even on error, try to get the current URL (required by Gemini Computer Use API)
      let url = '';
      try {
        const state = await this.kernel.browsers.computer.getState(this.sessionId);
        url = state.url || '';
      } catch {
        // Ignore URL fetch errors
      }
      return { error: `Action failed: ${error}`, url };
    }
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
