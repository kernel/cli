import type {
  BetaMessageParam as AnthropicMessageParam,
  BetaMessage as AnthropicMessage,
  BetaContentBlock as AnthropicContentBlock,
} from '@anthropic-ai/sdk/resources/beta/messages/messages';

// Re-export the SDK types
export type BetaMessageParam = AnthropicMessageParam;
export type BetaMessage = AnthropicMessage;
export type BetaContentBlock = AnthropicContentBlock;

// Action types
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

export interface BaseAnthropicTool {
  name: string;
  apiType: string;
  toParams(): ActionParams;
}

export class ToolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ToolError';
  }
}

// Local content block types
export interface BetaTextBlock {
  type: 'text';
  text: string;
  id?: string;
  cache_control?: { type: 'ephemeral' };
}

export interface BetaImageBlock {
  type: 'image';
  source: {
    type: 'base64';
    media_type: 'image/png';
    data: string;
  };
  id?: string;
  cache_control?: { type: 'ephemeral' };
}

export interface BetaToolUseBlock {
  type: 'tool_use';
  name: string;
  input: ActionParams;
  id?: string;
  cache_control?: { type: 'ephemeral' };
}

export interface BetaThinkingBlock {
  type: 'thinking';
  thinking:
    | {
        type: 'enabled';
        budget_tokens: number;
      }
    | {
        type: 'disabled';
      };
  signature?: string;
  id?: string;
  cache_control?: { type: 'ephemeral' };
}

export interface BetaToolResultBlock {
  type: 'tool_result';
  content: (BetaTextBlock | BetaImageBlock)[] | string;
  tool_use_id: string;
  is_error: boolean;
  id?: string;
  cache_control?: { type: 'ephemeral' };
}

export type BetaLocalContentBlock =
  | BetaTextBlock
  | BetaImageBlock
  | BetaToolUseBlock
  | BetaThinkingBlock
  | BetaToolResultBlock;
