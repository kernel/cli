import type {
  BetaContentBlockParam as AnthropicContentBlockParam,
  BetaImageBlockParam as AnthropicImageBlockParam,
  BetaMessage as AnthropicMessage,
  BetaMessageParam as AnthropicMessageParam,
  BetaTextBlockParam as AnthropicTextBlockParam,
  BetaToolResultBlockParam as AnthropicToolResultBlockParam,
} from '@anthropic-ai/sdk/resources/beta/messages/messages';

export type BetaMessageParam = AnthropicMessageParam;
export type BetaMessage = AnthropicMessage;
export type BetaContentBlock = AnthropicContentBlockParam;
export type BetaTextBlock = AnthropicTextBlockParam;
export type BetaImageBlock = AnthropicImageBlockParam;
export type BetaToolResultBlock = AnthropicToolResultBlockParam;
export type BetaLocalContentBlock = AnthropicContentBlockParam;