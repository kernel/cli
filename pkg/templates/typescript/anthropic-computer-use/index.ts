import { Kernel, type KernelContext } from '@onkernel/sdk';
import { chromium } from 'playwright-core';
import { samplingLoop } from './loop';

const kernel = new Kernel();

const app = kernel.app('ts-anthropic-cua');

interface QueryInput {
  query: string;
}

interface QueryOutput {
  result: string;
}

// LLM API Keys are set in the environment during `kernel deploy <filename> -e ANTHROPIC_API_KEY=XXX`
// See https://www.kernel.sh/docs/launch/deploy#environment-variables
const ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;

if (!ANTHROPIC_API_KEY) {
  throw new Error('ANTHROPIC_API_KEY is not set');
}

app.action<QueryInput, QueryOutput>(
  'cua-task',
  async (ctx: KernelContext, payload?: QueryInput): Promise<QueryOutput> => {
    if (!payload?.query) {
      throw new Error('Query is required');
    }

    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
      stealth: true,
    });

    console.log("Kernel browser live view url: ", kernelBrowser.browser_live_view_url);

    const browser = await chromium.connectOverCDP(kernelBrowser.cdp_ws_url);
    const context = await browser.contexts()[0];
    const page = await context?.pages()[0];
    if (!page) {
      throw new Error('Error getting initial page');
    }

    try {
      // Run the sampling loop
      const finalMessages = await samplingLoop({
        model: 'claude-sonnet-4-20250514',
        messages: [{
          role: 'user',
          content: payload.query,
        }],
        apiKey: ANTHROPIC_API_KEY,
        thinkingBudget: 1024,
        playwrightPage: page,
      });

      // Extract the final result from the messages
      if (finalMessages.length === 0) {
        throw new Error('No messages were generated during the sampling loop');
      }

      const lastMessage = finalMessages[finalMessages.length - 1];
      if (!lastMessage) {
        throw new Error('Failed to get the last message from the sampling loop');
      }

      const result = typeof lastMessage.content === 'string'
        ? lastMessage.content
        : lastMessage.content.map(block =>
          block.type === 'text' ? block.text : ''
        ).join('');

      return { result };
    } catch (error) {
      console.error('Error in sampling loop:', error);
      throw error;
    } finally {
      await browser.close();
      await kernel.browsers.deleteByID(kernelBrowser.session_id);
    }
  },
);
