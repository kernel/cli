export type AgentEventName =
  | 'session_state'
  | 'backend'
  | 'prompt'
  | 'reasoning_delta'
  | 'text_delta'
  | 'text_done'
  | 'action'
  | 'screenshot'
  | 'turn_done'
  | 'run_complete'
  | 'error';

export interface AgentEvent {
  event: AgentEventName;
  data: Record<string, unknown>;
}

function toInt(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) return Math.trunc(value);
  return 0;
}

function truncate(text: string, max = 60): string {
  return text.length > max ? `${text.slice(0, max - 3)}...` : text;
}

export function describeAction(actionType: string, actionArgs: Record<string, unknown>): string {
  switch (actionType) {
    case 'click': {
      const x = toInt(actionArgs.x);
      const y = toInt(actionArgs.y);
      const button = typeof actionArgs.button === 'string' ? actionArgs.button : 'left';
      return button === 'left' ? `click(${x}, ${y})` : `click(${x}, ${y}, ${button})`;
    }
    case 'double_click':
      return `double_click(${toInt(actionArgs.x)}, ${toInt(actionArgs.y)})`;
    case 'type': {
      const text = typeof actionArgs.text === 'string' ? actionArgs.text : '';
      return `type(${JSON.stringify(truncate(text))})`;
    }
    case 'keypress': {
      const keys = Array.isArray(actionArgs.keys) ? actionArgs.keys : [];
      const holdKeys = Array.isArray(actionArgs.hold_keys) ? actionArgs.hold_keys : [];
      const serializedKeys = keys.filter((k): k is string => typeof k === 'string');
      const serializedHoldKeys = holdKeys.filter((k): k is string => typeof k === 'string');
      if (serializedHoldKeys.length > 0) {
        return `keypress(hold=${JSON.stringify(serializedHoldKeys)}, keys=${JSON.stringify(serializedKeys)})`;
      }
      return `keypress(${JSON.stringify(serializedKeys)})`;
    }
    case 'scroll':
      return `scroll(${toInt(actionArgs.x)}, ${toInt(actionArgs.y)}, dx=${toInt(actionArgs.scroll_x)}, dy=${toInt(actionArgs.scroll_y)})`;
    case 'move':
      return `move(${toInt(actionArgs.x)}, ${toInt(actionArgs.y)})`;
    case 'drag':
      return 'drag(...)';
    case 'wait': {
      const ms = typeof actionArgs.ms === 'number' ? Math.trunc(actionArgs.ms) : 1000;
      return `wait(${ms}ms)`;
    }
    case 'goto': {
      const url = typeof actionArgs.url === 'string' ? actionArgs.url : '';
      return `goto(${JSON.stringify(url)})`;
    }
    case 'back':
      return 'back()';
    case 'url':
      return 'url()';
    case 'screenshot':
      return 'screenshot()';
    default:
      return actionType;
  }
}

export function describeBatchActions(actions: Array<Record<string, unknown>>): string {
  const pieces = actions.map((action) => {
    const actionType = typeof action.type === 'string' ? action.type : 'unknown';
    const { type: _ignored, ...actionArgs } = action;
    return describeAction(actionType, actionArgs);
  });
  return `batch[${pieces.join(' -> ')}]`;
}
