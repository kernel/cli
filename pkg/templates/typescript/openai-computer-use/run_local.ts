import * as dotenv from 'dotenv';
import { Kernel } from '@onkernel/sdk';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Agent } from './lib/agent';
import { KernelComputer } from './lib/kernel-computer';
import {
  createEventLogger,
  emitBrowserDeleteDone,
  emitBrowserDeleteStarted,
  emitBrowserNewDone,
  emitBrowserNewStarted,
  emitSessionState,
} from './lib/logging';
import type { OutputMode } from './lib/log-events';

dotenv.config({ override: true, quiet: true });

/**
 * Local test script that creates a remote Kernel browser and runs the CUA agent.
 * No Kernel app deployment needed.
 *
 * Usage:
 *   KERNEL_API_KEY=... OPENAI_API_KEY=... npx tsx run_local.ts
 */

export async function runLocalTest(args: string[] = process.argv.slice(2)): Promise<void> {
  if (!process.env.KERNEL_API_KEY) throw new Error('KERNEL_API_KEY is not set');
  if (!process.env.OPENAI_API_KEY) throw new Error('OPENAI_API_KEY is not set');

  const client = new Kernel({ apiKey: process.env.KERNEL_API_KEY });
  const outputMode = parseOutputMode(args);
  const debug = args.includes('--debug');
  const onEvent = createEventLogger({ output: outputMode, verbose: debug });

  emitBrowserNewStarted(onEvent);
  const browserCreateStartedAt = Date.now();
  const browser = await client.browsers.create({ timeout_seconds: 300 });
  emitBrowserNewDone(onEvent, browserCreateStartedAt, browser.browser_live_view_url);
  emitSessionState(onEvent, browser.session_id, browser.browser_live_view_url);

  const computer = new KernelComputer(client, browser.session_id, onEvent);

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

    await agent.runFullTurn({
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
      debug,
      show_images: false,
      onEvent,
    });
  } finally {
    emitBrowserDeleteStarted(onEvent);
    const browserDeleteStartedAt = Date.now();
    try {
      await client.browsers.deleteByID(browser.session_id);
    } finally {
      emitBrowserDeleteDone(onEvent, browserDeleteStartedAt);
    }
    console.log('> Browser session deleted');
  }
}

function parseOutputMode(args: string[]): OutputMode {
  const outputArg = args.find((arg) => arg.startsWith('--output='));
  const outputFromEquals = outputArg?.split('=')[1];
  const outputFlagIndex = args.findIndex((arg) => arg === '--output');
  const outputFromNext = outputFlagIndex >= 0 ? args[outputFlagIndex + 1] : undefined;
  const output = outputFromEquals ?? outputFromNext;
  return output === 'jsonl' ? 'jsonl' : 'text';
}

function isDirectRun(): boolean {
  const entry = process.argv[1];
  if (!entry) return false;
  return resolve(entry) === resolve(fileURLToPath(import.meta.url));
}

if (isDirectRun()) {
  runLocalTest().catch((error) => {
    console.error(error);
    process.exit(1);
  });
}
