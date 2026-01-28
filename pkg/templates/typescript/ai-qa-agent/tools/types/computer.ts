export enum Action {
  // Mouse actions
  MOUSE_MOVE = 'mouse_move',
  LEFT_CLICK = 'left_click',
  RIGHT_CLICK = 'right_click',
  MIDDLE_CLICK = 'middle_click',
  DOUBLE_CLICK = 'double_click',
  TRIPLE_CLICK = 'triple_click',
  LEFT_CLICK_DRAG = 'left_click_drag',
  LEFT_MOUSE_DOWN = 'left_mouse_down',
  LEFT_MOUSE_UP = 'left_mouse_up',

  // Keyboard actions
  KEY = 'key',
  TYPE = 'type',
  HOLD_KEY = 'hold_key',

  // System actions
  SCREENSHOT = 'screenshot',
  CURSOR_POSITION = 'cursor_position',
  SCROLL = 'scroll',
  WAIT = 'wait',
}

export type MouseButton = 'left' | 'right' | 'middle';
export type ScrollDirection = 'up' | 'down' | 'left' | 'right';
export type Coordinate = [number, number];
export type Duration = number;

export interface ActionParams {
  action: Action;
  text?: string;
  coordinate?: Coordinate;
  scrollDirection?: ScrollDirection;
  scroll_amount?: number;
  scrollAmount?: number;
  duration?: Duration;
  key?: string;
  [key: string]: Action | string | Coordinate | ScrollDirection | number | Duration | undefined;
}

export interface ToolResult {
  output?: string;
  error?: string;
  base64Image?: string;
  system?: string;
}

export interface ToolDefinitionParams {
  name: string;
  type: string;
  display_width_px: number;
  display_height_px: number;
  display_number: number | null;
}

export interface BaseAnthropicTool {
  name: string;
  apiType: string;
  toParams(): ToolDefinitionParams;
}

export class ToolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ToolError';
  }
}
