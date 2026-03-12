import {
  type ResponseItem,
  type ResponseInputItem,
  type ResponseOutputMessage,
  type ResponseFunctionToolCallItem,
  type ResponseFunctionToolCallOutputItem,
  type ResponseComputerToolCall,
  type ResponseComputerToolCallOutputItem,
  type Tool,
} from 'openai/resources/responses/responses';

import * as utils from './utils';
import type { AgentEvent } from './log-events';
import { describeAction, describeBatchActions } from './log-events';
import { batchInstructions, batchComputerTool, computerUseExtraTool } from './toolset';
import type { CuaAction, KernelComputer } from './kernel-computer';

const BATCH_FUNC_NAME = 'batch_computer_actions';
const EXTRA_FUNC_NAME = 'computer_use_extra';
// Keep this shape aligned with CUA and current OpenAI Responses API.
const OPENAI_COMPUTER_TOOL = { type: 'computer' } as unknown as Tool;

export class Agent {
  private model: string;
  private computer: KernelComputer;
  private tools: Tool[];
  private print_steps = true;
  private debug = false;
  private show_images = false;
  private ackCb: (msg: string) => boolean;
  private onEvent: ((event: AgentEvent) => void) | null = null;
  private modelRequestStartedAt: number | null = null;

  constructor(opts: {
    model?: string;
    computer: KernelComputer;
    tools?: Tool[];
    acknowledge_safety_check_callback?: (msg: string) => boolean;
  }) {
    this.model = opts.model ?? 'gpt-5.4';
    this.computer = opts.computer;
    this.ackCb = opts.acknowledge_safety_check_callback ?? ((): boolean => true);

    this.tools = [
      OPENAI_COMPUTER_TOOL,
      batchComputerTool,
      computerUseExtraTool,
      ...(opts.tools ?? []),
    ];
  }

  private debugPrint(...args: unknown[]): void {
    if (this.debug) {
      try {
        console.dir(
          args.map((msg) => utils.sanitizeMessage(msg as ResponseItem)),
          { depth: null },
        );
      } catch {
        console.dir(args, { depth: null });
      }
    }
  }

  private emit(event: AgentEvent['event'], data: Record<string, unknown>): void {
    if (this.print_steps) this.onEvent?.({ event, data });
  }

  private currentModelElapsedMs(): number | null {
    return this.modelRequestStartedAt === null ? null : Date.now() - this.modelRequestStartedAt;
  }

  private extractReasoningText(item: Record<string, unknown>): string {
    const summary = item.summary;
    if (!Array.isArray(summary)) return '';
    const chunks = summary
      .map((part) => {
        if (!part || typeof part !== 'object') return '';
        const text = (part as { text?: unknown }).text;
        return typeof text === 'string' ? text : '';
      })
      .filter(Boolean);
    return chunks.join(' ').trim();
  }

  private extractUserPrompt(item: ResponseInputItem): string | null {
    const message = item as unknown as { role?: unknown; content?: unknown };
    if (message.role !== 'user') return null;
    if (typeof message.content === 'string') return message.content;
    if (!Array.isArray(message.content)) return null;
    const pieces = message.content
      .map((entry) => {
        if (!entry || typeof entry !== 'object') return '';
        const text = (entry as { text?: unknown }).text;
        return typeof text === 'string' ? text : '';
      })
      .filter(Boolean);
    return pieces.length > 0 ? pieces.join(' ') : null;
  }

