import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';
import type { ActionParams, BaseAnthropicTool, ToolResult } from './types/computer';
import { Action, ToolError } from './types/computer';
import { ActionValidator } from './utils/validator';

const TYPING_DELAY_MS = 12;

export class ComputerTool implements BaseAnthropicTool {
  name: 'computer' = 'computer';
  protected kernel: Kernel;
  protected sessionId: string;
  protected _screenshotDelay = 2.0;
  protected version: '20241022' | '20250124';
  
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
      display_width_px: 1024,
      display_height_px: 768,
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
      this.lastMousePosition = [x, y];
    }

    await new Promise(resolve => setTimeout(resolve, 500));
    return await this.screenshot();
  }

  private async handleKeyboardAction(action: Action, text: string, duration?: number): Promise<ToolResult> {
    if (action === Action.HOLD_KEY) {
      const key = this.convertToKernelKey(text);
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: [key],
        duration: duration ? duration * 1000 : undefined,
      });
    } else if (action === Action.KEY) {
      const key = this.convertKeyCombinationToKernel(text);
      await this.kernel.browsers.computer.pressKey(this.sessionId, {
        keys: [key],
      });
    } else {
      await this.kernel.browsers.computer.typeText(this.sessionId, {
        text,
        delay: TYPING_DELAY_MS,
      });
    }

    await new Promise(resolve => setTimeout(resolve, 500));
    return await this.screenshot();
  }

  // Key mappings for Kernel Computer Controls API (xdotool format)
  private static readonly KEY_MAP: Record<string, string> = {
    // Enter/Return
    'return': 'Return',
    'enter': 'Return',
    'Enter': 'Return',
    // Arrow keys
    'left': 'Left',
    'right': 'Right',
    'up': 'Up',
    'down': 'Down',
    'ArrowLeft': 'Left',
    'ArrowRight': 'Right',
    'ArrowUp': 'Up',
    'ArrowDown': 'Down',
    // Navigation
    'home': 'Home',
    'end': 'End',
    'pageup': 'Page_Up',
    'page_up': 'Page_Up',
    'PageUp': 'Page_Up',
    'pagedown': 'Page_Down',
    'page_down': 'Page_Down',
    'PageDown': 'Page_Down',
    // Editing
    'delete': 'Delete',
    'backspace': 'BackSpace',
    'Backspace': 'BackSpace',
    'tab': 'Tab',
    'insert': 'Insert',
    // Escape
    'esc': 'Escape',
    'escape': 'Escape',
    // Function keys
    'f1': 'F1',
    'f2': 'F2',
    'f3': 'F3',
    'f4': 'F4',
    'f5': 'F5',
    'f6': 'F6',
    'f7': 'F7',
    'f8': 'F8',
    'f9': 'F9',
    'f10': 'F10',
    'f11': 'F11',
    'f12': 'F12',
    // Misc
    'space': 'space',
    'minus': 'minus',
    'equal': 'equal',
    'plus': 'plus',
  };

  // Modifier key mappings (xdotool format)
  private static readonly MODIFIER_MAP: Record<string, string> = {
    'ctrl': 'ctrl',
    'control': 'ctrl',
    'Control': 'ctrl',
    'alt': 'alt',
    'Alt': 'alt',
    'shift': 'shift',
    'Shift': 'shift',
    'meta': 'super',
    'Meta': 'super',
    'cmd': 'super',
    'command': 'super',
    'win': 'super',
    'super': 'super',
  };

  private convertToKernelKey(key: string): string {
    // Check modifier keys first
    if (ComputerTool.MODIFIER_MAP[key]) {
      return ComputerTool.MODIFIER_MAP[key];
    }
    // Check special keys
    if (ComputerTool.KEY_MAP[key]) {
      return ComputerTool.KEY_MAP[key];
    }
    // Return as-is if no mapping exists
    return key;
  }

  private convertKeyCombinationToKernel(combo: string): string {
    // Handle key combinations (e.g., "ctrl+a", "Control+t")
    if (combo.includes('+')) {
      const parts = combo.split('+');
      const mappedParts = parts.map(part => this.convertToKernelKey(part.trim()));
      return mappedParts.join('+');
    }
    // Single key - just convert it
    return this.convertToKernelKey(combo);
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
      throw new ToolError('Cursor position is not available with Kernel Computer Controls API');
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
        : this.lastMousePosition;

      let delta_x = 0;
      let delta_y = 0;
      // Each scroll_amount unit = 1 scroll wheel click ≈ 120 pixels (matches Anthropic's xdotool behavior)
      const scrollDelta = (scrollAmountValue ?? 1) * 120;

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
      
      const [endX, endY] = ActionValidator.validateAndGetCoordinates(coordinate);
      const startCoordinate = kwargs.start_coordinate as [number, number] | undefined;
      const [startX, startY] = startCoordinate 
        ? ActionValidator.validateAndGetCoordinates(startCoordinate)
        : this.lastMousePosition;
      
      console.log(`Dragging from (${startX}, ${startY}) to (${endX}, ${endY})`);
      
      await this.kernel.browsers.computer.dragMouse(this.sessionId, {
        path: [[startX, startY], [endX, endY]],
        button: 'left',
      });
      
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
