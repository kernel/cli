/**
 * QA Computer Use Loop
 * 
 * Uses Anthropic Computer Use to navigate, scroll, dismiss popups, and capture screenshots
 * for QA analysis.
 */

import { Anthropic } from '@anthropic-ai/sdk';
import { DateTime } from 'luxon';
import { Buffer } from 'buffer';
import { createHash } from 'crypto';
import type { Kernel } from '@onkernel/sdk';
import { DEFAULT_TOOL_VERSION, TOOL_GROUPS_BY_VERSION, ToolCollection, type ToolVersion } from '../tools/collection';
import { ComputerTool20241022, ComputerTool20250124 } from '../tools/computer';
import type { ActionParams } from '../tools/types/computer';
import { Action } from '../tools/types/computer';
import type { BetaMessageParam, BetaTextBlock, BetaContentBlock } from '../types/beta';
import { injectPromptCaching, PROMPT_CACHING_BETA_FLAG, responseToParams } from '../utils/computer-use';
import { makeApiToolResult } from '../utils/tool-results';

const SYSTEM_PROMPT = `<SYSTEM_CAPABILITY>
* You are utilising an Ubuntu virtual machine using ${process.arch} architecture with internet access.
* When you connect to the display, CHROMIUM IS ALREADY OPEN. The url bar is not visible but it is there.
* If you need to navigate to a new page, use ctrl+l to focus the url bar and then enter the url.
* You won't be able to see the url bar from the screenshot but ctrl-l still works.
* The current date is ${DateTime.now().toFormat('EEEE, MMMM d, yyyy')}.
* Be efficient: Only take screenshots when the page content has changed significantly or when explicitly needed.
* When scrolling, use larger scroll amounts (e.g., 3-5 scroll units) to move through the page faster.
* After navigation and key actions, take a screenshot to verify the outcome.
</SYSTEM_CAPABILITY>`;

interface ToolUseInput extends Record<string, unknown> {
  action: Action;
}

export interface QaScreenshot {
  base64: string;
  buffer: Buffer;
  url: string;
}

export interface QaNavigationResult {
  success: boolean;
  screenshots: QaScreenshot[];
  message: string;
}

/**
 * Navigate to a URL, scroll through the page, dismiss popups, and capture screenshots
 * using Anthropic Computer Use.
 */
