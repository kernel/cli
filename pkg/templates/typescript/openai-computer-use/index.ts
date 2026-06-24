import { Kernel, type KernelContext } from '@onkernel/sdk';
import { CuaAgent } from '@onkernel/cua-agent';
import type { AssistantMessage } from '@onkernel/cua-ai';
import { maybeStartReplay, maybeStopReplay } from './lib/replay';

const kernel = new Kernel();
const app = kernel.app('ts-openai-cua');

interface CuaInput {
  task: string;
  replay?: boolean;
}
interface CuaOutput {
  elapsed: number;
  answer: string | null;
  replay_url?: string;
}

if (!process.env.OPENAI_API_KEY) {
  throw new Error('OPENAI_API_KEY is not set');
}

app.action<CuaInput, CuaOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: CuaInput): Promise<CuaOutput> => {
    const start = Date.now();
    if (!payload?.task) throw new Error('task is required');

    const browser = await kernel.browsers.create({ invocation_id: ctx.invocation_id });
    console.log('Kernel browser live view url:', browser.browser_live_view_url);

    const replay = await maybeStartReplay(kernel, browser.session_id, {
      enabled: payload.replay === true,
    });
    let answer: string | null = null;
    let replayUrl: string | null = null;

    try {
      const agent = new CuaAgent({
        browser,
        client: kernel,
        // OpenAI's computer tool has no native URL navigation; this exposes a
        // goto/back/forward/url helper so the model can open pages directly.
        computerUseExtra: true,
        initialState: {
          model: 'openai:gpt-5.5',
          systemPrompt: `You are operating a Chromium browser on a Kernel cloud VM. Use the navigation tool to open URLs directly, and review the screenshot after each action before continuing. The current date and time is ${new Date().toISOString()}.`,
        },
      });

      await agent.prompt(payload.task);

      const lastAssistant = [...agent.state.messages]
        .reverse()
        .find((message): message is AssistantMessage => message.role === 'assistant');
      answer = lastAssistant?.content
        .flatMap((block) => (block.type === 'text' ? [block.text] : []))
        .join('') || null;
    } catch (error) {
      console.error('Error in cua-task:', error);
      answer = null;
    } finally {
      replayUrl = await maybeStopReplay(kernel, browser.session_id, replay);
      await kernel.browsers.deleteByID(browser.session_id);
    }

    const elapsed = parseFloat(((Date.now() - start) / 1000).toFixed(2));
    return replayUrl ? { elapsed, answer, replay_url: replayUrl } : { elapsed, answer };
  },
);
