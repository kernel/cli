/**
 * OpenAI CUA provider.
 *
 * Uses OpenAI's Responses API with the computer tool. Handles both
 * computer_call and function_call response types.
 */

import OpenAI from 'openai';
import type { KernelExecutor, CommonAction } from '../tools';
import type { CUAProvider, ProviderConfig, ProviderResult } from './index';

type ResponseItem = OpenAI.Responses.ResponseItem;
type ResponseInputItem = OpenAI.Responses.ResponseInputItem;

const SYSTEM_INSTRUCTIONS = `You have access to a computer tool for browser automation.
- Current date and time: ${new Date().toISOString()} (${new Date().toLocaleDateString('en-US', { weekday: 'long' })})
- CHROMIUM IS ALREADY OPEN. Use it directly.
- To navigate, use the browser URL bar (Ctrl+L).
- Prefer batching predictable action sequences when possible.`;

// Map OpenAI computer_call actions to CommonAction
function toCommonAction(action: Record<string, unknown>): CommonAction {
  const type = action.type as string;
  switch (type) {
    case 'click': {
      if (action.button === 'back') return { type: 'back' };
      if (action.button === 'wheel') {
        return {
          type: 'scroll',
          x: action.x as number ?? 0, y: action.y as number ?? 0,
          scrollX: Math.trunc(action.scroll_x as number ?? 0),
          scrollY: Math.trunc(action.scroll_y as number ?? 0),
        };
      }
      return { type: 'click', x: action.x as number, y: action.y as number };
    }
    case 'double_click': return { type: 'double_click', x: action.x as number, y: action.y as number };
    case 'type': return { type: 'type', text: action.text as string };
    case 'keypress': {
      const keys = (action.keys as string[] ?? []).join('+');
      const holdKeys = (action.hold_keys as string[] ?? []);
      const combo = holdKeys.length > 0 ? [...holdKeys, keys].join('+') : keys;
      return { type: 'key', keys: combo };
    }
    case 'scroll': return {
      type: 'scroll',
      x: action.x as number ?? 0, y: action.y as number ?? 0,
      scrollX: Math.trunc(action.scroll_x as number ?? 0),
      scrollY: Math.trunc(action.scroll_y as number ?? 0),
    };
    case 'move': return { type: 'mouse_move', x: action.x as number, y: action.y as number };
    case 'drag': {
      const path = action.path as Array<{ x: number; y: number }> ?? [];
      return { type: 'drag', path: path.map(p => [p.x, p.y]) };
    }
    case 'wait': return { type: 'wait', duration: action.ms as number ?? 1000 };
    case 'screenshot': return { type: 'screenshot' };
    case 'goto': return { type: 'goto', url: action.url as string };
    case 'back': return { type: 'back' };
    default: return { type: 'screenshot' };
  }
}

export class OpenAIProvider implements CUAProvider {
  name = 'openai' as const;

  async run(config: ProviderConfig, executor: KernelExecutor): Promise<ProviderResult> {
    const apiKey = config.apiKey || process.env.OPENAI_API_KEY;
    if (!apiKey) throw new Error('OPENAI_API_KEY is required for OpenAI provider');

    const model = config.model || 'gpt-5.4';
    const client = new OpenAI({ apiKey });

    // Navigate to a starting page
    await executor.execute({ type: 'goto', url: 'https://duckduckgo.com' });

    const inputMessages: ResponseInputItem[] = [
      { role: 'system', content: SYSTEM_INSTRUCTIONS } as ResponseInputItem,
      { type: 'message', role: 'user', content: [{ type: 'input_text', text: config.query }] } as ResponseInputItem,
    ];

    const newItems: ResponseItem[] = [];

    while (true) {
      const lastItem = newItems[newItems.length - 1] as ResponseItem & { role?: string } | undefined;
      if (lastItem?.role === 'assistant') break;

      const response = await client.responses.create({
        model,
        input: [...inputMessages, ...newItems] as ResponseInputItem[],
        tools: [{ type: 'computer' } as OpenAI.Responses.Tool],
        truncation: 'auto',
        reasoning: { effort: 'low', summary: 'concise' },
        instructions: SYSTEM_INSTRUCTIONS,
      });

      if (!response.output) throw new Error('No output from OpenAI model');

      for (const item of response.output) {
        newItems.push(item);

        if (item.type === 'computer_call') {
          const cc = item as OpenAI.Responses.ResponseComputerToolCall & {
            action?: Record<string, unknown>;
            actions?: Array<Record<string, unknown>>;
          };
          const actionList = Array.isArray(cc.actions) ? cc.actions : cc.action ? [cc.action] : [];

          for (const action of actionList) {
            const common = toCommonAction(action);
            console.log(`[openai] action: ${common.type}`);
            await executor.execute(common);
          }

          // Take post-action screenshot
          const screenshot = await executor.screenshot();
          const pending = cc.pending_safety_checks ?? [];
          for (const check of pending) {
            console.log(`[openai] safety check: ${check.message}`);
          }

          newItems.push({
            type: 'computer_call_output',
            call_id: cc.call_id,
            acknowledged_safety_checks: pending,
            output: {
              type: 'computer_screenshot',
              image_url: `data:image/png;base64,${screenshot.base64Image}`,
            },
          } as unknown as ResponseItem);
        }

        if (item.type === 'message') {
          const msg = item as OpenAI.Responses.ResponseOutputMessage;
          if (msg.role === 'assistant') {
            const textParts = msg.content?.filter(c => 'text' in c) ?? [];
            const text = textParts.map(c => 'text' in c ? c.text : '').join('');
            if (text) {
              return { result: text, provider: 'openai' };
            }
          }
        }
      }
    }

    // Extract final answer from items
    const messages = newItems.filter(i => i.type === 'message') as OpenAI.Responses.ResponseOutputMessage[];
    const lastAssistant = messages.find(m => m.role === 'assistant');
    const lastContent = lastAssistant?.content?.slice(-1)[0];
    const answer = lastContent && 'text' in lastContent ? lastContent.text : '';
    return { result: answer || 'Task completed.', provider: 'openai' };
  }
}
