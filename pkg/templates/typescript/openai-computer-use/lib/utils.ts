import * as dotenv from 'dotenv';
import OpenAI from 'openai';
import { type ResponseItem } from 'openai/resources/responses/responses';

dotenv.config({ override: true, quiet: true });

const openai = new OpenAI();

const BLOCKED_DOMAINS: readonly string[] = [
  'maliciousbook.com',
  'evilvideos.com',
  'darkwebforum.com',
  'shadytok.com',
  'suspiciouspins.com',
  'ilanbigio.com',
] as const;

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
  const maxAttempts = Number(process.env.OPENAI_RETRY_MAX_ATTEMPTS ?? '4');
  const baseDelaySeconds = Number(process.env.OPENAI_RETRY_BASE_DELAY_SECONDS ?? '0.5');

  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    try {
      const response = await openai.responses.create(params);
      return 'output' in response ? response : { output: undefined };
    } catch (err: unknown) {
      const status = getErrorStatus(err);
      const retryable = isRetryableError(err);
      const message = getErrorMessage(err);

      if (!retryable || attempt >= maxAttempts) {
        console.error(message);
        throw err;
      }

      const delayMs = baseDelaySeconds * 1000 * 2 ** (attempt - 1);
      const label = status === null ? 'OpenAI request failed' : `OpenAI server error ${status}`;
      console.warn(
        `Warning: ${label}; retrying in ${(delayMs / 1000).toFixed(1)}s (${attempt}/${maxAttempts})`,
      );
      await sleep(delayMs);
    }
  }
  throw new Error('OpenAI request failed unexpectedly');
}

function getErrorStatus(err: unknown): number | null {
  if (typeof err !== 'object' || err === null) return null;
  if (!('status' in err)) return null;
  const status = (err as { status?: unknown }).status;
  return typeof status === 'number' ? status : null;
}

function getErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return String(err);
}

function isRetryableError(err: unknown): boolean {
  const status = getErrorStatus(err);
  if (status !== null) return status >= 500;

  const msg = getErrorMessage(err).toLowerCase();
  return (
    msg.includes('fetch failed') ||
    msg.includes('network') ||
    msg.includes('econnreset') ||
    msg.includes('etimedout') ||
    msg.includes('timeout')
  );
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export function checkBlocklistedUrl(url: string): boolean {
  try {
    const host = new URL(url).hostname;
    return BLOCKED_DOMAINS.some((d) => host === d || host.endsWith(`.${d}`));
  } catch {
    return false;
  }
}

export default {
  sanitizeMessage,
  createResponse,
  checkBlocklistedUrl,
};