  private async handleItem(item: ResponseItem): Promise<ResponseItem[]> {
    const itemType = (item as { type?: string }).type;
    if (itemType === 'reasoning') {
      const text = this.extractReasoningText(item as unknown as Record<string, unknown>);
      if (text) this.emit('reasoning_delta', { text });
    }

    if (item.type === 'message') {
      const msg = item as ResponseOutputMessage;
      const c = msg.content;
      if (msg.role === 'assistant' && Array.isArray(c)) {
        for (const part of c) {
          if (part && typeof part === 'object' && 'text' in part && typeof part.text === 'string') {
            this.emit('text_delta', { text: part.text });
          }
        }
        this.emit('text_done', {});
      }
    }

    if (item.type === 'function_call') {
      const fc = item as ResponseFunctionToolCallItem;
      const argsObj = JSON.parse(fc.arguments) as Record<string, unknown>;
      if (fc.name === BATCH_FUNC_NAME && Array.isArray(argsObj.actions)) {
        const actions = argsObj.actions.filter(
          (action): action is Record<string, unknown> =>
            typeof action === 'object' && action !== null,
        );
        const elapsedMs = this.currentModelElapsedMs();
        this.emit('action', {
          action_type: 'batch',
          description: describeBatchActions(actions),
          action: { type: 'batch', actions },
          ...(elapsedMs === null ? {} : { elapsed_ms: elapsedMs }),
        });
      } else {
        const elapsedMs = this.currentModelElapsedMs();
        this.emit('action', {
          action_type: fc.name,
          description: `${fc.name}(${JSON.stringify(argsObj)})`,
          action: argsObj,
          ...(elapsedMs === null ? {} : { elapsed_ms: elapsedMs }),
        });
      }

      if (fc.name === BATCH_FUNC_NAME) {
        return this.handleBatchCall(fc.call_id, argsObj);
      }
      if (fc.name === EXTRA_FUNC_NAME) {
        return this.handleExtraCall(fc.call_id, argsObj);
      }

      return [
        {
          type: 'function_call_output',
          call_id: fc.call_id,
          output: `Unsupported function call: ${fc.name}`,
        } as unknown as ResponseFunctionToolCallOutputItem,
      ];
    }

    if (item.type === 'computer_call') {
      const cc = item as ResponseComputerToolCall & {
        action?: Record<string, unknown>;
        actions?: Array<Record<string, unknown>>;
      };
      const actionList = Array.isArray(cc.actions)
        ? cc.actions
        : cc.action
          ? [cc.action]
          : [];

      const elapsedMs = this.currentModelElapsedMs();
      const actionType =
        actionList.length === 1 ? String(actionList[0]?.type ?? 'unknown') : 'batch';
      const description =
        actionList.length === 1
          ? describeAction(actionType, actionList[0] ?? {})
          : describeBatchActions(actionList);
      const actionPayload =
        actionList.length === 1 ? (actionList[0] ?? {}) : { type: 'batch', actions: actionList };
      this.emit('action', {
        action_type: actionType,
        description,
        action: actionPayload,
        ...(elapsedMs === null ? {} : { elapsed_ms: elapsedMs }),
      });
      await this.computer.batchActions(actionList as CuaAction[]);

      const screenshot = await this.computer.screenshot();
      this.emit('screenshot', { captured: true, bytes_base64: screenshot.length });

      const pending = cc.pending_safety_checks ?? [];
      for (const check of pending) {
        const msg = check.message ?? '';
        if (!this.ackCb(msg)) throw new Error(`Safety check failed: ${msg}`);
      }

      const currentUrl = await this.computer.getCurrentUrl();
      utils.checkBlocklistedUrl(currentUrl);

      const screenshotOutput = {
        type: 'computer_screenshot',
        image_url: `data:image/png;base64,${screenshot}`,
      } as unknown as ResponseComputerToolCallOutputItem['output'];
      (screenshotOutput as { current_url?: string }).current_url = currentUrl;

      const out: Omit<ResponseComputerToolCallOutputItem, 'id'> = {
        type: 'computer_call_output',
        call_id: cc.call_id,
        acknowledged_safety_checks: pending,
        output: screenshotOutput,
      };
      return [out as ResponseItem];
    }

    return [];
  }

  private async handleBatchCall(
    callId: string,
    argsObj: Record<string, unknown>,
  ): Promise<ResponseItem[]> {
    const actions = Array.isArray(argsObj.actions) ? (argsObj.actions as CuaAction[]) : [];
    await this.computer.batchActions(actions);

    let statusText = 'Actions executed successfully.';
    const terminalReadAction = this.batchTerminalReadAction(actions);
    if (terminalReadAction === 'url') {
      try {
        const currentUrl = await this.computer.getCurrentUrl();
        statusText = `Actions executed successfully. Current URL: ${currentUrl}`;
      } catch (error) {
        statusText = `Actions executed, but url() failed: ${error instanceof Error ? error.message : String(error)}`;
      }
    }

    const outputItems: Array<Record<string, unknown>> = [{ type: 'text', text: statusText }];
    if (terminalReadAction !== 'url') {
      const screenshot = await this.computer.screenshot();
      outputItems.push({
        type: 'image_url',
        image_url: `data:image/png;base64,${screenshot}`,
        detail: 'original',
      });
    }
    return [
      {
        type: 'function_call_output',
        call_id: callId,
        output: JSON.stringify(outputItems),
      } as unknown as ResponseFunctionToolCallOutputItem,
    ];
  }

