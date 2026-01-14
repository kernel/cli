import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';
import type { ActionParams, BaseAnthropicTool, ToolResult } from './types/computer';
import { Action, ToolError } from './types/computer';
import { KeyboardUtils } from './utils/keyboard';
import { ActionValidator } from './utils/validator';

const TYPING_DELAY_MS = 12;

export class ComputerTool implements BaseAnthropicTool {
  name: 'computer' = 'computer';
  protected kernel: Kernel;
  protected sessionId: string;
  protected _screenshotDelay = 2.0;
  protected version: '20241022' | '20250124';
  
  // Track the last known mouse position for drag operations
  private lastMousePosition: [number, number] = [0, 0];

  private readonly mouseActions = new Set([
    Action.LEFT_CLICK,
    Action.RIGHT_CLICK,
    Action.MIDDLE_CLICK,
    Action.DOUBLE_CLICK,
    Action.TRIPLE_CLICK,
    Action.MOUSE_MOVE,
    Action.LEFT_MOUSE_DOWN,
    Action.LEFT_MOUSE_UP,
  ]);

  private readonly keyboardActions = new Set([
    Action.KEY,
    Action.TYPE,
    Action.HOLD_KEY,
  ]);

  private readonly systemActions = new Set([
    Action.SCREENSHOT,
    Action.CURSOR_POSITION,
    Action.SCROLL,
    Action.WAIT,
  ]);

  constructor(kernel: Kernel, sessionId: string, version: '20241022' | '20250124' = '20250124') {
    this.kernel = kernel;
    this.sessionId = sessionId;
    this.version = version;
  }

  get apiType(): 'computer_20241022' | 'computer_20250124' {
    return this.version === '20241022' ? 'computer_20241022' : 'computer_20250124';
  }

  toParams(): ActionParams {
    const params = {
      name: this.name,
      type: this.apiType,
      display_width_px: 1920,
      display_height_px: 1080,
      display_number: null,
    };
    return params;
  }

  private getMouseButton(action: Action): 'left' | 'right' | 'middle' {
    switch (action) {
      case Action.LEFT_CLICK:
      case Action.DOUBLE_CLICK:
      case Action.TRIPLE_CLICK:
      case Action.LEFT_CLICK_DRAG:
      case Action.LEFT_MOUSE_DOWN:
      case Action.LEFT_MOUSE_UP:
        return 'left';
      case Action.RIGHT_CLICK:
        return 'right';
      case Action.MIDDLE_CLICK:
        return 'middle';
      default:
        throw new ToolError(`Invalid mouse action: ${action}`);
    }
  }

  private async handleMouseAction(action: Action, coordinate: [number, number]): Promise<ToolResult> {
    const [x, y] = ActionValidator.validateAndGetCoordinates(coordinate);

    if (action === Action.MOUSE_MOVE) {
      await this.kernel.browsers.computer.moveMouse(this.sessionId, {
        x,
        y,
      });
      // Track mouse position for drag operations
      this.lastMousePosition = [x, y];
    } else if (action === Action.LEFT_MOUSE_DOWN) {
      await this.kernel.browsers.computer.clickMouse(this.sessionId, {
        x,
        y,
        button: 'left',
        click_type: 'down',
      });
      this.lastMousePosition = [x, y];
    } else if (action === Action.LEFT_MOUSE_UP) {
      await this.kernel.browsers.computer.clickMouse(this.sessionId, {
        x,
        y,
        button: 'left',
        click_type: 'up',
      });
      this.lastMousePosition = [x, y];
    } else {
      const button = this.getMouseButton(action);
      let numClicks = 1;
      if (action === Action.DOUBLE_CLICK) {
        numClicks = 2;
      } else if (action === Action.TRIPLE_CLICK) {
        numClicks = 3;
      }

      await this.kernel.browsers.computer.clickMouse(this.sessionId, {
        x,
        y,
        button,
        click_type: 'click',
        num_clicks: numClicks,
      });
      // Track mouse position for drag operations
      this.lastMousePosition = [x, y];
    }

    await new Promise(resolve => setTimeout(resolve, 500));
    return await this.screenshot();
  }

