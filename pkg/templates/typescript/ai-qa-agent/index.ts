/**
 * QA Agent - AI-Powered Website Quality Assurance
 *
 * This agent performs comprehensive QA analysis on websites including:
 * - Compliance checks (accessibility, legal, brand, regulatory)
 * - Policy violation detection (content, security)
 * - Visual/UI quality checks
 *
 * Supports multiple AI vision providers: Claude, GPT-4o, and Gemini.
 */

import { Kernel, type KernelContext } from "@onkernel/sdk";
import { config } from "dotenv";
import { runQaTask } from "./src/ai-qa-runner";
import type { ModelType, QaTaskInput, QaTaskOutput } from "./src/types";

// Load environment variables from .env file
config();

// ============================================================================
// Kernel App Setup
// ============================================================================

const kernel = new Kernel();
const app = kernel.app("ts-qa-agent");

// ============================================================================
// Kernel Action Registration
// ============================================================================

app.action<QaTaskInput, QaTaskOutput>(
  "qa-test",
  async (ctx: KernelContext, payload?: QaTaskInput): Promise<QaTaskOutput> => {
    if (!payload?.url) {
      throw new Error("URL is required");
    }

    return runQaTask(ctx.invocation_id, payload);
  }
);

// ============================================================================
// Exports
// ============================================================================

// Export the runner for UI server usage
export default runQaTask;

// Re-export types for external consumers
export type { ModelType, QaIssue, QaTaskInput, QaTaskOutput } from "./src/types";

// ============================================================================
// Local Execution Support
// ============================================================================

if (import.meta.url === `file://${process.argv[1]}`) {
  const testUrl = process.argv[2] || "https://cash.app";
  const testModel = (process.argv[3] as ModelType) || "claude";

  console.log("Running QA Agent locally...");

  runQaTask(undefined, {
    url: testUrl,
    model: testModel,
  })
    .then((result) => {
      console.log("\n" + "=".repeat(80));
      console.log("JSON REPORT:");
      console.log("=".repeat(80));
      console.log(result.jsonReport);

      console.log("\n" + "=".repeat(80));
      console.log("HTML REPORT:");
      console.log("=".repeat(80));
      console.log("HTML report generated (view in browser)");
      console.log("=".repeat(80));

      process.exit(0);
    })
    .catch((error) => {
      console.error("Local execution failed:", error);
      process.exit(1);
    });
}
