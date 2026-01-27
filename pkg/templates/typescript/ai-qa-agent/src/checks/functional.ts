/**
 * Functional QA Checks Module
 *
 * Performs functional checks including JS errors, console errors,
 * broken images, and interactive element analysis.
 */

import type { Page } from "playwright-core";
import { parseAIResponse, scrollAndLoadImages } from "../helpers";
import { FUNCTIONAL_QA_PROMPT } from "../prompts";
import type { ParsedFunctionalIssue, QaIssue, VisionProvider } from "../types";

/**
 * Perform functional checks on a page.
 */
export async function performFunctionalChecks(
  page: Page,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  const issues: QaIssue[] = [];

  console.log(`Performing functional checks on ${url}...`);

  // Collect JavaScript errors
  const jsErrors: string[] = [];
  page.on("pageerror", (error) => {
    jsErrors.push(error.message);
  });

  // Collect console errors
  const consoleErrors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text());
    }
  });

  // Wait a moment to catch any immediate errors
  await page.waitForTimeout(2000);

  // Report JavaScript errors
  if (jsErrors.length > 0) {
    issues.push(createJsErrorIssue(jsErrors, url));
  }

  // Report significant console errors
  const significantConsoleErrors = filterSignificantConsoleErrors(consoleErrors);
  if (significantConsoleErrors.length > 0) {
    issues.push(createConsoleErrorIssue(significantConsoleErrors, url));
  }

  // Check for broken images
  const brokenImages = await findBrokenImages(page);
  if (brokenImages.length > 0) {
    issues.push(createBrokenImagesIssue(brokenImages, url));
  }

  // Scroll through page to load all lazy-loaded images
  await scrollAndLoadImages(page);

  // Analyze interactive elements with AI
  const aiIssues = await analyzeInteractiveElements(page, url, visionProvider);
  issues.push(...aiIssues);

  return issues;
}

/**
 * Create an issue for JavaScript errors
 */
function createJsErrorIssue(jsErrors: string[], url: string): QaIssue {
  const errorSummary = jsErrors.slice(0, 3).join("; ");
  const additionalCount = jsErrors.length > 3 ? ` (and ${jsErrors.length - 3} more)` : "";

  return {
    severity: "critical",
    category: "functional",
    description: `JavaScript errors detected: ${errorSummary}${additionalCount}`,
    page: url,
  };
}

/**
 * Filter out common non-critical console errors
 */
function filterSignificantConsoleErrors(errors: string[]): string[] {
  return errors.filter((err) => !err.includes("favicon") && !err.includes("analytics"));
}

/**
 * Create an issue for console errors
 */
function createConsoleErrorIssue(errors: string[], url: string): QaIssue {
  return {
    severity: "warning",
    category: "functional",
    description: `Console errors: ${errors.slice(0, 2).join("; ")}`,
    page: url,
  };
}

/**
 * Find broken images on the page
 */
async function findBrokenImages(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const images = Array.from(document.querySelectorAll("img"));
    return images
      .filter((img) => !img.complete || img.naturalHeight === 0)
      .map((img) => img.src || img.alt || "unknown")
      .slice(0, 5);
  });
}

/**
 * Create an issue for broken images
 */
function createBrokenImagesIssue(brokenImages: string[], url: string): QaIssue {
  return {
    severity: "critical",
    category: "functional",
    description: `Broken images detected: ${brokenImages.join(", ")}`,
    page: url,
  };
}

/**
 * Analyze interactive elements using AI vision
 */
async function analyzeInteractiveElements(
  page: Page,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  const screenshot = await page.screenshot({ fullPage: true });

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
