import { ComputerTool20241022, ComputerTool20250124 } from './computer';
import { Action } from './types/computer';
import type { ActionParams, ToolResult } from './types/computer';

export type ToolVersion = 'computer_use_20250124' | 'computer_use_20241022' | 'computer_use_20250429';

export const DEFAULT_TOOL_VERSION: ToolVersion = 'computer_use_20250429';

interface ToolGroup {
  readonly version: ToolVersion;
  readonly tools: (typeof ComputerTool20241022 | typeof ComputerTool20250124)[];
  readonly beta_flag: string;
}

export const TOOL_GROUPS: ToolGroup[] = [
  {
    version: 'computer_use_20241022',
    tools: [ComputerTool20241022],
    beta_flag: 'computer-use-2024-10-22',
  },
  {
    version: 'computer_use_20250124',
    tools: [ComputerTool20250124],
    beta_flag: 'computer-use-2025-01-24',
  },
  // 20250429 version inherits from 20250124
  {
    version: 'computer_use_20250429',
    tools: [ComputerTool20250124],
    beta_flag: 'computer-use-2025-01-24',
  },
];

export const TOOL_GROUPS_BY_VERSION: Record<ToolVersion, ToolGroup> = Object.fromEntries(
  TOOL_GROUPS.map(group => [group.version, group])
) as Record<ToolVersion, ToolGroup>;

export class ToolCollection {
  private tools: Map<string, ComputerTool20241022 | ComputerTool20250124>;

  constructor(...tools: (ComputerTool20241022 | ComputerTool20250124)[]) {
    this.tools = new Map(tools.map(tool => [tool.name, tool]));
  }

  toParams(): ActionParams[] {
    return Array.from(this.tools.values()).map(tool => tool.toParams());
  }

  async run(name: string, toolInput: { action: Action } & Record<string, ActionParams>): Promise<ToolResult> {
    const tool = this.tools.get(name);
    if (!tool) {
      throw new Error(`Tool ${name} not found`);
    }

    if (!Object.values(Action).includes(toolInput.action)) {
      throw new Error(`Invalid action ${toolInput.action} for tool ${name}`);
    }

    return await tool.call(toolInput);
  }
} 