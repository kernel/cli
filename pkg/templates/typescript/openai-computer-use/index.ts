import { Kernel, type KernelContext } from '@onkernel/sdk';
import 'dotenv/config';
import type { ResponseItem, ResponseOutputMessage } from 'openai/resources/responses/responses';
import { Agent } from './lib/agent';
import computers from './lib/computers';

interface CuaInput {
  task: string;
}
interface CuaOutput {
  elapsed: number;
  answer: string | null;
  logs?: ResponseItem[];
}

const kernel = new Kernel();
const app = kernel.app('ts-openai-cua');

if (!process.env.OPENAI_API_KEY) {
  throw new Error('OPENAI_API_KEY is not set');
}

/**
 * Example app that run an agent using openai CUA
 * Args:
 *     ctx: Kernel context containing invocation information
 *     payload: An object with a `task` property
 * Returns:
 *     An answer to the task, elapsed time and optionally the messages stack
 * Invoke this via CLI:
 *  kernel login  # or: export KERNEL_API_KEY=<your_api_key>
 *  kernel deploy index.ts -e OPENAI_API_KEY=XXXXX --force
 *  kernel invoke ts-openai-cua cua-task -p "{\"task\":\"current market price range for a used dreamcast\"}"
 */

app.action<CuaInput, CuaOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: CuaInput): Promise<CuaOutput> => {
    const start = Date.now();
    if (!payload?.task) throw new Error('task is required');

    const kb = await kernel.browsers.create({ invocation_id: ctx.invocation_id });
    console.log('> Kernel browser live view url:', kb.browser_live_view_url);

    try {
      const { computer } = await computers.create({ type: 'kernel', cdp_ws_url: kb.cdp_ws_url });

      // Navigate to DuckDuckGo as starting page (less likely to trigger captchas than Google)
      await computer.goto('https://duckduckgo.com');

      const agent = new Agent({
        model: 'computer-use-preview',
        computer,
        tools: [],
        acknowledge_safety_check_callback: (m: string): boolean => {
          console.log(`> safety check: ${m}`);
          return true;
        },
      });

      // run agent and get response
      const logs = await agent.runFullTurn({
        messages: [
          {
            role: 'system',
            content: `- Current date and time: ${new Date().toISOString()} (${new Date().toLocaleDateString(
              'en-US',
              { weekday: 'long' },
            )})`,
          },
          {
            type: 'message',
            role: 'user',
            content: [{ type: 'input_text', text: payload.task }],
          },
        ],
        print_steps: true,
        debug: true,
        show_images: false,
      });

      const elapsed = parseFloat(((Date.now() - start) / 1000).toFixed(2));

      // filter only LLM messages
      const messages = logs.filter(
        (item): item is ResponseOutputMessage =>
          item.type === 'message' &&
          typeof (item as ResponseOutputMessage).role === 'string' &&
          Array.isArray((item as ResponseOutputMessage).content),
      );
      const assistant = messages.find((m) => m.role === 'assistant');
      const lastContentIndex = assistant?.content?.length ? assistant.content.length - 1 : -1;
      const lastContent = lastContentIndex >= 0 ? assistant?.content?.[lastContentIndex] : null;
      const answer = lastContent && 'text' in lastContent ? lastContent.text : null;

      return {
        // logs, // optionally, get the full agent run messages logs
        elapsed,
        answer,
      };
    } catch (error) {
      const elapsed = parseFloat(((Date.now() - start) / 1000).toFixed(2));
      console.error('Error in cua-task:', error);
      return {
        elapsed,
        answer: null,
      };
    } finally {
      await kernel.browsers.deleteByID(kb.session_id);
    }
  },
);
