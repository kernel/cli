import { Anthropic } from '@anthropic-ai/sdk';
import { DateTime } from 'luxon';
import type { Kernel } from '@onkernel/sdk';
import { DEFAULT_TOOL_VERSION, TOOL_GROUPS_BY_VERSION, ToolCollection, type ToolVersion } from './collection';
import { ComputerTool20241022, ComputerTool20250124 } from './computer';
import type { ActionParams, BetaMessageParam, BetaTextBlock } from './types';
import { Action } from './types';
import {
  injectPromptCaching,
  maybeFilterToNMostRecentImages,
  PROMPT_CACHING_BETA_FLAG,
  responseToParams,
} from './utils/message-processing';
import { makeApiToolResult } from './utils/tool-results';

// System prompt optimized for the environment
const SYSTEM_PROMPT = `<SYSTEM_CAPABILITY>
* You are utilising an Ubuntu virtual machine using ${process.arch} architecture with internet access.
* When you connect to the display, CHROMIUM IS ALREADY OPEN. The url bar is not visible but it is there.
* If you need to navigate to a new page, use ctrl+l to focus the url bar and then enter the url.
* You won't be able to see the url bar from the screenshot but ctrl-l still works.
* As the initial step click on the search bar.
* When viewing a page it can be helpful to zoom out so that you can see everything on the page.
* Either that, or make sure you scroll down to see everything before deciding something isn't available.
* When using your computer function calls, they take a while to run and send back to you.
* Where possible/feasible, try to chain multiple of these calls all into one function calls request.
* The current date is ${DateTime.now().toFormat('EEEE, MMMM d, yyyy')}.
* After each step, take a screenshot and carefully evaluate if you have achieved the right outcome.
* Explicitly show your thinking: "I have evaluated step X..." If not correct, try again.
* Only when you confirm a step was executed correctly should you move on to the next one.
</SYSTEM_CAPABILITY>

<IMPORTANT>
* When using Chromium, if a startup wizard appears, IGNORE IT. Do not even click "skip this step".
* Instead, click on the search bar on the center of the screen where it says "Search or enter address", and enter the appropriate search term or URL there.
</IMPORTANT>`;

// Add new type definitions
interface ThinkingConfig {
  type: 'enabled';
  budget_tokens: number;
}

interface ExtraBodyConfig {
  thinking?: ThinkingConfig;
}

interface ToolUseInput extends Record<string, unknown> {
  action: Action;
}

export interface AnthropicLoopOptions {
  query: string;
  apiKey: string;
  kernel: Kernel;
  sessionId: string;
  model?: string;
  systemPromptSuffix?: string;
  onlyNMostRecentImages?: number;
  maxTokens?: number;
  toolVersion?: ToolVersion;
  thinkingBudget?: number;
  tokenEfficientToolsBeta?: boolean;
}

export interface AnthropicLoopResult {
  result: string;
  error?: string;
}

export async function runAnthropicLoop(options: AnthropicLoopOptions): Promise<AnthropicLoopResult> {
  const {
    query,
    apiKey,
    kernel,
    sessionId,
    model = 'claude-sonnet-4-5-20250929',
    systemPromptSuffix,
    onlyNMostRecentImages,
    maxTokens = 4096,
    toolVersion,
    thinkingBudget,
    tokenEfficientToolsBeta = false,
  } = options;

  const messages: BetaMessageParam[] = [
    {
      role: 'user',
      content: query,
    },
  ];

  try {
    const finalMessages = await samplingLoop({
      model,
      systemPromptSuffix,
      messages,
      apiKey,
      onlyNMostRecentImages,
      maxTokens,
      toolVersion,
      thinkingBudget,
      tokenEfficientToolsBeta,
      kernel,
      sessionId,
    });

    // Extract final text response
    const lastMessage = finalMessages[finalMessages.length - 1];
    let resultText = '';

    if (lastMessage && lastMessage.role === 'assistant' && Array.isArray(lastMessage.content)) {
      for (const block of lastMessage.content) {
        if (block.type === 'text' && 'text' in block) {
          resultText += block.text + '\n';
        }
      }
    }

    return {
      result: resultText.trim() || 'Task completed',
    };
  } catch (error) {
    return {
      result: '',
      error: error instanceof Error ? error.message : String(error),
    };
  }
}

