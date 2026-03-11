import { Kernel } from '@onkernel/sdk';
import { describeAction, type AgentEvent } from './log-events';

// CUA model key names -> X11 keysym names for the Kernel computer API
const KEYSYM_MAP: Record<string, string> = {
  ENTER: 'Return',
  Enter: 'Return',
  RETURN: 'Return',
  BACKSPACE: 'BackSpace',
  Backspace: 'BackSpace',
  DELETE: 'Delete',
  TAB: 'Tab',
  ESCAPE: 'Escape',
  Escape: 'Escape',
  ESC: 'Escape',
  SPACE: 'space',
  Space: 'space',
  UP: 'Up',
  DOWN: 'Down',
  LEFT: 'Left',
  RIGHT: 'Right',
  HOME: 'Home',
  END: 'End',
  PAGEUP: 'Prior',
  PAGE_UP: 'Prior',
  PageUp: 'Prior',
  PAGEDOWN: 'Next',
  PAGE_DOWN: 'Next',
  PageDown: 'Next',
  CAPS_LOCK: 'Caps_Lock',
  CapsLock: 'Caps_Lock',
  CTRL: 'Control_L',
  Ctrl: 'Control_L',
  CONTROL: 'Control_L',
  Control: 'Control_L',
  ALT: 'Alt_L',
  Alt: 'Alt_L',
  SHIFT: 'Shift_L',
  Shift: 'Shift_L',
  META: 'Super_L',
  Meta: 'Super_L',
  SUPER: 'Super_L',
  Super: 'Super_L',
  CMD: 'Super_L',
  COMMAND: 'Super_L',
  F1: 'F1',
  F2: 'F2',
  F3: 'F3',
  F4: 'F4',
  F5: 'F5',
  F6: 'F6',
  F7: 'F7',
  F8: 'F8',
  F9: 'F9',
  F10: 'F10',
  F11: 'F11',
  F12: 'F12',
  INSERT: 'Insert',
  Insert: 'Insert',
  PRINT: 'Print',
  SCROLLLOCK: 'Scroll_Lock',
  PAUSE: 'Pause',
  NUMLOCK: 'Num_Lock',
};

const MODIFIER_KEYSYMS = new Set([
  'Control_L',
  'Control_R',
  'Alt_L',
  'Alt_R',
  'Shift_L',
  'Shift_R',
  'Super_L',
  'Super_R',
  'Meta_L',
  'Meta_R',
]);
const GOTO_CHORD_DELAY_MS = 200;

function translateKeys(keys: string[]): string[] {
  return keys.map((k) => KEYSYM_MAP[k] ?? k);
}

function expandComboKeys(keys: string[]): string[] {
  const out: string[] = [];
  for (const raw of keys) {
    if (typeof raw !== 'string') continue;
    const parts = raw.includes('+') ? raw.split('+') : [raw];
    for (const part of parts) {
      const trimmed = part.trim();
      if (trimmed) out.push(trimmed);
    }
  }
  return out;
}

function normalizeKeypressPayload(
  keys: string[] = [],
  holdKeys: string[] = [],
): { keys: string[]; holdKeys: string[] } {
  const translatedHoldKeys = translateKeys(expandComboKeys(holdKeys));
  const translatedKeyEntries = translateKeys(expandComboKeys(keys));

  const holdFromKeys: string[] = [];
  const primaryKeys: string[] = [];
  for (const key of translatedKeyEntries) {
    if (MODIFIER_KEYSYMS.has(key)) holdFromKeys.push(key);
    else primaryKeys.push(key);
  }

  if (primaryKeys.length === 0) {
    return { keys: translatedKeyEntries, holdKeys: translatedHoldKeys };
  }

  const holdMerged = [...translatedHoldKeys, ...holdFromKeys];
  const dedupedHold: string[] = [];
  for (const key of holdMerged) {
    if (!dedupedHold.includes(key)) dedupedHold.push(key);
  }
  return { keys: primaryKeys, holdKeys: dedupedHold };
}

