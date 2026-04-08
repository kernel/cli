/**
 * Anthropic CUA provider.
 *
 * Uses Claude's computer use beta API. Maintains the sampling loop pattern
 * from the Anthropic reference implementation.
 */

import { Anthropic } from '@anthropic-ai/sdk';
import type { KernelExecutor, ToolResult, CommonAction } from '../tools';
import type { CUAProvider, ProviderConfig, ProviderResult } from './index';

type BetaMessageParam = Anthropic.Beta.Messages.BetaMessageParam;
type BetaContentBlock = Anthropic.Beta.Messages.BetaContentBlockParam;
type BetaTextBlock = Anthropic.Beta.Messages.BetaTextBlockParam;

const CURRENT_DATE = new Intl.DateTimeFormat('en-US', {
  weekday: 'long', month: 'long', day: 'numeric', year: 'numeric',
}).format(new Date());

const SYSTEM_PROMPT = `<SYSTEM_CAPABILITY>
* You are utilising an Ubuntu virtual machine using ${process.arch} architecture with internet access.
* When you connect to the display, CHROMIUM IS ALREADY OPEN. The url bar is not visible but it is there.
* If you need to navigate to a new page, use ctrl+l to focus the url bar and then enter the url.
* You won't be able to see the url bar from the screenshot but ctrl-l still works.
* As the initial step click on the search bar.
* When viewing a page it can be helpful to zoom out so that you can see everything on the page.
* Either that, or make sure you scroll down to see everything before deciding something isn't available.
* Scroll action: scroll_amount and the tool result are in wheel units (not pixels).
* When using your computer function calls, they take a while to run and send back to you.
* Where possible/feasible, try to chain multiple of these calls all into one function calls request.
* The current date is ${CURRENT_DATE}.
* After each step, take a screenshot and carefully evaluate if you have achieved the right outcome.
* Explicitly show your thinking: "I have evaluated step X..." If not correct, try again.
* Only when you confirm a step was executed correctly should you move on to the next one.
</SYSTEM_CAPABILITY>

<IMPORTANT>
* When using Chromium, if a startup wizard appears, IGNORE IT. Do not even click "skip this step".
* Instead, click on the search bar on the center of the screen where it says "Search or enter address", and enter the appropriate search term or URL there.
</IMPORTANT>`;

function getToolVersion(model: string): 'computer_use_20251124' | 'computer_use_20250124' {
  if (model.includes('claude-sonnet-4-6') || model.includes('claude-opus-4-6') || model.includes('claude-opus-4-5')) {
    return 'computer_use_20251124';
  }
  return 'computer_use_20250124';
}

function getBetaFlags(toolVersion: string): string[] {
  const betas: string[] = ['prompt-caching-2024-07-31'];
  if (toolVersion === 'computer_use_20251124') {
    betas.push('computer-use-2025-11-24');
  } else {
    betas.push('computer-use-2025-01-24');
  }
  return betas;
}

// Map Anthropic tool_use actions to CommonAction
function toCommonAction(input: Record<string, unknown>): CommonAction {
  const action = input.action as string;
  switch (action) {
    case 'screenshot': return { type: 'screenshot' };
    case 'left_click': return { type: 'click', x: (input.coordinate as number[])?.[0], y: (input.coordinate as number[])?.[1] };
    case 'right_click': return { type: 'right_click', x: (input.coordinate as number[])?.[0], y: (input.coordinate as number[])?.[1] };
    case 'middle_click': return { type: 'middle_click', x: (input.coordinate as number[])?.[0], y: (input.coordinate as number[])?.[1] };
    case 'double_click': return { type: 'double_click', x: (input.coordinate as number[])?.[0], y: (input.coordinate as number[])?.[1] };
    case 'triple_click': return { type: 'triple_click', x: (input.coordinate as number[])?.[0], y: (input.coordinate as number[])?.[1] };
    case 'mouse_move': return { type: 'mouse_move', x: (input.coordinate as number[])?.[0], y: (input.coordinate as number[])?.[1] };
    case 'left_mouse_down': return { type: 'mouse_down', x: (input.coordinate as number[])?.[0], y: (input.coordinate as number[])?.[1] };
    case 'left_mouse_up': return { type: 'mouse_up', x: (input.coordinate as number[])?.[0], y: (input.coordinate as number[])?.[1] };
    case 'type': return { type: 'type', text: input.text as string };
    case 'key': return { type: 'key', keys: input.text as string };
    case 'hold_key': return { type: 'key', keys: input.text as string, duration: input.duration as number };
    case 'scroll': {
      const dir = (input.scrollDirection || input.scroll_direction) as string;
      const amount = (input.scrollAmount || input.scroll_amount || 3) as number;
      let scrollX = 0, scrollY = 0;
      if (dir === 'down') scrollY = amount;
      else if (dir === 'up') scrollY = -amount;
      else if (dir === 'right') scrollX = amount;
      else if (dir === 'left') scrollX = -amount;
      return {
        type: 'scroll',
        x: (input.coordinate as number[])?.[0] ?? 640,
        y: (input.coordinate as number[])?.[1] ?? 400,
        scrollX, scrollY,
      };
    }
    case 'left_click_drag': {
      const start = input.start_coordinate as number[] | undefined;
      const coord = input.coordinate as number[];
      return {
        type: 'drag',
        startX: start?.[0], startY: start?.[1],
        endX: coord?.[0], endY: coord?.[1],
      };
    }
    case 'wait': return { type: 'wait', duration: (input.duration as number ?? 1) * 1000 };
    default: return { type: 'screenshot' };
  }
}

