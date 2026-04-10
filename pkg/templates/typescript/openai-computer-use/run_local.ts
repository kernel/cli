import * as dotenv from 'dotenv';
import { Kernel } from '@onkernel/sdk';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
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

/**
 * Local test script that creates a remote Kernel browser and runs the CUA agent.
 * No Kernel app deployment needed.
 *
 * Usage:
 *   KERNEL_API_KEY=... OPENAI_API_KEY=... npx tsx run_local.ts --task "go to example.com and summarize it"
 */

const DEFAULT_TASK = 'go to example.com and summarize what the page says';

export async function runLocalTest(args: string[] = process.argv.slice(2)): Promise<void> {
  if (!process.env.KERNEL_API_KEY) throw new Error('KERNEL_API_KEY is not set');
  if (!process.env.OPENAI_API_KEY) throw new Error('OPENAI_API_KEY is not set');

  const client = new Kernel({ apiKey: process.env.KERNEL_API_KEY });
  const task = parseTask(args);
  const replayEnabled = parseReplay(args);
  const debug = args.includes('--debug');
  const onEvent = createEventLogger({ verbose: debug });

  emitBrowserNewStarted(onEvent);
  const browserCreateStartedAt = Date.now();
  const browser = await client.browsers.create({ timeout_seconds: 300 });
  emitBrowserNewDone(onEvent, browserCreateStartedAt, browser.browser_live_view_url);
  emitSessionState(onEvent, browser.session_id, browser.browser_live_view_url);

  const computer = new KernelComputer(client, browser.session_id, onEvent);
  const replay = await maybeStartReplay(client, browser.session_id, {
    enabled: replayEnabled,
    onEvent,
  });

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
              text: task,
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
      const replayUrl = await maybeStopReplay(client, browser.session_id, replay, { onEvent });
      if (replayUrl) {
        console.log(`> Replay URL: ${replayUrl}`);
      }
      await client.browsers.deleteByID(browser.session_id);
    } finally {
      emitBrowserDeleteDone(onEvent, browserDeleteStartedAt);
    }
    console.log('> Browser session deleted');
  }
}

function parseTask(args: string[]): string {
  const taskFromEquals = args.find((arg) => arg.startsWith('--task='))?.slice('--task='.length).trim();
  const taskFlagIndex = args.findIndex((arg) => arg === '--task');
  const nextArg = taskFlagIndex >= 0 ? args[taskFlagIndex + 1] : undefined;
  const taskFromNext = nextArg && !nextArg.startsWith('--') ? nextArg.trim() : undefined;
  const task = taskFromEquals || taskFromNext;
  return task && task.length > 0 ? task : DEFAULT_TASK;
}

function parseReplay(args: string[]): boolean {
  const replayFromEquals = args.find((arg) => arg.startsWith('--replay='))?.slice('--replay='.length).trim();
  if (replayFromEquals) {
    return !['0', 'false', 'no', 'off'].includes(replayFromEquals.toLowerCase());
  }
  return args.includes('--replay');
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
