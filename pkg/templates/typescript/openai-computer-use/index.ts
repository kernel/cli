import { Kernel, type KernelContext } from '@onkernel/sdk';
import 'dotenv/config';
import type { ResponseItem, ResponseOutputMessage } from 'openai/resources/responses/responses';
import { Agent } from './lib/agent';
import computers from './lib/computers';
import { KernelBrowserSession } from './lib/session';

interface CuaInput {
  task: string;
  record_replay?: boolean;
}
interface CuaOutput {
  elapsed: number;
  answer: string | null;
  replay_url?: string;
  logs?: ResponseItem[];
}

const kernel = new Kernel();
const app = kernel.app('ts-openai-cua');

if (!process.env.OPENAI_API_KEY) {
  throw new Error('OPENAI_API_KEY is not set');
}

/**
 * Example app that runs an agent using OpenAI CUA with Kernel Computer Controls API.
 *
 * This uses OS-level input emulation (mouse, keyboard) instead of CDP/Playwright,
 * which reduces bot detection signals.
 *
 * Args:
 *     ctx: Kernel context containing invocation information
 *     payload: An object with a `task` property and optional `record_replay` flag
 * Returns:
 *     An answer to the task, elapsed time, optional replay URL, and optionally the messages stack
 *
 * Invoke this via CLI:
 *  kernel login  # or: export KERNEL_API_KEY=<your_api_key>
 *  kernel deploy index.ts -e OPENAI_API_KEY=XXXXX --force
 *  kernel invoke ts-openai-cua cua-task -p '{"task":"current market price range for a used dreamcast"}'
 */

app.action<CuaInput, CuaOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: CuaInput): Promise<CuaOutput> => {
    const start = Date.now();
    if (!payload?.task) throw new Error('task is required');

    const session = new KernelBrowserSession(kernel, {
      stealth: true,
      recordReplay: payload.record_replay ?? false,
      invocationId: ctx.invocation_id,
    });

    await session.start();
    console.log('> Kernel browser live view url:', session.liveViewUrl);

    try {
      const { computer } = await computers.create({
        type: 'kernel-computer',
        kernel,
        sessionId: session.sessionId,
      });

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
      const sessionInfo = await session.stop();

      return {
        // logs, // optionally, get the full agent run messages logs
        elapsed,
        answer,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      const elapsed = parseFloat(((Date.now() - start) / 1000).toFixed(2));
      console.error('Error in cua-task:', error);
      await session.stop();
      return {
        elapsed,
        answer: null,
      };
    }
  },
);