function pixelsToScrollTicks(delta: number | undefined): number {
  const value = typeof delta === 'number' && Number.isFinite(delta) ? delta : 0;
  return Math.trunc(value);
}

export interface CuaAction {
  type: string;
  x?: number;
  y?: number;
  text?: string;
  url?: string;
  keys?: string[];
  hold_keys?: string[];
  button?: string | number;
  scroll_x?: number;
  scroll_y?: number;
  ms?: number;
  path?: Array<{ x: number; y: number }>;
  [key: string]: unknown;
}

type BatchAction = {
  type: 'click_mouse' | 'move_mouse' | 'type_text' | 'press_key' | 'scroll' | 'drag_mouse' | 'sleep';
  click_mouse?: { x: number; y: number; button?: string; num_clicks?: number };
  move_mouse?: { x: number; y: number };
  type_text?: { text: string };
  press_key?: { keys: string[]; hold_keys?: string[] };
  scroll?: { x: number; y: number; delta_x?: number; delta_y?: number };
  drag_mouse?: { path: number[][] };
  sleep?: { duration_ms: number };
};

function normalizeButton(button?: string | number): string {
  if (button === undefined || button === null) return 'left';
  if (typeof button === 'number') {
    switch (button) {
      case 1: return 'left';
      case 2: return 'middle';
      case 3: return 'right';
      default: return 'left';
    }
  }
  return button;
}

function translateCuaAction(action: CuaAction): BatchAction {
  switch (action.type) {
    case 'click':
      return {
        type: 'click_mouse',
        click_mouse: { x: action.x ?? 0, y: action.y ?? 0, button: normalizeButton(action.button) },
      };
    case 'double_click':
      return {
        type: 'click_mouse',
        click_mouse: { x: action.x ?? 0, y: action.y ?? 0, num_clicks: 2 },
      };
    case 'type':
      return { type: 'type_text', type_text: { text: action.text ?? '' } };
    case 'keypress': {
      const normalized = normalizeKeypressPayload(action.keys ?? [], action.hold_keys ?? []);
      return {
        type: 'press_key',
        press_key: {
          keys: normalized.keys,
          ...(normalized.holdKeys.length > 0 ? { hold_keys: normalized.holdKeys } : {}),
        },
      };
    }
    case 'scroll':
      return {
        type: 'scroll',
        scroll: {
          x: action.x ?? 0,
          y: action.y ?? 0,
          delta_x: pixelsToScrollTicks(action.scroll_x),
          delta_y: pixelsToScrollTicks(action.scroll_y),
        },
      };
    case 'move':
      return { type: 'move_mouse', move_mouse: { x: action.x ?? 0, y: action.y ?? 0 } };
    case 'drag': {
      const path = (action.path ?? []).map((p) => [p.x, p.y]);
      return { type: 'drag_mouse', drag_mouse: { path } };
    }
    case 'wait':
      return { type: 'sleep', sleep: { duration_ms: action.ms ?? 1000 } };
    default:
      throw new Error(`Unknown CUA action type: ${action.type}`);
  }
}

function isBatchComputerActionType(actionType: string): boolean {
  return ['click', 'double_click', 'type', 'keypress', 'scroll', 'move', 'drag', 'wait'].includes(
    actionType,
  );
}

function gotoBatchActions(url: string): BatchAction[] {
  return [
    { type: 'press_key', press_key: { hold_keys: ['Ctrl'], keys: ['l'] } },
    { type: 'sleep', sleep: { duration_ms: GOTO_CHORD_DELAY_MS } },
    { type: 'press_key', press_key: { hold_keys: ['Ctrl'], keys: ['a'] } },
    { type: 'type_text', type_text: { text: url } },
    { type: 'press_key', press_key: { keys: ['Return'] } },
  ];
}

function backBatchActions(): BatchAction[] {
  return [
    { type: 'press_key', press_key: { hold_keys: ['Alt'], keys: ['Left'] } },
  ];
}

