import {
  createSdkMcpServer,
  query,
  tool,
  type Options,
} from "@anthropic-ai/claude-agent-sdk";
import { Kernel, type KernelContext } from "@onkernel/sdk";
import { exec } from "child_process";
import { promisify } from "util";
import { z } from "zod";

const execAsync = promisify(exec);

// Initialize Kernel SDK
const kernel = new Kernel();

// Create Kernel app
const app = kernel.app("ts-claude-agent-sdk");

// Ensure API key is set
const ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;
if (!ANTHROPIC_API_KEY) {
  throw new Error("ANTHROPIC_API_KEY is not set");
}

/**
 * Claude Agent SDK + Kernel Browser Automation
 *
 * This example demonstrates how to use the Claude Agent SDK with Kernel's
 * Playwright Execution API to perform browser automation tasks.
 *
 * The agent is given a single tool that executes Playwright code against
 * a Kernel browser, allowing Claude to autonomously browse the web.
 */

interface AgentInput {
  task: string;
}

interface AgentOutput {
  result: string;
  cost_usd: number;
  duration_ms: number;
}

/**
 * Core agent logic that can be called from both local CLI and Kernel app
 */
async function runAgent(
  task: string,
  invocationId?: string
): Promise<AgentOutput> {
  // Create a Kernel browser session
  console.log("Creating Kernel browser...");
  const kernelBrowser = await kernel.browsers.create({
    invocation_id: invocationId,
    stealth: true,
    timeout_seconds: 300, // 5 minutes timeout
  });

  console.log("Browser live view URL:", kernelBrowser.browser_live_view_url);
  console.log("Session ID:", kernelBrowser.session_id);

  try {
    // Create an in-process MCP server with a Playwright execution tool
    const playwrightServer = createSdkMcpServer({
      name: "kernel-playwright",
      version: "1.0.0",
      tools: [
        tool(
          "execute_playwright",
          `Execute Playwright/TypeScript code against the browser. 
           The code runs in a sandboxed environment with access to page, context, and browser objects.
           Use 'return' to return values from the script.
           Available objects:
           - page: The current page instance
           - context: The browser context
           - browser: The browser instance
           
           Example code:
           - Navigate: await page.goto('https://example.com');
           - Get title: return await page.title();
           - Click: await page.click('button');
           - Type: await page.fill('input', 'text');
           - Screenshot: return (await page.screenshot()).toString('base64');
           - Extract text: return await page.locator('h1').textContent();`,
          {
            code: z
              .string()
              .describe(
                "The Playwright/TypeScript code to execute. Must be valid TypeScript that uses page/context/browser objects."
              ),
            timeout_sec: z
              .number()
              .optional()
              .describe(
                "Timeout in seconds for the execution (default: 60, max: 300)"
              ),
          },
          async (args) => {
            console.log(
              "\n--- Executing Playwright code ---\n",
              args.code,
              "\n---\n"
            );

            try {
              const result = await kernel.browsers.playwright.execute(
                kernelBrowser.session_id,
                {
                  code: args.code,
                  timeout_sec: args.timeout_sec ?? 60,
                }
              );

              if (result.success) {
                const output =
                  result.result !== undefined
                    ? JSON.stringify(result.result, null, 2)
                    : "Code executed successfully (no return value)";

                console.log("Execution result:", output);

                return {
                  content: [
                    {
                      type: "text" as const,
                      text: output,
                    },
                  ],
                };
              } else {
                const errorMsg = `Execution failed: ${result.error || "Unknown error"}\n${result.stderr || ""}`;
                console.error("Execution error:", errorMsg);

                return {
                  content: [
                    {
                      type: "text" as const,
                      text: errorMsg,
                    },
                  ],
                  isError: true,
                };
              }
            } catch (error) {
              const errorMsg = `Failed to execute Playwright code: ${error instanceof Error ? error.message : String(error)}`;
              console.error(errorMsg);

              return {
                content: [
                  {
                    type: "text" as const,
                    text: errorMsg,
                  },
                ],
                isError: true,
              };
            }
          }
        ),
      ],
    });

    console.log("\n=== Starting Claude Agent ===");
    console.log("Task:", task);
    console.log("=============================\n");

    // Determine Claude Code executable path
    const homedir = process.env.HOME || "/root";
    const claudeCodePath = `${homedir}/.local/bin/claude`;

    // Configure agent options
    const options: Options = {
      model: "claude-opus-4-5-20251101",
      systemPrompt: `You are a browser automation assistant that can control a web browser to accomplish tasks.

You have access to a tool called "execute_playwright" that lets you run Playwright code against a real browser.

Guidelines:
1. Always start by navigating to the target URL using page.goto()
2. Wait for pages to load before interacting with elements
3. Use descriptive selectors when possible (text content, aria labels, test IDs)
4. Return the results of your queries using 'return' statements
5. If something fails, try alternative approaches

When you've completed the task, summarize what you found or accomplished.`,
      mcpServers: {
        "kernel-playwright": playwrightServer,
      },
      maxTurns: 20,
      permissionMode: "acceptEdits",
      allowedTools: ["mcp__kernel-playwright__execute_playwright"],
      pathToClaudeCodeExecutable: claudeCodePath,
    };

    // Run the agent
    let finalResult = "";
    let costUsd = 0;
    let durationMs = 0;

    for await (const message of query({
      prompt: task,
      options,
    })) {
      // Process different message types
      if (message.type === "assistant" && message.message?.content) {
        for (const block of message.message.content) {
          if (block.type === "text") {
            console.log("Claude:", block.text);
          } else if (block.type === "tool_use") {
            console.log(`\nUsing tool: ${block.name}`);
          }
        }
      } else if (message.type === "result") {
        if (message.subtype === "success") {
          finalResult = message.result;
          costUsd = message.total_cost_usd;
          durationMs = message.duration_ms;
          console.log("\n=== Agent completed successfully ===");
          console.log("Final result:", message.result);
          console.log(`Cost: $${message.total_cost_usd.toFixed(4)}`);
          console.log(`Duration: ${message.duration_ms}ms`);
        } else {
          console.error("\n=== Agent failed ===");
          const errors = (message as { errors?: string[] }).errors;
          console.error("Errors:", errors);
          throw new Error(`Agent failed: ${errors?.join(", ") || "Unknown error"}`);
        }
      }
    }

    return {
      result: finalResult,
      cost_usd: costUsd,
      duration_ms: durationMs,
    };
  } finally {
    // Clean up: Delete the browser session
    console.log("\nCleaning up browser session...");
    await kernel.browsers.deleteByID(kernelBrowser.session_id);
    console.log("Browser session deleted.");
  }
}

