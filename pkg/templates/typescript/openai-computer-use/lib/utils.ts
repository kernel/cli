import 'dotenv/config';
import sharp from 'sharp';
import OpenAI from 'openai';
import { type ResponseItem } from 'openai/resources/responses/responses';
const openai = new OpenAI();

const BLOCKED_DOMAINS: readonly string[] = [
  'maliciousbook.com',
  'evilvideos.com',
  'darkwebforum.com',
  'shadytok.com',
  'suspiciouspins.com',
  'ilanbigio.com',
] as const;

export async function calculateImageDimensions(
  base64Image: string,
): Promise<{ width: number; height: number }> {
  const buf = Buffer.from(base64Image, 'base64');
  const meta = await sharp(buf).metadata();
  return { width: meta.width ?? 0, height: meta.height ?? 0 };
}
export function sanitizeMessage(msg: ResponseItem): ResponseItem {
  const sanitizedMsg = { ...msg } as ResponseItem;
  if (
    sanitizedMsg.type === 'computer_call_output' &&
    typeof sanitizedMsg.output === 'object' &&
    sanitizedMsg.output !== null
  ) {
    sanitizedMsg.output = { ...sanitizedMsg.output };
    const output = sanitizedMsg.output as { image_url?: string };
    if (output.image_url) {
      output.image_url = '[omitted]';
    }
  }
  return sanitizedMsg;
}

export async function createResponse(
  params: OpenAI.Responses.ResponseCreateParams,
): Promise<{ output?: OpenAI.Responses.ResponseOutputItem[] }> {
  try {
    const response = await openai.responses.create(params);
    return 'output' in response ? response : { output: undefined };
  } catch (err: unknown) {
    console.error((err as Error).message);
    throw err;
  }
}

export function checkBlocklistedUrl(url: string): boolean {
  const host = new URL(url).hostname;
  return BLOCKED_DOMAINS.some((d) => host === d || host.endsWith(`.${d}`));
}

export default {
  calculateImageDimensions,
  sanitizeMessage,
  createResponse,
  checkBlocklistedUrl,
};
