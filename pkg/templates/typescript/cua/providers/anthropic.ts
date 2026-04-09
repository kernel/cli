/**
 * Anthropic CUA provider adapter.
 *
 * Uses the Anthropic SDK's beta computer-use API with Claude models.
 */

import { Anthropic } from '@anthropic-ai/sdk';
import type { CuaProvider, TaskOptions, TaskResult } from './index';

function getSystemPrompt(): string {
  const date = new Date().toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' });
  return `<SYSTEM_CAPABILITY>
* You are utilising an Ubuntu virtual machine with internet access.
* When you connect to the display, CHROMIUM IS ALREADY OPEN.
* If you need to navigate to a new page, use ctrl+l to focus the url bar and then enter the url.
* After each step, take a screenshot and carefully evaluate if you have achieved the right outcome.
* Explicitly show your thinking: "I have evaluated step X..." If not correct, try again.
* Only when you confirm a step was executed correctly should you move on to the next one.
* The current date is ${date}.
</SYSTEM_CAPABILITY>

<IMPORTANT>
* When using Chromium, if a startup wizard appears, IGNORE IT.
* Click on the search bar and enter the appropriate URL there.
</IMPORTANT>`;
}

type BetaMessageParam = Anthropic.Beta.Messages.BetaMessageParam;
type BetaContentBlockParam = Anthropic.Beta.Messages.BetaContentBlockParam;

export class AnthropicProvider implements CuaProvider {
  readonly name = 'anthropic';
  private apiKey: string;

  constructor() {
    this.apiKey = process.env.ANTHROPIC_API_KEY ?? '';
  }

  isConfigured(): boolean {
    return this.apiKey.length > 0;
  }

  async runTask(options: TaskOptions): Promise<TaskResult> {
    const { query, kernel, sessionId, viewportWidth = 1280, viewportHeight = 800 } = options;
    const client = new Anthropic({ apiKey: this.apiKey, maxRetries: 4 });
    const model = options.model || 'claude-sonnet-4-6';

    const messages: BetaMessageParam[] = [{ role: 'user', content: query }];

    while (true) {
      const response = await client.beta.messages.create({
        max_tokens: 4096,
        messages,
        model,
        system: [{ type: 'text', text: getSystemPrompt(), cache_control: { type: 'ephemeral' } }],
        tools: [{
          type: 'computer_20251124',
          name: 'computer',
          display_width_px: viewportWidth,
          display_height_px: viewportHeight,
          display_number: 1,
        }],
        betas: ['computer-use-2025-11-24', 'prompt-caching-2024-07-31'],
        thinking: { type: 'enabled', budget_tokens: 1024 },
      });

      // Build assistant content for the messages array
      const assistantContent: BetaContentBlockParam[] = response.content.map(block => {
        if (block.type === 'thinking') {
          return { type: 'thinking' as const, thinking: block.thinking, signature: block.signature };
        }
        if (block.type === 'text') {
          return { type: 'text' as const, text: block.text };
        }
        if (block.type === 'tool_use') {
          return { type: 'tool_use' as const, id: block.id, name: block.name, input: block.input };
        }
        return block as unknown as BetaContentBlockParam;
      });
      messages.push({ role: 'assistant', content: assistantContent });

      if (response.stop_reason === 'end_turn') {
        const text = response.content
          .filter((b): b is Anthropic.Beta.Messages.BetaTextBlock => b.type === 'text')
          .map(b => b.text)
          .join('');
        return { result: text, provider: this.name };
      }

      // Process tool calls
      const toolResults: BetaContentBlockParam[] = [];
      for (const block of response.content) {
        if (block.type !== 'tool_use') continue;

        const input = block.input as Record<string, unknown>;
        const action = input.action as string;

        try {
          const screenshot = await this.executeAction(kernel, sessionId, action, input);
          toolResults.push({
            type: 'tool_result' as unknown as 'text',
            tool_use_id: block.id,
            content: [{ type: 'image', source: { type: 'base64', media_type: 'image/png', data: screenshot } }],
          } as unknown as BetaContentBlockParam);
        } catch (error) {
          toolResults.push({
            type: 'tool_result' as unknown as 'text',
            tool_use_id: block.id,
            content: [{ type: 'text', text: `Error: ${error instanceof Error ? error.message : String(error)}` }],
            is_error: true,
          } as unknown as BetaContentBlockParam);
        }
      }

      if (toolResults.length > 0) {
        messages.push({ role: 'user', content: toolResults });
      } else {
        // No tool use and not end_turn — model is done
        const text = response.content
          .filter((b): b is Anthropic.Beta.Messages.BetaTextBlock => b.type === 'text')
          .map(b => b.text)
          .join('');
        return { result: text || '(no response)', provider: this.name };
      }
    }
  }