/**
 * Install Claude Code runtime (required for Claude Agent SDK)
 * This is called on the Kernel app VM before running the agent
 */
async function installClaudeCode(): Promise<void> {
  console.log("Installing Claude Code runtime...");

  try {
    // First ensure curl is available (try without sudo first, then with)
    console.log("Installing curl...");
    try {
      await execAsync("apt-get update && apt-get install -y curl", {
        timeout: 60000,
      });
    } catch {
      // If that fails, curl might already be available
      console.log("apt-get failed, checking if curl is already available...");
    }

    // Now install Claude Code
    console.log("Installing Claude Code...");
    const { stdout, stderr } = await execAsync(
      "curl -fsSL https://claude.ai/install.sh | bash",
      { timeout: 120000 } // 2 minute timeout
    );

    if (stdout) console.log("Claude Code install stdout:", stdout);
    if (stderr) console.log("Claude Code install stderr:", stderr);

    // Add Claude Code to PATH for this process
    const homedir = process.env.HOME || "/root";
    process.env.PATH = `${homedir}/.local/bin:${process.env.PATH}`;
    console.log("Added ~/.local/bin to PATH");

    console.log("Claude Code installed successfully");
  } catch (error) {
    const err = error as { stdout?: string; stderr?: string; message?: string };
    throw new Error(
      `Failed to install Claude Code: ${err.stderr || err.stdout || err.message}`
    );
  }
}

// ============================================================================
// Kernel App Action
// ============================================================================

/**
 * Kernel app action for browser automation with Claude Agent SDK
 *
 * Deploy and invoke via CLI:
 *   kernel login  # or: export KERNEL_API_KEY=<your_api_key>
 *   kernel deploy index.ts --env-file .env
 *   kernel invoke ts-claude-agent-sdk agent-task -p '{"task": "Go to https://news.ycombinator.com and get the top 3 stories"}'
 */
app.action<AgentInput, AgentOutput>(
  "agent-task",
  async (ctx: KernelContext, payload?: AgentInput): Promise<AgentOutput> => {
    if (!payload?.task) {
      throw new Error("task is required");
    }

    // Install Claude Code runtime on the Kernel app VM
    await installClaudeCode();

    // Run the agent
    return await runAgent(payload.task, ctx.invocation_id);
  }
);

// ============================================================================
// Local CLI Execution
// ============================================================================

/**
 * Check if running on Kernel infrastructure
 * On Kernel, the app runs from /boot-node/ directory
 */
function isRunningOnKernel(): boolean {
  return process.cwd().startsWith("/boot-node") || 
         process.argv[1]?.startsWith("/boot-node");
}

// Run locally if executed directly via CLI (not on Kernel)
if (!isRunningOnKernel()) {
  const task = process.argv[2] || 
    "Go to https://news.ycombinator.com and tell me the titles of the top 3 stories on the front page";

  runAgent(task)
    .then((result) => {
      console.log("\n=== Done ===");
      console.log("Result:", result.result);
      process.exit(0);
    })
    .catch((error) => {
      console.error("Fatal error:", error);
      process.exit(1);
    });
}
