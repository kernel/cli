import 'dotenv/config';
import { Kernel } from '@onkernel/sdk';
import { Agent } from './lib/agent';
import { KernelComputer } from './lib/kernel-computer';

/**
 * Local test script that creates a remote Kernel browser and runs the CUA agent.
 * No Kernel app deployment needed.
 *
 * Usage:
 *   KERNEL_API_KEY=... OPENAI_API_KEY=... npx tsx test.local.ts
 */

async function test(): Promise<void> {
  if (!process.env.KERNEL_API_KEY) throw new Error('KERNEL_API_KEY is not set');
  if (!process.env.OPENAI_API_KEY) throw new Error('OPENAI_API_KEY is not set');

  const client = new Kernel({ apiKey: process.env.KERNEL_API_KEY });
  const browser = await client.browsers.create({ timeout_seconds: 300 });
  console.log('> Browser session:', browser.session_id);
  console.log('> Live view:', browser.browser_live_view_url);

  const computer = new KernelComputer(client, browser.session_id);

  try {
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
          content: [
            {
              type: 'input_text',
              text: 'go to ebay.com and look up oberheim ob-x prices and give me a report',
            },
          ],
        },
      ],
      print_steps: true,
      debug: true,
      show_images: false,
    });
    console.dir(logs, { depth: null });
  } finally {
    await client.browsers.deleteByID(browser.session_id);
    console.log('> Browser session deleted');
  }
}

test();