  private async executeAction(
    kernel: TaskOptions['kernel'],
    sessionId: string,
    action: string,
    input: Record<string, unknown>,
  ): Promise<string> {
    const computer = kernel.browsers.computer;

    switch (action) {
      case 'screenshot': break;
      case 'key': {
        const key = input.key as string;
        await computer.pressKey(sessionId, { keys: [this.mapKey(key)] });
        break;
      }
      case 'hold_key': {
        const key = input.key as string;
        const duration = (input.duration as number) ?? 500;
        await computer.pressKey(sessionId, { keys: [this.mapKey(key)], duration });
        break;
      }
      case 'type': {
        const text = input.text as string;
        await computer.typeText(sessionId, { text });
        break;
      }
      case 'cursor_position': break;
      case 'mouse_move': {
        const [x, y] = input.coordinate as [number, number];
        await computer.moveMouse(sessionId, { x, y });
        break;
      }
      case 'left_click':
      case 'right_click':
      case 'middle_click': {
        const [x, y] = input.coordinate as [number, number];
        const button = action === 'right_click' ? 'right' : action === 'middle_click' ? 'middle' : 'left';
        await computer.clickMouse(sessionId, { x, y, button });
        break;
      }
      case 'double_click': {
        const [x, y] = input.coordinate as [number, number];
        await computer.clickMouse(sessionId, { x, y, num_clicks: 2 });
        break;
      }
      case 'triple_click': {
        const [x, y] = input.coordinate as [number, number];
        await computer.clickMouse(sessionId, { x, y, num_clicks: 3 });
        break;
      }
      case 'left_click_drag': {
        const startCoordinate = input.start_coordinate as [number, number];
        const [ex, ey] = input.coordinate as [number, number];
        await computer.dragMouse(sessionId, {
          path: [
            [startCoordinate[0], startCoordinate[1]],
            [ex, ey],
          ],
        });
        break;
      }
      case 'scroll': {
        const [x, y] = input.coordinate as [number, number];
        const direction = input.direction as string;
        const amount = (input.amount as number) ?? 3;
        const deltaX = direction === 'left' ? -amount : direction === 'right' ? amount : 0;
        const deltaY = direction === 'up' ? -amount : direction === 'down' ? amount : 0;
        await computer.scroll(sessionId, { x, y, delta_x: deltaX, delta_y: deltaY });
        break;
      }
      case 'wait': {
        const duration = (input.duration as number) ?? 1000;
        await new Promise(r => setTimeout(r, duration));
        break;
      }
      default:
        throw new Error(`Unknown action: ${action}`);
    }

    // Take screenshot after every action
    await new Promise(r => setTimeout(r, 500));
    const resp = await computer.captureScreenshot(sessionId);
    const buf = Buffer.from(await resp.arrayBuffer());
    return buf.toString('base64');
  }

  private mapKey(key: string): string {
    const map: Record<string, string> = {
      Return: 'Return', Enter: 'Return', Backspace: 'BackSpace',
      Tab: 'Tab', Escape: 'Escape', space: 'space', Space: 'space',
      Up: 'Up', Down: 'Down', Left: 'Left', Right: 'Right',
      Home: 'Home', End: 'End', Page_Up: 'Prior', Page_Down: 'Next',
      ctrl: 'Control_L', Control_L: 'Control_L', alt: 'Alt_L', Alt_L: 'Alt_L',
      shift: 'Shift_L', Shift_L: 'Shift_L', super: 'Super_L', Super_L: 'Super_L',
    };
    // Handle combos like "ctrl+l"
    if (key.includes('+')) {
      return key.split('+').map(k => map[k.trim()] ?? k.trim()).join('+');
    }
    return map[key] ?? key;
  }
}
