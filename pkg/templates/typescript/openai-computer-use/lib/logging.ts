import type { AgentEvent, OutputMode } from './log-events';

const MAX_LINE_WIDTH = 120;

function timestamp(): string {
  return new Date().toISOString().slice(11, 23);
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function asNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function truncateOneLine(text: string, max = 90): string {
  const singleLine = text.replace(/\s+/g, ' ').trim();
  return singleLine.length > max ? `${singleLine.slice(0, max - 3)}...` : singleLine;
}

class ThinkingSpinner {
  private active = false;
  private timer: NodeJS.Timeout | null = null;
  private frame = 0;
  private startAt = 0;
  private startTS = '';
  private reasoning = '';

  constructor(private readonly enabled: boolean) {}

  start(): void {
    if (!this.enabled || this.active) return;
    this.active = true;
    this.frame = 0;
    this.reasoning = '';
    this.startAt = Date.now();
    this.startTS = timestamp();
    this.timer = setInterval(() => this.redraw(), 100);
  }

  addReasoning(text: string): void {
    if (!this.active) return;
    this.reasoning += text;
  }

  stop(action?: string, opts?: { elapsedSeconds?: number }): void {
    const elapsedSeconds = opts?.elapsedSeconds;
    if (!this.active) {
      if (action) {
        const elapsedPrefix =
          typeof elapsedSeconds === 'number' ? `[${elapsedSeconds.toFixed(3)}s] ` : '';
        process.stdout.write(`${timestamp()}  agent> ${elapsedPrefix}${action}\n`);
      }
      return;
    }
    this.active = false;
    if (this.timer) clearInterval(this.timer);
    this.timer = null;

    const elapsed =
      typeof elapsedSeconds === 'number'
        ? elapsedSeconds.toFixed(3)
        : ((Date.now() - this.startAt) / 1000).toFixed(3);
    if (this.reasoning.trim()) {
      const thinkingText = truncateOneLine(this.reasoning, 70);
      const suffix = action ? ` -> ${action}` : '';
      process.stdout.write(`\r\x1b[2K${this.startTS}  agent> [${elapsed}s] ${thinkingText}${suffix}\n`);
    } else if (action) {
      process.stdout.write(`\r\x1b[2K${this.startTS}  agent> [${elapsed}s] ${action}\n`);
    } else {
      process.stdout.write(`\r\x1b[2K${this.startTS}  agent> [${elapsed}s] thinking...\n`);
    }
  }

  private redraw(): void {
    if (!this.active) return;
    this.frame += 1;
    const elapsed = ((Date.now() - this.startAt) / 1000).toFixed(3);
    if (this.reasoning.trim()) {
      const prefix = `${this.startTS}  agent> [${elapsed}s] `;
      const maxReasoningLen = Math.max(20, MAX_LINE_WIDTH - prefix.length);
      const text = truncateOneLine(this.reasoning, maxReasoningLen);
      process.stdout.write(`\r\x1b[2K${prefix}${text}`);
      return;
    }
    const dots = '.'.repeat((this.frame % 3) + 1).padEnd(3, ' ');
    process.stdout.write(`\r\x1b[2K${this.startTS}  agent> [${elapsed}s] thinking${dots}`);
  }
}

export function createEventLogger(opts?: {
  output?: OutputMode;
  verbose?: boolean;
}): (event: AgentEvent) => void {
  const output = opts?.output ?? 'text';
  const verbose = opts?.verbose ?? false;

  if (output === 'jsonl') {
    return (event: AgentEvent): void => {
      process.stdout.write(`${JSON.stringify({ event: event.event, data: event.data })}\n`);
    };
  }

  let inText = false;
  const spinner = new ThinkingSpinner(process.stdout.isTTY);

  return (event: AgentEvent): void => {
    const data = event.data;
    switch (event.event) {
      case 'session_state': {
        const liveUrl = asString(data.live_view_url);
        if (liveUrl) process.stdout.write(`${timestamp()} kernel> live view: ${liveUrl}\n`);
        break;
      }
      case 'backend': {
        const op = asString(data.op);
        if (!op) break;

        if (inText) {
          process.stdout.write('\n');
          inText = false;
        }

        if (op === 'live_url') {
          const detail = asString(data.detail);
          if (detail) process.stdout.write(`${timestamp()} kernel> live view: ${detail}\n`);
          break;
        }

        if (op.endsWith('.done')) {
          const baseOp = op.slice(0, -'.done'.length);
          if (baseOp.startsWith('get_current_url') && !verbose) break;
          const detail = asString(data.detail);
          const elapsedMs = asNumber(data.elapsed_ms);
          const elapsed = elapsedMs === null ? '' : `[${(elapsedMs / 1000).toFixed(3)}s] `;
          process.stdout.write(
            `${timestamp()} kernel> ${elapsed}${baseOp}${detail ? ` ${detail}` : ''}\n`,
          );
          break;
        }

        if (verbose) process.stdout.write(`${timestamp()} kernel> ${op}\n`);
        break;
      }
      case 'prompt': {
        const text = asString(data.text);
        if (text) process.stdout.write(`${timestamp()}   user> ${text}\n`);
        break;
      }
      case 'reasoning_delta': {
        const text = asString(data.text);
        if (process.stdout.isTTY) {
          spinner.start();
          spinner.addReasoning(text);
        } else if (verbose && text) {
          process.stdout.write(`${timestamp()}  agent> thinking: ${truncateOneLine(text)}\n`);
        }
        break;
      }
      case 'text_delta': {
        spinner.stop();
        const text = asString(data.text);
        if (!text) break;
        if (!inText) {
          process.stdout.write(`${timestamp()}  agent> `);
          inText = true;
        }
        process.stdout.write(text);
        break;
      }
      case 'text_done': {
        if (inText) {
          process.stdout.write('\n');
          inText = false;
        }
        break;
      }
      case 'action': {
        const actionType = asString(data.action_type);
        const description = asString(data.description) || actionType;
        const elapsedMs = asNumber(data.elapsed_ms);
        const elapsedSeconds = elapsedMs === null ? undefined : elapsedMs / 1000;
        if (inText) {
          process.stdout.write('\n');
          inText = false;
        }
        spinner.stop(description, { elapsedSeconds });
        break;
      }
      case 'screenshot': {
        if (verbose) process.stdout.write(`${timestamp()} debug> screenshot captured\n`);
        break;
      }
      case 'turn_done':
      case 'run_complete': {
        spinner.stop();
        if (inText) {
          process.stdout.write('\n');
          inText = false;
        }
        break;
      }
      case 'error': {
        const message = asString(data.message) || 'unknown error';
        spinner.stop();
        if (inText) {
          process.stdout.write('\n');
          inText = false;
        }
        process.stderr.write(`${timestamp()} error> ${message}\n`);
        break;
      }
      default:
        break;
    }
  };
}
