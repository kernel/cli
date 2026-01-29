/**
 * QA Runner - Main Orchestration Module
 *
 * Coordinates all QA checks and report generation using Anthropic Computer Use.
 */

import { Kernel } from "@onkernel/sdk";
import { KernelBrowserSession } from "../session";
import { navigateAndCaptureScreenshots } from "./ai-qa-computer-use";
import {
  detectPolicyViolations,
  performComplianceChecks,
  performVisualChecks,
} from "./checks";
import { normalizeUrl } from "./helpers";
import type { QaIssue, QaTaskInput, QaTaskOutput } from "./types";
import { createVisionProvider } from "./vision-providers";

/** Progress callback type */
export type ProgressCallback = (step: string, message: string) => void;

/** Default progress callback that logs to console */
const defaultProgressCallback: ProgressCallback = (step, msg) => {
  console.log(`[${step}] ${msg}`);
};

/**
 * Normalize input: merge with defaults so all QA attributes default to true when not passed.
 */
function normalizeInput(input: QaTaskInput): QaTaskInput {
  const checks = input.checks ?? {};
  const compliance = checks.compliance ?? {};
  const policyViolations = checks.policyViolations ?? {};

  return {
    ...input,
    dismissPopups: input.dismissPopups ?? true,
    checks: {
      compliance: {
        accessibility: compliance.accessibility ?? true,
        legal: compliance.legal ?? true,
        brand: compliance.brand ?? true,
        regulatory: compliance.regulatory ?? true,
      },
      policyViolations: {
        content: policyViolations.content ?? true,
        security: policyViolations.security ?? true,
      },
      brokenUI: checks.brokenUI ?? true,
    },
  };
}

/**
 * Run a QA analysis task on a URL.
 *
 * @param invocationId - Kernel invocation ID (optional for local execution)
 * @param input - QA task input configuration
 * @param progressCallback - Optional callback for progress updates
 * @returns QA analysis results including issues and reports
 */
export async function runQaTask(
  invocationId: string | undefined,
  input: QaTaskInput,
  progressCallback?: ProgressCallback
): Promise<QaTaskOutput> {
  const progress = progressCallback || defaultProgressCallback;
  const normalized = normalizeInput(input);
  const url = normalizeUrl(normalized.url);
  const model = normalized.model || "claude";

  logTaskConfiguration(url, model, normalized);

  // Navigation always uses Anthropic Computer Use (regardless of analysis model)
  const anthropicApiKey = process.env.ANTHROPIC_API_KEY;
  if (!anthropicApiKey) {
    throw new Error("ANTHROPIC_API_KEY is required for navigation (Computer Use)");
  }

  // Analysis model API key (for vision checks)
  const visionApiKey = getApiKeyForModel(model);
  if (!visionApiKey) {
    throw new Error(`${model.toUpperCase()}_API_KEY is required for analysis`);
  }

  // Create vision provider
  progress("model", `Using ${model.toUpperCase()} model for analysis`);
  const visionProvider = createVisionProvider(model);
  console.log(`Using ${visionProvider.name} for analysis`);

  // Create Kernel browser session
  progress("browser", "Launching browser...");
  const kernel = new Kernel();
  const session = new KernelBrowserSession(kernel, {
    stealth: true,
    recordReplay: false,
  });

  await session.start();
  console.log("Kernel browser live view url:", session.liveViewUrl);

  try {
    // Navigate and capture screenshots using Anthropic Computer Use
    progress("navigate", `Navigating to ${url}...`);
    console.log(`\nNavigating to ${url} using Anthropic Computer Use...`);

    const navResult = await navigateAndCaptureScreenshots({
      url,
      dismissPopups: normalized.dismissPopups,
      apiKey: anthropicApiKey,
      kernel,
      sessionId: session.sessionId,
    });

    if (!navResult.success) {
      throw new Error(`Failed to navigate to ${url}: ${navResult.message}`);
    }

    if (navResult.screenshots.length === 0) {
      throw new Error("No screenshots captured");
    }

    // Use the last screenshot (most complete view of the page)
    const mainScreenshot = navResult.screenshots[navResult.screenshots.length - 1];
    if (!mainScreenshot) {
      throw new Error("No screenshots captured");
    }
    console.log(`Captured ${navResult.screenshots.length} screenshots, using the final one for analysis`);

    const allIssues: QaIssue[] = [];

    // Compliance Checks
    if (normalized.checks?.compliance) {
      const checkTypes = getEnabledCheckTypes(normalized.checks.compliance);
      if (checkTypes.length > 0) {
        progress("compliance", `Running compliance checks (${checkTypes.join(", ")})...`);
      }
      const complianceIssues = await performComplianceChecks(
        mainScreenshot.buffer,
        url,
        visionProvider,
        normalized.checks.compliance,
        normalized.context
      );
      allIssues.push(...complianceIssues);
      console.log(`\nCompliance analysis: Found ${complianceIssues.length} issues`);
    }

    // Policy Violation Checks
    if (normalized.checks?.policyViolations) {
      const checkTypes = getEnabledCheckTypes(normalized.checks.policyViolations);
      if (checkTypes.length > 0) {
        progress("policy", `Detecting policy violations (${checkTypes.join(", ")})...`);
      }
      const policyViolations = await detectPolicyViolations(
        mainScreenshot.buffer,
        url,
        visionProvider,
        normalized.checks.policyViolations,
        normalized.context?.customPolicies
      );
      allIssues.push(...policyViolations);
      console.log(`\nPolicy analysis: Found ${policyViolations.length} violations`);
    }

    // Broken UI Check
    if (normalized.checks?.brokenUI) {
      progress("ui", "Checking for broken UI elements...");
      const uiIssues = await performVisualChecks(mainScreenshot.buffer, url, visionProvider);
      allIssues.push(...uiIssues);
      console.log(`\nUI analysis: Found ${uiIssues.length} UI issues`);
    }

    // Log completion summary
    progress("generating", "Generating reports...");
    logCompletionSummary(allIssues);
    progress("complete", `Analysis complete! Found ${allIssues.length} issues.`);

    return {
      success: true,
      summary: {
        totalIssues: allIssues.length,
        criticalIssues: allIssues.filter((i) => i.severity === "critical").length,
        warnings: allIssues.filter((i) => i.severity === "warning").length,
        infos: allIssues.filter((i) => i.severity === "info").length,
      },
      issues: allIssues,
    };
  } catch (error) {
    console.error("QA analysis failed:", error);
    throw error;
  } finally {
    await session.stop();
  }
}