function makeToolResult(result: ToolResult, toolUseId: string): BetaContentBlock {
  const content: unknown[] = [];
  if (result.error) {
    return { type: 'tool_result', tool_use_id: toolUseId, content: result.error, is_error: true } as unknown as BetaContentBlock;
  }
  if (result.output) {
    content.push({ type: 'text', text: result.output });
  }
  if (result.base64Image) {
    content.push({ type: 'image', source: { type: 'base64', media_type: 'image/png', data: result.base64Image } });
  }
  return { type: 'tool_result', tool_use_id: toolUseId, content } as unknown as BetaContentBlock;
}

export class AnthropicProvider implements CUAProvider {
  name = 'anthropic' as const;

  async run(config: ProviderConfig, executor: KernelExecutor): Promise<ProviderResult> {
    const apiKey = config.apiKey || process.env.ANTHROPIC_API_KEY;
    if (!apiKey) throw new Error('ANTHROPIC_API_KEY is required for Anthropic provider');

    const model = config.model || 'claude-sonnet-4-6';
    const toolVersion = getToolVersion(model);
    const betas = getBetaFlags(toolVersion);
    const client = new Anthropic({ apiKey, maxRetries: 4 });

    const system: BetaTextBlock & { cache_control?: { type: 'ephemeral' } } = {
      type: 'text',
      text: SYSTEM_PROMPT,
      cache_control: { type: 'ephemeral' },
    };

    const toolParams = [{
      name: 'computer',
      type: toolVersion === 'computer_use_20251124' ? 'computer_20251124' : 'computer_20250124',
      display_width_px: config.viewportWidth ?? 1280,
      display_height_px: config.viewportHeight ?? 800,
      display_number: null,
    }];

    const messages: BetaMessageParam[] = [{ role: 'user', content: config.query }];

    while (true) {
      const response = await client.beta.messages.create({
        max_tokens: 4096,
        messages,
        model,
        system: [system],
        tools: toolParams as Anthropic.Beta.Messages.BetaToolUnionParam[],
        betas,
        thinking: { type: 'enabled', budget_tokens: 1024 },
      });

      const responseParams = response.content.map(block => {
        if (block.type === 'text') return { type: 'text' as const, text: block.text };
        if (block.type === 'thinking') return { type: 'thinking' as const, thinking: block.thinking, signature: (block as Record<string, unknown>).signature };
        return block;
      });

      console.log(`[anthropic] stop_reason=${response.stop_reason}`);
      messages.push({ role: 'assistant', content: responseParams as BetaContentBlock[] });

      if (response.stop_reason === 'end_turn') {
        const textBlocks = response.content.filter(b => b.type === 'text');
        const resultText = textBlocks.map(b => 'text' in b ? b.text : '').join('');
        return { result: resultText, provider: 'anthropic' };
      }

      const toolResults: BetaContentBlock[] = [];
      for (const block of response.content) {
        if (block.type === 'tool_use') {
          const input = block.input as Record<string, unknown>;
          const commonAction = toCommonAction(input);
          console.log(`[anthropic] action: ${commonAction.type}`);
          const result = await executor.execute(commonAction);
          toolResults.push(makeToolResult(result, block.id));
        }
      }

      if (toolResults.length === 0) {
        const textBlocks = response.content.filter(b => b.type === 'text');
        return { result: textBlocks.map(b => 'text' in b ? b.text : '').join(''), provider: 'anthropic' };
      }

      messages.push({ role: 'user', content: toolResults });
    }
  }
}