async function samplingLoop({
  model,
  systemPromptSuffix,
  messages,
  apiKey,
  onlyNMostRecentImages,
  maxTokens = 4096,
  toolVersion,
  thinkingBudget,
  tokenEfficientToolsBeta = false,
  kernel,
  sessionId,
}: {
  model: string;
  systemPromptSuffix?: string;
  messages: BetaMessageParam[];
  apiKey: string;
  onlyNMostRecentImages?: number;
  maxTokens?: number;
  toolVersion?: ToolVersion;
  thinkingBudget?: number;
  tokenEfficientToolsBeta?: boolean;
  kernel: Kernel;
  sessionId: string;
}): Promise<BetaMessageParam[]> {
  const selectedVersion = toolVersion || DEFAULT_TOOL_VERSION;
  const toolGroup = TOOL_GROUPS_BY_VERSION[selectedVersion];
  const toolCollection = new ToolCollection(
    ...toolGroup.tools.map(
      (Tool: typeof ComputerTool20241022 | typeof ComputerTool20250124) => new Tool(kernel, sessionId)
    )
  );

  const system: BetaTextBlock = {
    type: 'text',
    text: `${SYSTEM_PROMPT}${systemPromptSuffix ? ' ' + systemPromptSuffix : ''}`,
  };

  while (true) {
    const betas: string[] = toolGroup.beta_flag ? [toolGroup.beta_flag] : [];

    if (tokenEfficientToolsBeta) {
      betas.push('token-efficient-tools-2025-02-19');
    }

    let imageTruncationThreshold = onlyNMostRecentImages || 0;

    const client = new Anthropic({ apiKey, maxRetries: 4 });
    const enablePromptCaching = true;

    if (enablePromptCaching) {
      betas.push(PROMPT_CACHING_BETA_FLAG);
      injectPromptCaching(messages);
      onlyNMostRecentImages = 0;
      (system as BetaTextBlock).cache_control = { type: 'ephemeral' };
    }

    if (imageTruncationThreshold) {
      maybeFilterToNMostRecentImages(messages, imageTruncationThreshold, imageTruncationThreshold);
    }

    const extraBody: ExtraBodyConfig = {};
    if (thinkingBudget) {
      extraBody.thinking = { type: 'enabled', budget_tokens: thinkingBudget };
    }

    const toolParams = toolCollection.toParams();

    const response = await client.beta.messages.create({
      max_tokens: maxTokens,
      messages,
      model,
      system: [system],
      tools: toolParams,
      betas,
      ...extraBody,
    });

    const responseParams = responseToParams(response);

    const loggableContent = responseParams.map((block) => {
      if (block.type === 'tool_use') {
        return {
          type: 'tool_use',
          name: block.name,
          input: block.input,
        };
      }
      return block;
    });
    console.log('=== LLM RESPONSE ===');
    console.log('Stop reason:', response.stop_reason);
    console.log(loggableContent);
    console.log('===');

    messages.push({
      role: 'assistant',
      content: responseParams,
    });

    if (response.stop_reason === 'end_turn') {
      console.log('LLM has completed its task, ending loop');
      return messages;
    }

    const toolResultContent = [];
    let hasToolUse = false;

    for (const contentBlock of responseParams) {
      if (
        contentBlock.type === 'tool_use' &&
        contentBlock.name &&
        contentBlock.input &&
        typeof contentBlock.input === 'object'
      ) {
        const input = contentBlock.input as ToolUseInput;
        if ('action' in input && typeof input.action === 'string') {
          hasToolUse = true;
          const toolInput: ActionParams = {
            action: input.action as Action,
            ...Object.fromEntries(Object.entries(input).filter(([key]) => key !== 'action')),
          };

          try {
            const result = await toolCollection.run(contentBlock.name, toolInput);

            const toolResult = makeApiToolResult(result, contentBlock.id!);
            toolResultContent.push(toolResult);
          } catch (error) {
            console.error(error);
            throw error;
          }
        }
      }
    }

    if (toolResultContent.length === 0 && !hasToolUse && response.stop_reason !== 'tool_use') {
      console.log('No tool use or results, and not waiting for tool use, ending loop');
      return messages;
    }

    if (toolResultContent.length > 0) {
      messages.push({
        role: 'user',
        content: toolResultContent,
      });
    }
  }
}
