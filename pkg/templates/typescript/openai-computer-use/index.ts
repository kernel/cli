import { Kernel, type KernelContext } from '@onkernel/sdk';
import * as dotenv from 'dotenv';
import type { ResponseItem, ResponseOutputMessage } from 'openai/resources/responses/responses';
import { Agent } from './lib/agent';
import { KernelComputer } from './lib/kernel-computer';
import { maybeStartReplay, maybeStopReplay } from './lib/replay';
import {
  createEventLogger,
  emitBrowserDeleteDone,
  emitBrowserDeleteStarted,
  emitBrowserNewDone,
  emitBrowserNewStarted,
  emitSessionState,
} from './lib/logging';

dotenv.config({ override: true, quiet: true });

interface CuaInput {
  task: string;
  replay?: boolean;
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
    const onEvent = createEventLogger();

    emitBrowserNewStarted(onEvent);
    const browserCreateStartedAt = Date.now();
    const kb = await kernel.browsers.create({ invocation_id: ctx.invocation_id });
    emitBrowserNewDone(onEvent, browserCreateStartedAt, kb.browser_live_view_url);
    emitSessionState(onEvent, kb.session_id, kb.browser_live_view_url);

    const computer = new KernelComputer(kernel, kb.session_id, onEvent);
    const replay = await maybeStartReplay(kernel, kb.session_id, {
      enabled: payload.replay === true,
      onEvent,
    });
    let answer: string | null = null;
    let replayUrl: string | null = null;

    try {
      await computer.goto('https://duckduckgo.com');

      const agent = new Agent({
        model: 'gpt-5.4',
        computer,
        tools: [],
        acknowledge_safety_check_callback: (m: string): boolean => {
          console.log(`> safety check: ${m}`);
          return true;
        },
      });

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
        debug: false,
        show_images: false,
        onEvent,
      });

      const elapsed = parseFloat(((Date.now() - start) / 1000).toFixed(2));

      const messages = logs.filter(
        (item): item is ResponseOutputMessage =>
          item.type === 'message' &&
          typeof (item as ResponseOutputMessage).role === 'string' &&
          Array.isArray((item as ResponseOutputMessage).content),
      );
      const assistant = messages.find((m) => m.role === 'assistant');
      const lastContentIndex = assistant?.content?.length ? assistant.content.length - 1 : -1;
      const lastContent = lastContentIndex >= 0 ? assistant?.content?.[lastContentIndex] : null;
      answer = lastContent && 'text' in lastContent ? lastContent.text : null;
    } catch (error) {
      console.error('Error in cua-task:', error);
      answer = null;
    } finally {
      emitBrowserDeleteStarted(onEvent);
      const browserDeleteStartedAt = Date.now();
      try {
        replayUrl = await maybeStopReplay(kernel, kb.session_id, replay, { onEvent });
        await kernel.browsers.deleteByID(kb.session_id);
      } finally {
        emitBrowserDeleteDone(onEvent, browserDeleteStartedAt);
      }
    }

    const elapsed = parseFloat(((Date.now() - start) / 1000).toFixed(2));
    return replayUrl ? { elapsed, answer, replay_url: replayUrl } : { elapsed, answer };
  },
);