function validateBatchTerminalReadActions(actions: CuaAction[]): void {
  let readIdx = -1;
  let readType = '';
  actions.forEach((action, idx) => {
    if (action.type !== 'url' && action.type !== 'screenshot') return;
    if (readIdx >= 0) {
      throw new Error(
        `batch can include at most one return-value action (${readType} or ${action.type}); found ${readType} at index ${readIdx} and ${action.type} at index ${idx}`,
      );
    }
    if (idx !== actions.length - 1) {
      throw new Error(`return-value action "${action.type}" must be last in batch`);
    }
    readIdx = idx;
    readType = action.type;
  });
}

function buildPendingBatch(actions: CuaAction[]): BatchAction[] {
  const pending: BatchAction[] = [];
  for (const action of actions) {
    const actionType = action.type;
    if (isBatchComputerActionType(actionType)) {
      pending.push(translateCuaAction(action));
      continue;
    }
    if (actionType === 'goto') {
      pending.push(...gotoBatchActions(action.url ?? ''));
      continue;
    }
    if (actionType === 'back') {
      pending.push(...backBatchActions());
      continue;
    }
    if (actionType === 'url' || actionType === 'screenshot') {
      continue;
    }
    throw new Error(`Unknown CUA action type: ${actionType}`);
  }
  return pending;
}

function truncateText(text: string, max = 30): string {
  if (text.length <= max) return text;
  return `${text.slice(0, max - 3)}...`;
}

function describeTranslatedBatch(actions: BatchAction[]): string {
  const parts = actions.map((action) => {
    switch (action.type) {
      case 'click_mouse': {
        const click = action.click_mouse;
        if (!click) return action.type;
        if ((click.num_clicks ?? 0) > 1) return `double_click(${click.x},${click.y})`;
        return `click(${click.x},${click.y})`;
      }
      case 'type_text': {
        const text = action.type_text?.text ?? '';
        return `type(${JSON.stringify(truncateText(text))})`;
      }
      case 'press_key':
        return `key(hold=${JSON.stringify(action.press_key?.hold_keys ?? [])}, keys=${JSON.stringify(action.press_key?.keys ?? [])})`;
      case 'scroll':
        return 'scroll';
      case 'move_mouse':
        return 'move';
      case 'drag_mouse':
        return 'drag';
      case 'sleep':
        return `sleep(${action.sleep?.duration_ms ?? 0}ms)`;
      default:
        return action.type;
    }
  });
  return `batch[${parts.join(' -> ')}]`;
}

export class KernelComputer {
  private client: Kernel;
  private sessionId: string;
  private width = 1920;
  private height = 1080;
  private onEvent: ((event: AgentEvent) => void) | null;

  constructor(client: Kernel, sessionId: string, onEvent?: (event: AgentEvent) => void) {
    this.client = client;
    this.sessionId = sessionId;
    this.onEvent = onEvent ?? null;
  }

  getEnvironment(): 'browser' {
    return 'browser';
  }

  getDimensions(): [number, number] {
    return [this.width, this.height];
  }

  private emitBackend(op: string, detail?: string, elapsedMs?: number): void {
    const data: Record<string, unknown> = { op };
    if (detail) data.detail = detail;
    if (typeof elapsedMs === 'number') data.elapsed_ms = elapsedMs;
    this.onEvent?.({ event: 'backend', data });
  }

  private async traceCall<T>(
    op: string,
    fn: () => Promise<T>,
    detail?: string | ((result: T) => string | undefined),
  ): Promise<T> {
    this.emitBackend(op);
    const started = Date.now();
    let result!: T;
    let completed = false;
    try {
      result = await fn();
      completed = true;
      return result;
    } finally {
      const elapsedMs = Date.now() - started;
      let resolvedDetail: string | undefined;
      if (completed) {
        resolvedDetail =
          typeof detail === 'function' ? detail(result) : detail;
      }
      this.emitBackend(`${op}.done`, resolvedDetail, elapsedMs);
    }
  }

  async screenshot(): Promise<string> {
    return this.traceCall('screenshot', async () => {
      const resp = await this.client.browsers.computer.captureScreenshot(this.sessionId);
      const buf = Buffer.from(await resp.arrayBuffer());
      return buf.toString('base64');
    });
  }

