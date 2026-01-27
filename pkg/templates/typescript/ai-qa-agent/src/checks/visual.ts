/**
 * Visual QA Checks Module
 *
 * Performs visual and UI design quality checks.
 */

import type { Page } from "playwright-core";
import { parseAIResponse, scrollAndLoadImages } from "../helpers";
import { VISUAL_QA_PROMPT } from "../prompts";
import type { ParsedVisualIssue, QaIssue, VisionProvider } from "../types";

/**
 * Perform visual checks on a page.
 * Checks for broken UI elements and design quality issues.
 */
export async function performVisualChecks(
  page: Page,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  const issues: QaIssue[] = [];

  console.log(`Performing visual checks on ${url}...`);

  // Scroll through page to load all lazy-loaded images
  await scrollAndLoadImages(page);

  // Capture full page screenshot
  const screenshot = await page.screenshot({ fullPage: true });
  const screenshotBase64 = screenshot.toString("base64");

  try {
    const response = await visionProvider.analyzeScreenshot(screenshot, VISUAL_QA_PROMPT);
    const parsed = parseAIResponse<ParsedVisualIssue>(response);

    for (const issue of parsed) {
      issues.push({
        severity: issue.severity || "info",
        category: "visual",
        description: issue.description,
        page: url,
        location: issue.location,
        screenshot: screenshotBase64,
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
