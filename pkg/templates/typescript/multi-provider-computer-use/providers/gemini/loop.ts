/**
 * Gemini Computer Use sampling loop.
 * Based on Google's computer-use-preview reference implementation.
 */

import { GoogleGenAI, type Content, type FunctionCall, type Part } from '@google/genai';
import type { Kernel } from '@onkernel/sdk';
import { ComputerTool } from './computer';
import { PREDEFINED_COMPUTER_USE_FUNCTIONS, DEFAULT_SCREEN_SIZE, type GeminiFunctionArgs, type ScreenSize } from './types';

// System prompt for browser-based computer use
function getSystemPrompt(): string {
  const currentDate = new Date().toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });

  return `You are a helpful assistant that can use a web browser.
You are operating a Chrome browser through computer use tools.
The browser is already open and ready for use.

When you need to navigate to a page, use the navigate action with a full URL.
When you need to interact with elements, use click_at, type_text_at, etc.
After each action, carefully evaluate the screenshot to determine your next step.

Current date: ${currentDate}.`;
}

// Maximum number of recent turns to keep screenshots for (to manage context)
const MAX_RECENT_TURN_WITH_SCREENSHOTS = 3;

export interface GeminiLoopOptions {
  query: string;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  model?: string;
  maxIterations?: number;
  systemPromptSuffix?: string;
  screenSize?: ScreenSize;
}

export interface GeminiLoopResult {
  result: string;
  error?: string;
}

/**
 * Run the Gemini computer use sampling loop.
 */
export async function runGeminiLoop(options: GeminiLoopOptions): Promise<GeminiLoopResult> {
  const {
    query,
    apiKey,
    kernel,
    sessionId,
    model = 'gemini-2.5-computer-use-preview-10-2025',
    maxIterations = 50,
    systemPromptSuffix = '',
    screenSize = DEFAULT_SCREEN_SIZE,
  } = options;

  const ai = new GoogleGenAI({ apiKey });

  const computerTool = new ComputerTool(kernel, sessionId, screenSize);

  // Initialize conversation with user query
  const contents: Content[] = [
    {
      role: 'user',
      parts: [{ text: query }],
    },
  ];

  const basePrompt = getSystemPrompt();
  const systemPrompt = systemPromptSuffix ? `${basePrompt}\n\n${systemPromptSuffix}` : basePrompt;

  let iteration = 0;
  let finalResponse = '';
  let error: string | undefined;

  while (iteration < maxIterations) {
    iteration++;
    console.log(`\n=== Iteration ${iteration} ===`);

    try {
      // Generate response from Gemini
      const response = await ai.models.generateContent({
        model,
        contents,
        config: {
          temperature: 1,
          topP: 0.95,
          topK: 40,
          maxOutputTokens: 8192,
          systemInstruction: systemPrompt,
          tools: [
            {
              computerUse: {
                environment: 'ENVIRONMENT_BROWSER',
              },
            },
          ],
          thinkingConfig: {
            includeThoughts: true,
          },
        },
      });

      if (!response.candidates || response.candidates.length === 0) {
        console.log('No candidates in response');
        break;
      }

      const candidate = response.candidates[0];
      if (!candidate.content) {
        console.log('No content in candidate');
        break;
      }

      // Add assistant response to conversation
      contents.push(candidate.content);

      // Extract text and function calls
      const reasoning = extractText(candidate.content);
      const functionCalls = extractFunctionCalls(candidate.content);

      // Log the response
      console.log('Reasoning:', reasoning || '(none)');
      console.log('Function calls:', functionCalls.length);
      for (const fc of functionCalls) {
        console.log(`  - ${fc.name}:`, fc.args);
      }

      // Check finish reason
      const finishReason = candidate.finishReason;
      if (finishReason === 'MALFORMED_FUNCTION_CALL' && !functionCalls.length) {
        console.log('Malformed function call, retrying...');
        continue;
      }

      // If no function calls, the model is done
      if (functionCalls.length === 0) {
        console.log('Agent loop complete');
        finalResponse = reasoning || '';
        break;
      }

      // Execute function calls and collect results
      const functionResponses: Part[] = [];
      for (const fc of functionCalls) {
        const args = (fc.args as GeminiFunctionArgs) || {};

        // Handle safety decisions if present
        if (args.safety_decision?.decision === 'require_confirmation') {
          console.log('Safety confirmation required:', args.safety_decision.explanation);
          // Auto-acknowledge for automated execution
          console.log('Auto-acknowledging safety check');
        }

        // Execute the action
        console.log(`Executing action: ${fc.name}`);
        const result = await computerTool.executeAction(fc.name, args);

        if (result.error) {
          console.log(`Action error: ${result.error}`);
          functionResponses.push({
            functionResponse: {
              name: fc.name,
              // Always include URL (required by Gemini Computer Use API)
              response: { error: result.error, url: result.url || 'about:blank' },
            },
          });
        } else {
          // Build response with screenshot - always include URL (required by Computer Use API)
          const responseData: Record<string, unknown> = {
            url: result.url || 'about:blank',
          };

          functionResponses.push({
            functionResponse: {
              name: fc.name,
              response: responseData,
              // Include screenshot as inline data
              ...(result.base64Image && isPredefinedFunction(fc.name)
                ? {
                    parts: [
                      {
                        inlineData: {
                          mimeType: 'image/png',
                          data: result.base64Image,
                        },
                      },
                    ],
                  }
                : {}),
            },
          });
        }
      }

      // Add function responses to conversation
      contents.push({
        role: 'user',
        parts: functionResponses,
      });

      // Manage screenshot history to avoid context overflow
      pruneOldScreenshots(contents);
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
      console.error('Error in sampling loop:', error);
      break;
    }
  }

  if (iteration >= maxIterations) {
    console.log('Max iterations reached');
  }

  return {
    result: finalResponse || 'Task completed',
    error,
  };
}

