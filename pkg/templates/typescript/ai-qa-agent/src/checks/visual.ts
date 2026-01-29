/**
 * Visual QA Checks Module
 *
 * Performs visual and UI design quality checks.
 */

import { parseAIResponse } from "../helpers";
import { VISUAL_QA_PROMPT } from "../prompts";
import type { ParsedVisualIssue, QaIssue, VisionProvider } from "../types";

/**
 * Perform visual checks on a page.
 * Checks for broken UI elements and design quality issues.
 */
export async function performVisualChecks(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  const issues: QaIssue[] = [];

  console.log(`Performing visual checks on ${url}...`);

  try {
    const response = await visionProvider.analyzeScreenshot(screenshot, VISUAL_QA_PROMPT);
    console.log(`  AI response length: ${response.length} chars`);
    console.log(`  AI response preview: ${response.substring(0, 200)}...`);

    const parsed = parseAIResponse<ParsedVisualIssue>(response);
    console.log(`  Parsed ${parsed.length} issues from response`);

    for (const issue of parsed) {
      issues.push({
        severity: issue.severity || "info",
        category: "visual",
        description: issue.description,
        page: url,
        location: issue.location,
      });
    }

    console.log(`  Found ${issues.length} visual issues`);
  } catch (error) {
    console.error("Error in visual analysis:", error);
    issues.push({
      severity: "warning",
      category: "visual",
      description: `Visual analysis failed: ${error instanceof Error ? error.message : String(error)}`,
      page: url,
    });
  }

  return issues;
}