  private async handleKeyboardAction(action: Action, text: string, duration?: number): Promise<ToolResult> {
    if (action === Action.HOLD_KEY) {
      // For HOLD_KEY, we need to press and hold for the duration
      // OnKernel doesn't have a direct hold API, so we'll use pressKey with duration
      const key = this.convertToOnKernelKey(text);
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: [key],
        duration: duration ? duration * 1000 : undefined,
      });
    } else if (action === Action.KEY) {
      // Convert key combination to OnKernel format (e.g., "Ctrl+t")
      const key = this.convertKeyCombinationToOnKernel(text);
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: [key],
      });
    } else {
      // TYPE action - use typeText
      await this.kernel.browsers.computer.typeText(this.sessionId, {
        text,
        delay: TYPING_DELAY_MS,
      });
    }

    await new Promise(resolve => setTimeout(resolve, 500));
    return await this.screenshot();
  }

  private convertToOnKernelKey(key: string): string {
    // Convert Playwright key names to OnKernel format
    const keyMap: Record<string, string> = {
      'Control': 'Ctrl',
      'Meta': 'Meta',
      'Alt': 'Alt',
      'Shift': 'Shift',
      'Enter': 'Enter',
      'ArrowLeft': 'ArrowLeft',
      'ArrowRight': 'ArrowRight',
      'ArrowUp': 'ArrowUp',
      'ArrowDown': 'ArrowDown',
      'Home': 'Home',
      'End': 'End',
      'PageUp': 'PageUp',
      'PageDown': 'PageDown',
      'Delete': 'Delete',
      'Backspace': 'Backspace',
      'Tab': 'Tab',
      'Escape': 'Escape',
      'Insert': 'Insert',
    };
    return keyMap[key] || key;
  }

  private convertKeyCombinationToOnKernel(combo: string): string {
    // Convert key combinations like "Control+t" to "Ctrl+t"
    const parts = combo.split('+').map(part => {
      const trimmed = part.trim();
      if (trimmed.toLowerCase() === 'control' || trimmed.toLowerCase() === 'ctrl') {
        return 'Ctrl';
      }
      if (trimmed.toLowerCase() === 'meta' || trimmed.toLowerCase() === 'command' || trimmed.toLowerCase() === 'cmd') {
        return 'Meta';
      }
      return trimmed;
    });
    return parts.join('+');
  }

  async screenshot(): Promise<ToolResult> {
    try {
      console.log('Starting screenshot...');
      await new Promise(resolve => setTimeout(resolve, this._screenshotDelay * 1000));
      const response = await this.kernel.browsers.computer.captureScreenshot(this.sessionId);
      const blob = await response.blob();
      const arrayBuffer = await blob.arrayBuffer();
      const buffer = Buffer.from(arrayBuffer);
      console.log('Screenshot taken, size:', buffer.length, 'bytes');

      return {
        base64Image: buffer.toString('base64'),
      };
    } catch (error) {
      throw new ToolError(`Failed to take screenshot: ${error}`);
    }
  }

  async call(params: ActionParams): Promise<ToolResult> {
    const {
      action,
      text,
      coordinate,
      scrollDirection: scrollDirectionParam,
      scroll_amount,
      scrollAmount,
      duration,
      ...kwargs
    } = params;

    ActionValidator.validateActionParams(params, this.mouseActions, this.keyboardActions);

    if (action === Action.SCREENSHOT) {
      return await this.screenshot();
    }

    if (action === Action.CURSOR_POSITION) {
      // OnKernel computer controls don't have a direct cursor position API
      // This would need to be handled differently or removed
      // For now, we'll return an error indicating this feature isn't available
      throw new ToolError('Cursor position is not available with OnKernel computer controls API');
    }

    if (action === Action.SCROLL) {
      if (this.version !== '20250124') {
        throw new ToolError(`${action} is only available in version 20250124`);
      }

      const scrollDirection = scrollDirectionParam || kwargs.scroll_direction;
      const scrollAmountValue = scrollAmount || scroll_amount;

      if (!scrollDirection || !['up', 'down', 'left', 'right'].includes(scrollDirection)) {
        throw new ToolError(`Scroll direction "${scrollDirection}" must be 'up', 'down', 'left', or 'right'`);
      }
      if (typeof scrollAmountValue !== 'number' || scrollAmountValue < 0) {
        throw new ToolError(`Scroll amount "${scrollAmountValue}" must be a non-negative number`);
      }

      const [x, y] = coordinate 
        ? ActionValidator.validateAndGetCoordinates(coordinate)
        : [0, 0]; // Default to top-left if no coordinate provided

      // Convert scroll direction and amount to delta_x and delta_y
      // OnKernel uses positive delta_y for scrolling down, negative for up
      // Positive delta_x for scrolling right, negative for left
      let delta_x = 0;
      let delta_y = 0;
      const scrollDelta = scrollAmountValue || 120; // Default scroll amount

      if (scrollDirection === 'down') {
        delta_y = scrollDelta;
      } else if (scrollDirection === 'up') {
        delta_y = -scrollDelta;
      } else if (scrollDirection === 'right') {
        delta_x = scrollDelta;
      } else if (scrollDirection === 'left') {
        delta_x = -scrollDelta;
      }

      await this.kernel.browsers.computer.scroll(this.sessionId, {
        x,
        y,
        delta_x,
        delta_y,
      });

      await new Promise(resolve => setTimeout(resolve, 500));
      return await this.screenshot();
    }

    if (action === Action.WAIT) {
      if (this.version !== '20250124') {
        throw new ToolError(`${action} is only available in version 20250124`);
      }
      await new Promise(resolve => setTimeout(resolve, duration! * 1000));
      return await this.screenshot();
    }

    if (action === Action.LEFT_CLICK_DRAG) {
      if (!coordinate) {
        throw new ToolError(`coordinate is required for ${action}`);
      }
      
      // Get the destination coordinate
      const [endX, endY] = ActionValidator.validateAndGetCoordinates(coordinate);
      
      // Check if start_coordinate is provided in kwargs (for newer API versions)
      let startX: number, startY: number;
      const startCoordinate = kwargs.start_coordinate as [number, number] | undefined;
      
      if (startCoordinate) {
        [startX, startY] = ActionValidator.validateAndGetCoordinates(startCoordinate);
      } else {
        // Use last known mouse position as the start point
        [startX, startY] = this.lastMousePosition;
      }
      
      console.log(`Dragging from (${startX}, ${startY}) to (${endX}, ${endY})`);
      
      // Use Kernel's dragMouse API with a path from start to end
      await this.kernel.browsers.computer.dragMouse(this.sessionId, {
        path: [[startX, startY], [endX, endY]],
        button: 'left',
      });
      
      // Update tracked mouse position to the end of the drag
      this.lastMousePosition = [endX, endY];
      
      await new Promise(resolve => setTimeout(resolve, 500));
      return await this.screenshot();
    }

    if (this.mouseActions.has(action)) {
      if (!coordinate) {
        throw new ToolError(`coordinate is required for ${action}`);
      }
      return await this.handleMouseAction(action, coordinate);
    }

    if (this.keyboardActions.has(action)) {
      if (!text) {
        throw new ToolError(`text is required for ${action}`);
      }
      return await this.handleKeyboardAction(action, text, duration);
    }

    throw new ToolError(`Invalid action: ${action}`);
  }
}

// For backward compatibility
export class ComputerTool20241022 extends ComputerTool {
  constructor(kernel: Kernel, sessionId: string) {
    super(kernel, sessionId, '20241022');
  }
}

export class ComputerTool20250124 extends ComputerTool {
  constructor(kernel: Kernel, sessionId: string) {
    super(kernel, sessionId, '20250124');
  }
}