function extractText(content: Content): string {
  if (!content.parts) return '';

  const texts: string[] = [];
  for (const part of content.parts) {
    if ('text' in part && part.text) {
      texts.push(part.text);
    }
  }
  return texts.join(' ');
}

function extractFunctionCalls(content: Content): FunctionCall[] {
  if (!content.parts) return [];

  const calls: FunctionCall[] = [];
  for (const part of content.parts) {
    if ('functionCall' in part && part.functionCall) {
      calls.push(part.functionCall);
    }
  }
  return calls;
}

function isPredefinedFunction(name: string): boolean {
  return PREDEFINED_COMPUTER_USE_FUNCTIONS.includes(name as (typeof PREDEFINED_COMPUTER_USE_FUNCTIONS)[number]);
}

function pruneOldScreenshots(contents: Content[]): void {
  let turnsWithScreenshots = 0;

  // Iterate in reverse to find recent turns with screenshots
  for (let i = contents.length - 1; i >= 0; i--) {
    const content = contents[i];
    if (content.role !== 'user' || !content.parts) continue;

    // Check if this turn has screenshots from predefined functions
    let hasScreenshot = false;
    for (const part of content.parts) {
      if ('functionResponse' in part && part.functionResponse && isPredefinedFunction(part.functionResponse.name || '')) {
        // Check if it has inline data (screenshot)
        const fr = part.functionResponse as { parts?: Array<{ inlineData?: unknown }> };
        if (fr.parts?.some((p) => p.inlineData)) {
          hasScreenshot = true;
          break;
        }
      }
    }

    if (hasScreenshot) {
      turnsWithScreenshots++;

      // Remove screenshots from old turns
      if (turnsWithScreenshots > MAX_RECENT_TURN_WITH_SCREENSHOTS) {
        for (const part of content.parts) {
          if (
            'functionResponse' in part &&
            part.functionResponse &&
            isPredefinedFunction(part.functionResponse.name || '')
          ) {
            // Remove the parts array (which contains the screenshot)
            const fr = part.functionResponse as { parts?: unknown };
            delete fr.parts;
          }
        }
      }
    }
  }
}