/**
 * Get API key for the specified model
 */
function getApiKeyForModel(model: string): string | undefined {
  switch (model) {
    case "claude":
      return process.env.ANTHROPIC_API_KEY;
    case "gpt4o":
      return process.env.OPENAI_API_KEY;
    case "gemini":
      return process.env.GOOGLE_API_KEY;
    default:
      return undefined;
  }
}

/**
 * Log the task configuration at startup
 */
function logTaskConfiguration(url: string, model: string, input: QaTaskInput): void {
  console.log(`Starting QA analysis for: ${url}`);
  console.log(`Model: ${model}`);

  if (input.checks?.compliance) {
    const { accessibility, legal, brand, regulatory } = input.checks.compliance;
    console.log(
      `Compliance checks: Accessibility=${!!accessibility}, Legal=${!!legal}, Brand=${!!brand}, Regulatory=${!!regulatory}`
    );
  }

  if (input.checks?.policyViolations) {
    const { content, security } = input.checks.policyViolations;
    console.log(`Policy checks: Content=${!!content}, Security=${!!security}`);
  }

  if (input.checks?.brokenUI) {
    console.log(`UI checks: Broken UI=${!!input.checks.brokenUI}`);
  }
}

/**
 * Get list of enabled check types from a checks object
 */
function getEnabledCheckTypes(checks: object): string[] {
  return Object.entries(checks as Record<string, boolean | undefined>)
    .filter(([, enabled]) => enabled)
    .map(([type]) => type);
}

/**
 * Log completion summary
 */
function logCompletionSummary(issues: QaIssue[]): void {
  console.log("\nQA Analysis Complete!");
  console.log(`Total issues found: ${issues.length}`);
  console.log(`- Critical: ${issues.filter((i) => i.severity === "critical").length}`);
  console.log(`- Warnings: ${issues.filter((i) => i.severity === "warning").length}`);
  console.log(`- Info: ${issues.filter((i) => i.severity === "info").length}`);
}
