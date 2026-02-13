/**
 * Functional QA Checks Module
 *
 * Performs functional checks using AI vision analysis.
 * Note: JS error detection and console error monitoring are not available
 * with Computer Use - only visual analysis of interactive elements.
 */

import { parseAIResponse } from "../helpers";
import { FUNCTIONAL_QA_PROMPT } from "../prompts";
import type { ParsedFunctionalIssue, QaIssue, VisionProvider } from "../types";

/**
 * Perform functional checks on a page using screenshot analysis.
 */
export async function performFunctionalChecks(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  const issues: QaIssue[] = [];

  console.log(`Performing functional checks on ${url}...`);

  // Analyze interactive elements with AI vision
  const aiIssues = await analyzeInteractiveElements(screenshot, url, visionProvider);
  issues.push(...aiIssues);

  return issues;
}

/**
 * Analyze interactive elements using AI vision
 */
async function analyzeInteractiveElements(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  try {
    const response = await visionProvider.analyzeScreenshot(screenshot, FUNCTIONAL_QA_PROMPT);
    const parsed = parseAIResponse<ParsedFunctionalIssue>(response);

    return parsed.map((issue) => ({
      severity: issue.severity || "info",
      category: "functional" as const,
      description: issue.description,
      page: url,
      location: issue.location,
    }));
  } catch (error) {
    console.error("Error in functional analysis:", error);
    return [];
  }
}
