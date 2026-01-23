/**
 * Type definitions for Gemini Computer Use actions.
 * Based on Google's computer-use-preview reference implementation.
 */

export enum GeminiAction {
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

export const PREDEFINED_COMPUTER_USE_FUNCTIONS = [
  GeminiAction.OPEN_WEB_BROWSER,
  GeminiAction.CLICK_AT,
  GeminiAction.HOVER_AT,
  GeminiAction.TYPE_TEXT_AT,
  GeminiAction.SCROLL_DOCUMENT,
  GeminiAction.SCROLL_AT,
  GeminiAction.WAIT_5_SECONDS,
  GeminiAction.GO_BACK,
  GeminiAction.GO_FORWARD,
  GeminiAction.SEARCH,
  GeminiAction.NAVIGATE,
  GeminiAction.KEY_COMBINATION,
  GeminiAction.DRAG_AND_DROP,
] as const;

export type ScrollDirection = 'up' | 'down' | 'left' | 'right';

export interface GeminiFunctionArgs {
  // click_at, hover_at, scroll_at
  x?: number;
  y?: number;

  // type_text_at
  text?: string;
  press_enter?: boolean;
  clear_before_typing?: boolean;

  // scroll_document, scroll_at
  direction?: ScrollDirection;
  magnitude?: number;

  // navigate
  url?: string;

  // key_combination
  keys?: string;

  // drag_and_drop
  destination_x?: number;
  destination_y?: number;

  // Safety decision (may be included in any function call)
  safety_decision?: {
    decision: string;
    explanation: string;
  };
}

export interface ToolResult {
  /** Base64-encoded screenshot image */
  base64Image?: string;
  /** Current URL of the browser */
  url?: string;
  /** Error message if the action failed */
  error?: string;
}

export interface ScreenSize {
  width: number;
  height: number;
}

export const DEFAULT_SCREEN_SIZE: ScreenSize = {
  width: 1200,
  height: 800,
};

// Gemini uses normalized coordinates (0-1000)
export const COORDINATE_SCALE = 1000;