export async function navigateAndCaptureScreenshots({
  url,
  dismissPopups,
  apiKey,
  kernel,
  sessionId,
  maxTokens = 4096,
  toolVersion,
  thinkingBudget = 1024,
}: {
  url: string;
  dismissPopups?: boolean;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  maxTokens?: number;
  toolVersion?: ToolVersion;
  thinkingBudget?: number;
}): Promise<QaNavigationResult> {
  
  const selectedVersion = toolVersion || DEFAULT_TOOL_VERSION;
  const toolGroup = TOOL_GROUPS_BY_VERSION[selectedVersion];
  const toolCollection = new ToolCollection(...toolGroup.tools.map((Tool: typeof ComputerTool20241022 | typeof ComputerTool20250124) => new Tool(kernel, sessionId)));

  const taskPrompt = `Your task is to:
1. Navigate to ${url} (use ctrl+l to focus the URL bar, then type the URL and press Enter)
2. Wait for the page to fully load (take a screenshot to verify)
3. ${dismissPopups ? 'Dismiss any popups, cookie banners, or modals that appear' : ''}
4. Efficiently scroll through the page using larger scroll amounts (3-5 units) to load lazy-loaded content
5. Take a screenshot only when you reach a new section or when content visibly changes
6. Once you've scrolled to the bottom and loaded all content, take a final screenshot
7. Confirm you have successfully navigated to the page and captured all content

Be efficient: Avoid taking screenshots after every small scroll. Only capture when content changes significantly.`;

  const messages: BetaMessageParam[] = [{
    role: 'user',
    content: taskPrompt,
  }];

  const system: BetaTextBlock = {
    type: 'text',
    text: SYSTEM_PROMPT,
  };

  const client = new Anthropic({ apiKey, maxRetries: 4 });
  const betas = [toolGroup.beta_flag, PROMPT_CACHING_BETA_FLAG];

  const screenshots: QaScreenshot[] = [];
  const screenshotHashes = new Set<string>(); // Track screenshot hashes to avoid duplicates
  let finalMessage = '';
  let iterationCount = 0;
  const MAX_ITERATIONS = 30; // Prevent infinite loops

  while (iterationCount < MAX_ITERATIONS) {
    iterationCount++;
    injectPromptCaching(messages);
    (system as BetaTextBlock).cache_control = { type: 'ephemeral' };

    const response = await client.beta.messages.create({
      max_tokens: maxTokens,
      messages,
      model: 'claude-sonnet-4-5-20250929',
      system: [system],
      tools: toolCollection.toParams() as any, // Computer Use tools use special format
      betas,
      thinking: { type: 'enabled', budget_tokens: thinkingBudget },
    });

    const responseParams = responseToParams(response);

    console.log('=== LLM RESPONSE ===');
    console.log('Stop reason:', response.stop_reason);

    messages.push({ role: 'assistant', content: responseParams });

    if (response.stop_reason === 'end_turn') {
      const lastTextBlock = responseParams.find((b) => b.type === 'text' && 'text' in b);
      finalMessage = (lastTextBlock && typeof lastTextBlock === 'object' && 'text' in lastTextBlock ? (lastTextBlock as { text: string }).text : '') || 'Navigation completed';
      
      const success = finalMessage.toLowerCase().includes('success') || 
                    finalMessage.toLowerCase().includes('completed') ||
                    finalMessage.toLowerCase().includes('navigated');
      
      return { success, screenshots, message: finalMessage };
    }

    const toolResultContent = [];

    for (const contentBlock of responseParams) {
      if (contentBlock.type === 'tool_use' && contentBlock.name && contentBlock.input) {
        const input = contentBlock.input as ToolUseInput;
        if ('action' in input && typeof input.action === 'string') {
          const toolInput: ActionParams = {
            action: input.action as Action,
            ...Object.fromEntries(Object.entries(input).filter(([key]) => key !== 'action'))
          };

          try {
            const result = await toolCollection.run(contentBlock.name, toolInput);
            
            // Capture screenshots from tool results, but skip near-duplicates
            if (result.base64Image) {
              const buffer = Buffer.from(result.base64Image, 'base64');
              
              // Create a hash of the image to detect duplicates
              // Use a simple hash of buffer size + first 1KB to quickly detect identical/similar images
              const hashInput = `${buffer.length}-${buffer.subarray(0, Math.min(1024, buffer.length)).toString('base64').substring(0, 100)}`;
              const hash = createHash('md5').update(hashInput).digest('hex');
              
              // Only store if this is a new screenshot (different hash)
              // Always store the first screenshot and screenshots from explicit SCREENSHOT actions
              const isExplicitScreenshot = toolInput.action === Action.SCREENSHOT;
              const isFirstScreenshot = screenshots.length === 0;
              
              if (!screenshotHashes.has(hash) && (isExplicitScreenshot || isFirstScreenshot || screenshots.length < 5)) {
                screenshotHashes.add(hash);
                screenshots.push({
                  base64: result.base64Image,
                  buffer,
                  url,
                });
                console.log(`Screenshot ${screenshots.length} stored (hash: ${hash.substring(0, 8)}...)`);
              } else if (!screenshotHashes.has(hash)) {
                // Store unique screenshots up to a reasonable limit, then only store if significantly different
                screenshotHashes.add(hash);
                screenshots.push({
                  base64: result.base64Image,
                  buffer,
                  url,
                });
                console.log(`Screenshot ${screenshots.length} stored (new content detected)`);
              } else {
                console.log(`Skipping duplicate screenshot (hash: ${hash.substring(0, 8)}...)`);
              }
            }
            
            const toolResult = makeApiToolResult(result, contentBlock.id!);
            toolResultContent.push(toolResult);
          } catch (error) {
            console.error('Tool execution error:', error);
            throw error;
          }
        }
      }
    }

    if (toolResultContent.length > 0) {
      messages.push({ role: 'user', content: toolResultContent });
    } else if (response.stop_reason !== 'tool_use') {
      return { success: false, screenshots, message: 'Loop ended without completion' };
    }
  }

  // If we hit max iterations, return what we have
  if (iterationCount >= MAX_ITERATIONS) {
    console.log(`Reached max iterations (${MAX_ITERATIONS}), stopping navigation loop`);
    return { 
      success: screenshots.length > 0, 
      screenshots, 
      message: `Navigation stopped after ${iterationCount} iterations. Captured ${screenshots.length} unique screenshots.` 
    };
  }

  return { success: false, screenshots, message: 'Unexpected loop exit' };
}
