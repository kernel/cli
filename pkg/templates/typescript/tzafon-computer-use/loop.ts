/**
 * Tzafon Northstar Sampling Loop
 * 
 * Implements the agent loop for Tzafon's Northstar CUA Fast model.
 * Northstar uses the Lightcone SDK with a responses API:
 * - Actions are returned via computer_call outputs
 * - Tool results use computer_call_output with screenshot images
 * - The model stops when no computer_call is in the output or the action is terminal
 * - Continuation uses previous_response_id for multi-turn context
 * 
 * @see https://docs.lightcone.ai
 */

import type { Kernel } from '@onkernel/sdk';
import Lightcone from '@tzafon/lightcone';
import { ComputerTool } from './tools/computer';

const MODEL = 'tzafon.northstar-cua-fast';

const TOOL = {
  type: 'computer_use' as const,
  display_width: 1280,
  display_height: 800,
  environment: 'browser' as const,
};

const TERMINAL_ACTIONS = new Set(['terminate', 'done', 'answer']);

interface SamplingLoopOptions {
  task: string;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  model?: string;
  maxSteps?: number;
  viewportWidth?: number;
  viewportHeight?: number;
}

interface SamplingLoopResult {
  messages: string[];
  finalResult?: string;
}

export async function samplingLoop({
  task,
  apiKey,
  kernel,
  sessionId,
  model = MODEL,
  maxSteps = 50,
  viewportWidth = 1280,
  viewportHeight = 800,
}: SamplingLoopOptions): Promise<SamplingLoopResult> {
  const tzafon = new Lightcone({ apiKey });
  const computer = new ComputerTool(kernel, sessionId, viewportWidth, viewportHeight);

  const tool = {
    ...TOOL,
    display_width: viewportWidth,
    display_height: viewportHeight,
  };

  let screenshotUrl = await computer.captureScreenshot();

  let response = await tzafon.responses.create({
    model,
    tools: [tool],
    input: [
      {
        role: 'user',
        content: [
          { type: 'input_text', text: task },
          { type: 'input_image', image_url: screenshotUrl },
        ],
      },
    ],
  });

  let finalResult: string | undefined;

  for (let step = 0; step < maxSteps; step++) {
    const computerCall = (response.output ?? []).find(
      (o: any) => o.type === 'computer_call',
    );
    if (!computerCall) break;

    const action = (computerCall as any).action;

    const label = [
      action.type,
      action.x != null ? `@ (${action.x}, ${action.y})` : '',
      action.text ? `'${action.text}'` : '',
    ]
      .filter(Boolean)
      .join(' ');
    console.log(`[${step + 1}] ${label}`);

    if (TERMINAL_ACTIONS.has(action.type)) {
      finalResult = action.result ?? action.text ?? action.status;
      console.log(`Result: ${finalResult}`);
      break;
    }

    await computer.execute(action);
    await new Promise((r) => setTimeout(r, 1000));

    screenshotUrl = await computer.captureScreenshot();
    response = await tzafon.responses.create({
      model,
      previous_response_id: response.id,
      tools: [tool],
      input: [
        {
          type: 'computer_call_output',
          call_id: (computerCall as any).call_id,
          output: { type: 'input_image', image_url: screenshotUrl },
        },
      ],
    });
  }

  const messages = (response.output ?? [])
    .filter((o: any) => o.type === 'message')
    .flatMap((o: any) => (o.content ?? []).map((c: any) => c.text))
    .filter(Boolean);

  return {
    messages,
    finalResult,
  };
}
