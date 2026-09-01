import type { AgentEvent } from '@onkernel/cua-agent';

// Human-readable trace of what the agent is doing, meant for
// `agent.subscribe(logAgentEvent)`: the model's narration between steps
// (`agent>`), then each concrete browser action it takes (`→`). Failed
// actions are marked so retries are visible.
export function logAgentEvent(event: AgentEvent): void {
  switch (event.type) {
    case 'message_end': {
      const text = assistantText(event.message);
      if (text) console.log(`\nagent> ${text}`);
      return;
    }
    case 'tool_execution_start':
      console.log(`  → ${formatAction(event.toolName, event.args)}`);
      return;
    case 'tool_execution_end':
      if (event.isError) {
        console.log(`  ✗ ${event.toolName} failed`);
      }
      return;
  }
}

// Concatenate the visible text blocks of an assistant message. Non-assistant
// messages and tool-call-only turns have no narration and return ''.
function assistantText(message: unknown): string {
  const m = message as { role?: string; content?: unknown };
  if (m.role !== 'assistant' || !Array.isArray(m.content)) return '';
  return m.content
    .filter((b): b is { type: 'text'; text: string } => isTextBlock(b))
    .map((b) => b.text.trim())
    .filter(Boolean)
    .join(' ');
}

function isTextBlock(b: unknown): boolean {
  return typeof b === 'object' && b !== null && (b as { type?: string }).type === 'text';
}

// Render one tool call as a short action line. Anthropic batches actions under
// `computer_batch`; OpenAI/Gemini emit `goto`/`click`/... directly. Both flow
// through here — batch sub-actions carry their kind in `type`.
function formatAction(toolName: string, rawArgs: unknown): string {
  const a = (rawArgs ?? {}) as Record<string, any>;
  switch (toolName) {
    case 'computer_batch':
      return Array.isArray(a.actions)
        ? a.actions.map((sub: any) => formatAction(sub?.type, sub)).join('; ')
        : 'batch';
    case 'computer_use_extra':
      // OpenAI's navigation helper wraps the real action under `action`.
      return typeof a.action === 'string' ? formatAction(a.action, a) : compact(a);
    case 'screenshot':
      return 'screenshot';
    case 'goto':
      return `goto ${a.url ?? ''}`.trim();
    case 'click':
    case 'left_click':
    case 'double_click':
    case 'right_click':
      return `${toolName} ${point(a)}`;
    case 'drag':
      return `drag ${dragPath(a.path)}`;
    case 'keypress':
      return `keypress ${Array.isArray(a.keys) ? a.keys.join('+') : compact(a)}`;
    case 'type':
      return `type ${quote(a.text)}`;
    case 'scroll':
      return `scroll ${point(a)}`.trim();
    case 'wait':
      return `wait ${a.ms ?? ''}ms`;
    default:
      return `${toolName ?? 'action'} ${compact(a)}`.trim();
  }
}

function point(a: Record<string, any>): string {
  if (typeof a.x === 'number' && typeof a.y === 'number') return `(${a.x}, ${a.y})`;
  if (typeof a.x === 'number') return `(${a.x})`;
  return '';
}

function dragPath(path: unknown): string {
  if (!Array.isArray(path) || path.length === 0) return '';
  const first = path[0];
  const last = path[path.length - 1];
  return `${point(first)} → ${point(last)}`;
}

function quote(text: unknown): string {
  const s = typeof text === 'string' ? text : String(text ?? '');
  return `"${s.length > 80 ? `${s.slice(0, 77)}...` : s}"`;
}

function compact(value: unknown): string {
  const text = JSON.stringify(value) ?? '';
  return text === '{}' ? '' : text.length > 120 ? `${text.slice(0, 117)}...` : text;
}
