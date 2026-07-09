/**
 * Type definitions for computer use actions.
 */

export enum ComputerAction {
  OPEN_WEB_BROWSER = 'open_web_browser',
  CLICK_AT = 'click_at',
  HOVER_AT = 'hover_at',
  TYPE_TEXT_AT = 'type_text_at',
  SCROLL_DOCUMENT = 'scroll_document',
  SCROLL_AT = 'scroll_at',
  WAIT_5_SECONDS = 'wait_5_seconds',
  GO_BACK = 'go_back',
  GO_FORWARD = 'go_forward',
  SEARCH = 'search',
  NAVIGATE = 'navigate',
  KEY_COMBINATION = 'key_combination',
  DRAG_AND_DROP = 'drag_and_drop',
}

export const PREDEFINED_COMPUTER_USE_FUNCTIONS = Object.values(ComputerAction);

export type ScrollDirection = 'up' | 'down' | 'left' | 'right';

export interface ComputerFunctionArgs {
  x?: number;
  y?: number;
  clicks?: number;

  text?: string;
  press_enter?: boolean;
  clear_before_typing?: boolean;

  direction?: ScrollDirection;
  magnitude?: number;

  url?: string;

  keys?: string;

  destination_x?: number;
  destination_y?: number;

  safety_decision?: {
    decision: string;
    explanation: string;
  };
}

export interface ToolResult {
  base64Image?: string;
  url?: string;
  error?: string;
  width?: number;
  height?: number;
}

export interface ScreenSize {
  width: number;
  height: number;
}

export const DEFAULT_SCREEN_SIZE: ScreenSize = {
  width: 1200,
  height: 800,
};

export const COORDINATE_SCALE = 1000;
