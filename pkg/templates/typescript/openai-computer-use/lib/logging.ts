import type { AgentEvent } from '@onkernel/cua-agent';

// Logs each computer-use tool call as CuaAgent executes it. Pass to
// `agent.subscribe(logAgentEvent)` to see every action (click, type,
// screenshot, ...) as it happens instead of only the final answer.
export function logAgentEvent(event: AgentEvent): void {
  if (event.type === 'tool_execution_start') {
    console.log(`[tool:start] ${event.toolName} args=${formatJson(event.args)}`);
    return;
  }
  if (event.type === 'tool_execution_end') {
    // Log the structured `details` rather than the full result: `result.content`
    // is what's sent back to the model and includes screenshot image bytes.
    const details = (event.result as { details?: unknown } | undefined)?.details;
    console.log(`[tool:end] ${event.toolName} error=${event.isError} details=${formatJson(details)}`);
  }
}

function formatJson(value: unknown): string {
  const text = JSON.stringify(value) ?? 'undefined';
  return text.length > 500 ? `${text.slice(0, 497)}...` : text;
}