  private async handleExtraCall(
    callId: string,
    argsObj: Record<string, unknown>,
  ): Promise<ResponseItem[]> {
    const action = typeof argsObj.action === 'string' ? argsObj.action : '';
    const url = typeof argsObj.url === 'string' ? argsObj.url : '';
    let statusText = '';
    if (action === 'goto') {
      await this.computer.batchActions([{ type: 'goto', url }]);
      statusText = 'goto executed successfully.';
    } else if (action === 'back') {
      await this.computer.batchActions([{ type: 'back' }]);
      statusText = 'back executed successfully.';
    } else if (action === 'url') {
      const currentUrl = await this.computer.getCurrentUrl();
      statusText = `Current URL: ${currentUrl}`;
    } else {
      statusText = `unknown ${EXTRA_FUNC_NAME} action: ${action}`;
    }

    const outputItems: Array<Record<string, unknown>> = [{ type: 'text', text: statusText }];
    if (action !== 'url') {
      const screenshot = await this.computer.screenshot();
      outputItems.push({
        type: 'image_url',
        image_url: `data:image/png;base64,${screenshot}`,
        detail: 'original',
      });
    }
    return [
      {
        type: 'function_call_output',
        call_id: callId,
        output: JSON.stringify(outputItems),
      } as unknown as ResponseFunctionToolCallOutputItem,
    ];
  }

  private batchTerminalReadAction(actions: CuaAction[]): '' | 'url' | 'screenshot' {
    if (actions.length === 0) return '';
    const lastType = actions[actions.length - 1]?.type;
    if (lastType === 'url' || lastType === 'screenshot') return lastType;
    return '';
  }

  async runFullTurn(opts: {
    messages: ResponseInputItem[];
    print_steps?: boolean;
    debug?: boolean;
    show_images?: boolean;
    onEvent?: (event: AgentEvent) => void;
  }): Promise<ResponseItem[]> {
    this.print_steps = opts.print_steps ?? true;
    this.debug = opts.debug ?? false;
    this.show_images = opts.show_images ?? false;
    this.onEvent = opts.onEvent ?? null;
    const newItems: ResponseItem[] = [];
    let turns = 0;

    for (const message of opts.messages) {
      const prompt = this.extractUserPrompt(message);
      if (prompt) this.emit('prompt', { text: prompt });
    }

    try {
      while (
        newItems.length === 0 ||
        (newItems[newItems.length - 1] as ResponseItem & { role?: string }).role !== 'assistant'
      ) {
        turns += 1;
        const inputMessages = [...opts.messages];

        this.debugPrint(...inputMessages, ...newItems);
        this.modelRequestStartedAt = Date.now();
        const response = await utils.createResponse({
          model: this.model,
          input: [...inputMessages, ...newItems],
          tools: this.tools,
          truncation: 'auto',
          reasoning: {
            effort: 'low',
            summary: 'concise',
          },
          instructions: batchInstructions,
        });
        if (!response.output) throw new Error('No output from model');
        for (const msg of response.output as ResponseItem[]) {
          newItems.push(msg, ...(await this.handleItem(msg)));
        }
        this.modelRequestStartedAt = null;
        this.emit('turn_done', { turn: turns });
      }
    } catch (error) {
      this.modelRequestStartedAt = null;
      this.emit('error', { message: error instanceof Error ? error.message : String(error) });
      throw error;
    }
    this.emit('run_complete', { turns });

    return !this.show_images
      ? newItems.map((msg) => utils.sanitizeMessage(msg) as ResponseItem)
      : newItems;
  }
}
