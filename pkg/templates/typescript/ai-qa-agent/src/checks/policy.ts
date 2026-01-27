/**
 * Policy Violation Detection Module
 *
 * Detects content policy and security policy violations.
 */

import type { Page } from "playwright-core";
import { parseAIResponse, scrollAndLoadImages } from "../helpers";
import { createContentPolicyPrompt, SECURITY_PROMPT } from "../prompts";
import type { IssueSeverity, ParsedPolicyViolation, PolicyChecks, QaIssue, VisionProvider } from "../types";

/**
 * Convert risk level to issue severity
 */
function riskLevelToSeverity(riskLevel?: string): IssueSeverity {
  switch (riskLevel) {
    case "high":
      return "critical";
    case "medium":
      return "warning";
    default:
      return "info";
  }
}

/**
 * Detect policy violations on a page.
 */
export async function detectPolicyViolations(
  page: Page,
  url: string,
  visionProvider: VisionProvider,
  checks: PolicyChecks,
  customPolicies?: string
): Promise<QaIssue[]> {
  const violations: QaIssue[] = [];

  // Scroll through page to load all lazy-loaded images
  await scrollAndLoadImages(page);

  const screenshot = await page.screenshot({ fullPage: true });

  console.log("Detecting policy violations...");

  // Content Policy Violations
  if (checks.content) {
    const contentViolations = await checkContentPolicy(screenshot, url, visionProvider, customPolicies);
    violations.push(...contentViolations);
  }

  // Security Issues
  if (checks.security) {
    const securityIssues = await checkSecurityPolicy(screenshot, url, visionProvider);
    violations.push(...securityIssues);
  }

  return violations;
}

/**
 * Check for content policy violations.
 */
async function checkContentPolicy(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider,
  customPolicies?: string
): Promise<QaIssue[]> {
  console.log("  Checking content policy...");

  try {
    const prompt = createContentPolicyPrompt(customPolicies);
    const response = await visionProvider.analyzeScreenshot(screenshot, prompt);
    const parsed = parseAIResponse<ParsedPolicyViolation>(response);

    const issues = parsed.map((violation) => ({
      severity: riskLevelToSeverity(violation.riskLevel),
      category: "policy" as const,
      violationType: "content" as const,
      riskLevel: violation.riskLevel as "high" | "medium" | "low" | undefined,
      description: violation.description,
      page: url,
      location: violation.location,
    }));

    console.log(`    Found ${issues.length} content policy violations`);
    return issues;
  } catch (error) {
    console.error("    Error in content policy analysis:", error);
    return [];
  }
}

/**
 * Check for security policy issues.
 */
async function checkSecurityPolicy(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  console.log("  Checking security issues...");

  try {
    const response = await visionProvider.analyzeScreenshot(screenshot, SECURITY_PROMPT);
    const parsed = parseAIResponse<ParsedPolicyViolation>(response);

    const issues = parsed.map((issue) => ({
      severity: riskLevelToSeverity(issue.riskLevel),
      category: "policy" as const,
      violationType: "security" as const,
      riskLevel: issue.riskLevel as "high" | "medium" | "low" | undefined,
      description: issue.description,
      page: url,
      location: issue.location,
    }));

    console.log(`    Found ${issues.length} security issues`);
    return issues;
  } catch (error) {
    console.error("    Error in security analysis:", error);
    return [];
  }
}