  async click(x: number, y: number, button: string | number = 'left'): Promise<void> {
    const normalizedButton = normalizeButton(button) as 'left' | 'right' | 'middle';
    const op = describeAction('click', { x, y, button: normalizedButton });
    await this.traceCall(op, async () => {
      await this.client.browsers.computer.clickMouse(this.sessionId, {
        x,
        y,
        button: normalizedButton,
      });
    });
  }

  async doubleClick(x: number, y: number): Promise<void> {
    const op = describeAction('double_click', { x, y });
    await this.traceCall(op, async () => {
      await this.client.browsers.computer.clickMouse(this.sessionId, { x, y, num_clicks: 2 });
    });
  }

  async type(text: string): Promise<void> {
    const op = describeAction('type', { text });
    await this.traceCall(op, async () => {
      await this.client.browsers.computer.typeText(this.sessionId, { text });
    });
  }

  async keypress(keys: string[], holdKeys: string[] = []): Promise<void> {
    const normalized = normalizeKeypressPayload(keys, holdKeys);
    const op = describeAction('keypress', {
      keys: normalized.keys,
      ...(normalized.holdKeys.length > 0 ? { hold_keys: normalized.holdKeys } : {}),
    });
    await this.traceCall(op, async () => {
      await this.client.browsers.computer.pressKey(
        this.sessionId,
        {
          keys: normalized.keys,
          ...(normalized.holdKeys.length > 0 ? { hold_keys: normalized.holdKeys } : {}),
        } as Parameters<typeof this.client.browsers.computer.pressKey>[1],
      );
    });
  }

  async scroll(x: number, y: number, scrollX: number, scrollY: number): Promise<void> {
    const op = describeAction('scroll', { x, y, scroll_x: scrollX, scroll_y: scrollY });
    const tickX = pixelsToScrollTicks(scrollX);
    const tickY = pixelsToScrollTicks(scrollY);
    await this.traceCall(op, async () => {
      await this.client.browsers.computer.scroll(this.sessionId, {
        x,
        y,
        delta_x: tickX,
        delta_y: tickY,
      });
    });
  }

  async move(x: number, y: number): Promise<void> {
    const op = describeAction('move', { x, y });
    await this.traceCall(op, async () => {
      await this.client.browsers.computer.moveMouse(this.sessionId, { x, y });
    });
  }

  async drag(path: Array<{ x: number; y: number }>): Promise<void> {
    const op = describeAction('drag', { path });
    await this.traceCall(op, async () => {
      const p = path.map((pt) => [pt.x, pt.y]);
      await this.client.browsers.computer.dragMouse(this.sessionId, { path: p });
    });
  }

  async wait(ms = 1000): Promise<void> {
    await new Promise((resolve) => setTimeout(resolve, ms));
  }

  async batchActions(actions: CuaAction[]): Promise<void> {
    validateBatchTerminalReadActions(actions);
    const pending = buildPendingBatch(actions);
    const op = describeTranslatedBatch(pending);
    await this.traceCall(op, async () => {
      if (pending.length === 0) return;
      await this.client.browsers.computer.batch(this.sessionId, {
        actions: pending as Parameters<typeof this.client.browsers.computer.batch>[1]['actions'],
      });
    });
  }

  async goto(url: string): Promise<void> {
    await this.batchActions([{ type: 'goto', url }]);
  }

  async back(): Promise<void> {
    await this.batchActions([{ type: 'back' }]);
  }

  async forward(): Promise<void> {
    await this.traceCall('forward()', async () => {
      await this.client.browsers.playwright.execute(this.sessionId, {
        code: 'await page.goForward()',
      });
    });
  }

  async getCurrentUrl(): Promise<string> {
    return this.traceCall('get_current_url()', async () => {
      const result = await this.client.browsers.playwright.execute(this.sessionId, {
        code: 'return page.url()',
      });
      return (result.result as string) ?? '';
    });
  }
}
