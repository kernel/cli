import type { AgentEvent } from '@onkernel/cua-agent';

// Logs the agent's narration (`agent>`) and each browser action (`→`) it takes.
// Pass to `agent.subscribe(logAgentEvent)`.
export function logAgentEvent(event: AgentEvent): void {
  if (event.type === 'tool_execution_start') {
    console.log(`  → ${describe(event.toolName, event.args)}`);
  } else if (event.type === 'tool_execution_end' && event.isError) {
    console.log(`  ✗ ${event.toolName} failed`);
  } else if (event.type === 'message_end') {
    const text = narration(event.message);
    if (text) console.log(`\nagent> ${text}`);
  }
}

// Anthropic nests actions under `computer_batch`, OpenAI under
// `computer_use_extra`; unwrap those, then print the action name and its args.
function describe(name: string, args: any): string {
  if (name === 'computer_batch' && Array.isArray(args?.actions)) {
    return args.actions.map((a: any) => describe(a.type, a)).join('; ');
  }
  if (name === 'computer_use_extra' && typeof args?.action === 'string') {
    return describe(args.action, args);
  }
  const params = Object.entries(args ?? {})
    .filter(([key]) => key !== 'type' && key !== 'action')
    .map(([key, value]) => `${key}=${short(value)}`)
    .join(' ');
  return params ? `${name} ${params}` : name;
}

function narration(message: any): string {
  if (message?.role !== 'assistant') return '';
  return message.content
    .filter((block: any) => block.type === 'text')
    .map((block: any) => block.text)
    .join(' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function short(value: unknown): string {
  const text = typeof value === 'string' ? value : JSON.stringify(value);
  return text.length > 80 ? `${text.slice(0, 77)}...` : text;
}
