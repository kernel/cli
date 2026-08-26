/**
 * Yutori n2 Shell and File Tools
 *
 * n2's tool set always ships `bash`, `read`, `write`, and `edit` alongside
 * `computer_batch` — `disable_tools` is rejected by the API, so a loop has to
 * answer these too. They run against the Kernel browser VM via the process and
 * filesystem APIs.
 *
 * @see https://docs.yutori.com/reference/n2
 */

import { Buffer } from 'buffer';
import type { Kernel } from '@onkernel/sdk';
import { ToolError } from './computer';

const DEFAULT_TIMEOUT_SEC = 120;
const MAX_TIMEOUT_SEC = 600;
const DEFAULT_READ_LIMIT = 2000;
const MAX_WRITE_CHARS = 256_000;

// bash: "The working directory persists across calls; environment variables and
// shell functions do not." Each exec is its own process, so the command reports
// its final directory on a sentinel line that we strip before returning stdout.
const CWD_SENTINEL = '__n2_cwd__';

interface BashArgs {
  command?: string;
  timeout?: number;
  run_in_background?: boolean;
}

interface ReadArgs {
  file_path?: string;
  offset?: number;
  limit?: number;
}

interface WriteArgs {
  file_path?: string;
  content?: string;
}

interface EditArgs {
  file_path?: string;
  old_string?: string;
  new_string?: string;
  replace_all?: boolean;
}

export class SystemTools {
  private kernel: Kernel;
  private sessionId: string;
  private cwd: string | undefined;
  private seenPaths = new Set<string>();

  constructor(kernel: Kernel, sessionId: string) {
    this.kernel = kernel;
    this.sessionId = sessionId;
  }

  async execute(name: string, args: Record<string, unknown>): Promise<string> {
    switch (name) {
      case 'bash':
        return this.bash(args as BashArgs);
      case 'read':
        return this.read(args as ReadArgs);
      case 'write':
        return this.write(args as WriteArgs);
      case 'edit':
        return this.edit(args as EditArgs);
      default:
        throw new ToolError(`Unknown tool: ${name}`);
    }
  }

  private async bash(args: BashArgs): Promise<string> {
    const command = args.command;
    if (!command) {
      throw new ToolError('command is required for bash');
    }

    const timeoutSec = Math.min(args.timeout ?? DEFAULT_TIMEOUT_SEC, MAX_TIMEOUT_SEC);

    if (args.run_in_background) {
      return this.bashBackground(command);
    }

    const script = [
      command,
      'n2_status=$?',
      `printf '\\n${CWD_SENTINEL}%s' "$(pwd)"`,
      'exit $n2_status',
    ].join('\n');

    const result = await this.kernel.browsers.process.exec(this.sessionId, {
      command: 'bash',
      args: ['-lc', script],
      ...(this.cwd ? { cwd: this.cwd } : {}),
      timeout_sec: timeoutSec,
    });

    const stdout = this.takeCwd(decode(result.stdout_b64));
    const stderr = decode(result.stderr_b64);

    return formatCommandResult(stdout, stderr, result.exit_code);
  }

  private async read(args: ReadArgs): Promise<string> {
    const filePath = args.file_path;
    if (!filePath) {
      throw new ToolError('file_path is required for read');
    }

    const response = await this.kernel.browsers.fs.readFile(this.sessionId, { path: filePath });
    const lines = (await response.text()).split('\n');

    const offset = Math.max(0, args.offset ?? 0);
    const limit = Math.max(1, args.limit ?? DEFAULT_READ_LIMIT);
    const page = lines.slice(offset, offset + limit);

    this.seenPaths.add(filePath);

    if (page.length === 0) {
      return `${filePath} has ${lines.length} line(s); offset ${offset} is past the end.`;
    }

    // cat -n format, so line numbers survive into the model's next edit.
    return page.map((line, i) => `${String(offset + i + 1).padStart(6)}\t${line}`).join('\n');
  }

  private async write(args: WriteArgs): Promise<string> {
    const filePath = args.file_path;
    const content = args.content;
    if (!filePath || content === undefined) {
      throw new ToolError('file_path and content are required for write');
    }
    if (content.length > MAX_WRITE_CHARS) {
      throw new ToolError(`content exceeds the ${MAX_WRITE_CHARS} character cap`);
    }

    await this.kernel.browsers.fs.writeFile(this.sessionId, content, { path: filePath });
    this.seenPaths.add(filePath);

    return `Wrote ${content.length} character(s) to ${filePath}.`;
  }

  private async edit(args: EditArgs): Promise<string> {
    const { file_path: filePath, old_string: oldString, new_string: newString } = args;
    if (!filePath || oldString === undefined || newString === undefined) {
      throw new ToolError('file_path, old_string, and new_string are required for edit');
    }
    // n2 is expected to know the current bytes before changing them.
    if (!this.seenPaths.has(filePath)) {
      throw new ToolError(`${filePath} has not been read in this session — read it before editing.`);
    }

    const response = await this.kernel.browsers.fs.readFile(this.sessionId, { path: filePath });
    const original = await response.text();

    const occurrences = original.split(oldString).length - 1;
    if (occurrences === 0) {
      throw new ToolError(`old_string not found in ${filePath}`);
    }
    if (occurrences > 1 && !args.replace_all) {
      throw new ToolError(
        `old_string matches ${occurrences} times in ${filePath} — pass replace_all or include more context.`,
      );
    }

    const updated = args.replace_all
      ? original.split(oldString).join(newString)
      : original.replace(oldString, newString);

    await this.kernel.browsers.fs.writeFile(this.sessionId, updated, { path: filePath });

    return `Replaced ${args.replace_all ? occurrences : 1} occurrence(s) in ${filePath}.`;
  }

  private async bashBackground(command: string): Promise<string> {
    const logPath = `/tmp/n2-bg-${Date.now()}.log`;
    const script = `nohup bash -c ${shellQuote(command)} > ${logPath} 2>&1 &\necho $!`;

    const result = await this.kernel.browsers.process.exec(this.sessionId, {
      command: 'bash',
      args: ['-lc', script],
      ...(this.cwd ? { cwd: this.cwd } : {}),
    });

    const pid = decode(result.stdout_b64).trim();

    return [
      `Started in the background with pid ${pid}.`,
      `Output is being written to ${logPath} — use the read tool to check on it.`,
      `Cancel it with: kill ${pid}`,
    ].join('\n');
  }

  /** Strip the trailing sentinel line and remember the directory it reported. */
  private takeCwd(stdout: string): string {
    const marker = stdout.lastIndexOf(`\n${CWD_SENTINEL}`);
    if (marker === -1) {
      return stdout;
    }

    this.cwd = stdout.slice(marker + CWD_SENTINEL.length + 1).trim() || this.cwd;
    return stdout.slice(0, marker);
  }
}

function decode(base64?: string): string {
  return base64 ? Buffer.from(base64, 'base64').toString('utf-8') : '';
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function formatCommandResult(stdout: string, stderr: string, exitCode?: number): string {
  const parts: string[] = [];
  if (stdout.trim()) parts.push(stdout.trimEnd());
  if (stderr.trim()) parts.push(`stderr:\n${stderr.trimEnd()}`);
  if (exitCode) parts.push(`Exited with code ${exitCode}.`);

  return parts.length > 0 ? parts.join('\n') : 'Command produced no output.';
}
