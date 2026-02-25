import {
  type ResponseItem,
  type ResponseInputItem,
  type ResponseOutputMessage,
  type ResponseFunctionToolCallItem,
  type ResponseFunctionToolCallOutputItem,
  type ResponseComputerToolCall,
  type ResponseComputerToolCallOutputItem,
  type ComputerTool,
  type Tool,
} from 'openai/resources/responses/responses';

import * as utils from './utils';
import { batchInstructions, batchComputerTool, navigationTools } from './toolset';
import type { KernelComputer } from './kernel-computer';

const BATCH_FUNC_NAME = 'batch_computer_actions';

export class Agent {
  private model: string;
  private computer: KernelComputer;
  private tools: Tool[];
  private print_steps = true;
  private debug = false;
  private show_images = false;
  private ackCb: (msg: string) => boolean;

  constructor(opts: {
    model?: string;
    computer: KernelComputer;
    tools?: Tool[];
    acknowledge_safety_check_callback?: (msg: string) => boolean;
  }) {
    this.model = opts.model ?? 'computer-use-preview';
    this.computer = opts.computer;
    this.ackCb = opts.acknowledge_safety_check_callback ?? ((): boolean => true);

    const [w, h] = this.computer.getDimensions();
    this.tools = [
      ...navigationTools,
      batchComputerTool,
      ...(opts.tools ?? []),
      {
        type: 'computer_use_preview',
        display_width: w,
        display_height: h,
        environment: this.computer.getEnvironment(),
      } as ComputerTool,
    ];
  }

  private debugPrint(...args: unknown[]): void {
    if (this.debug) {
      console.warn('--- debug:agent:debugPrint');
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

  private async handleItem(item: ResponseItem): Promise<ResponseItem[]> {
    if (item.type === 'message' && this.print_steps) {
      const msg = item as ResponseOutputMessage;
      const c = msg.content;
      if (Array.isArray(c) && c[0] && 'text' in c[0] && typeof c[0].text === 'string')
        console.log(c[0].text);
    }

    if (item.type === 'function_call') {
      const fc = item as ResponseFunctionToolCallItem;
      const argsObj = JSON.parse(fc.arguments) as Record<string, unknown>;
      if (this.print_steps) console.log(`${fc.name}(${JSON.stringify(argsObj)})`);

      if (fc.name === BATCH_FUNC_NAME) {
        return this.handleBatchCall(fc.call_id, argsObj);
      }

      // Navigation tools (goto, back, forward)
      const navFn = (this.computer as unknown as Record<string, unknown>)[fc.name];
      if (typeof navFn === 'function') {
        await (navFn as (...a: unknown[]) => unknown).call(
          this.computer,
          ...Object.values(argsObj),
        );
      }
      return [
        {
          type: 'function_call_output',
          call_id: fc.call_id,
          output: 'success',
        } as unknown as ResponseFunctionToolCallOutputItem,
      ];
    }

    if (item.type === 'computer_call') {
      const cc = item as ResponseComputerToolCall;
      const { type: actionType, ...actionArgs } = cc.action;
      if (this.print_steps) console.log(`${actionType}(${JSON.stringify(actionArgs)})`);

      await this.executeComputerAction(actionType as string, cc.action as unknown as Record<string, unknown>);
      const screenshot = await this.computer.screenshot();

      const pending = cc.pending_safety_checks ?? [];
      for (const check of pending) {
        const msg = check.message ?? '';
        if (!this.ackCb(msg)) throw new Error(`Safety check failed: ${msg}`);
      }

      const currentUrl = await this.computer.getCurrentUrl();
      utils.checkBlocklistedUrl(currentUrl);

      const out: Omit<ResponseComputerToolCallOutputItem, 'id'> = {
        type: 'computer_call_output',
        call_id: cc.call_id,
        acknowledged_safety_checks: pending,
        output: {
          type: 'computer_screenshot',
          image_url: `data:image/png;base64,${screenshot}`,
        },
      };
      return [out as ResponseItem];
    }

    return [];
  }

  private async executeComputerAction(
    actionType: string,
    action: Record<string, unknown>,
  ): Promise<void> {
    switch (actionType) {
      case 'click':
        await this.computer.click(
          action.x as number,
          action.y as number,
          (action.button as string) ?? 'left',
        );
        break;
      case 'double_click':
        await this.computer.doubleClick(action.x as number, action.y as number);
        break;
      case 'type':
        await this.computer.type(action.text as string);
        break;
      case 'keypress':
        await this.computer.keypress(action.keys as string[]);
        break;
      case 'scroll':
        await this.computer.scroll(
          action.x as number,
          action.y as number,
          (action.scroll_x as number) ?? 0,
          (action.scroll_y as number) ?? 0,
        );
        break;
      case 'move':
        await this.computer.move(action.x as number, action.y as number);
        break;
      case 'drag':
        await this.computer.drag(action.path as Array<{ x: number; y: number }>);
        break;
      case 'wait':
        await this.computer.wait((action.ms as number) ?? 1000);
        break;
      case 'screenshot':
        break;
      default:
        console.warn(`Unknown computer action: ${actionType}`);
    }
  }

  private async handleBatchCall(
    callId: string,
    argsObj: Record<string, unknown>,
  ): Promise<ResponseItem[]> {
    const actions = argsObj.actions as unknown as Parameters<typeof this.computer.batchActions>[0];
    await this.computer.batchActions(actions);

    const screenshot = await this.computer.screenshot();
    return [
      {
        type: 'function_call_output',
        call_id: callId,
        output: JSON.stringify([
          { type: 'text', text: 'Actions executed successfully.' },
          { type: 'image_url', image_url: `data:image/png;base64,${screenshot}` },
        ]),
      } as unknown as ResponseFunctionToolCallOutputItem,
    ];
  }

  async runFullTurn(opts: {
    messages: ResponseInputItem[];
    print_steps?: boolean;
    debug?: boolean;
    show_images?: boolean;
  }): Promise<ResponseItem[]> {
    this.print_steps = opts.print_steps ?? true;
    this.debug = opts.debug ?? false;
    this.show_images = opts.show_images ?? false;
    const newItems: ResponseItem[] = [];

    while (
      newItems.length === 0 ||
      (newItems[newItems.length - 1] as ResponseItem & { role?: string }).role !== 'assistant'
    ) {
      const inputMessages = [...opts.messages];

      // Append current URL context to system message
      const currentUrl = await this.computer.getCurrentUrl();
      const sysIndex = inputMessages.findIndex((msg) => 'role' in msg && msg.role === 'system');
      if (sysIndex >= 0) {
        const msg = inputMessages[sysIndex];
        const urlInfo = `\n- Current URL: ${currentUrl}`;
        if (msg && 'content' in msg && typeof msg.content === 'string') {
          inputMessages[sysIndex] = { ...msg, content: msg.content + urlInfo } as typeof msg;
        }
      }

      this.debugPrint(...inputMessages, ...newItems);
      const response = await utils.createResponse({
        model: this.model,
        input: [...inputMessages, ...newItems],
        tools: this.tools,
        truncation: 'auto',
        instructions: batchInstructions,
      });
      if (!response.output) throw new Error('No output from model');
      for (const msg of response.output as ResponseItem[]) {
        newItems.push(msg, ...(await this.handleItem(msg)));
      }
    }

    return !this.show_images
      ? newItems.map((msg) => utils.sanitizeMessage(msg) as ResponseItem)
      : newItems;
  }
}
