import 'dotenv/config';
import { Agent } from './lib/agent';
import computers from './lib/computers';

/*
  to run a local browser test before deploying to kernel
*/

async function test(): Promise<void> {
  const { computer } = await computers.create({ type: 'local' });
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
}

test();
