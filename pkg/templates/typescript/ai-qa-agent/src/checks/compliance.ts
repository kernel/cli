/**
 * Compliance Checking Module
 *
 * Performs accessibility, legal, brand, and regulatory compliance checks.
 */

import { parseAIResponse } from "../helpers";
import {
  ACCESSIBILITY_PROMPT,
  createBrandGuidelinesPrompt,
  getIndustrySpecificPrompt,
  LEGAL_COMPLIANCE_PROMPT,
} from "../prompts";
import type {
  ComplianceChecks,
  ParsedComplianceIssue,
  QaContext,
  QaIssue,
  VisionProvider,
} from "../types";

/**
 * Perform all enabled compliance checks on a page.
 */
export async function performComplianceChecks(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider,
  checks: ComplianceChecks,
  context?: QaContext
): Promise<QaIssue[]> {
  const issues: QaIssue[] = [];

  console.log("Performing compliance checks...");

  // Run enabled checks
  if (checks.accessibility) {
    const accessibilityIssues = await checkAccessibility(screenshot, url, visionProvider);
    issues.push(...accessibilityIssues);
  }

  if (checks.legal) {
    const legalIssues = await checkLegalCompliance(screenshot, url, visionProvider);
    issues.push(...legalIssues);
  }

  if (checks.brand && context?.brandGuidelines) {
    const brandIssues = await checkBrandGuidelines(screenshot, url, visionProvider, context.brandGuidelines);
    issues.push(...brandIssues);
  }

  if (checks.regulatory && context?.industry) {
    const regulatoryIssues = await checkRegulatoryCompliance(screenshot, url, visionProvider, context.industry);
    issues.push(...regulatoryIssues);
  }

  return issues;
}

/**
 * Check for WCAG 2.1 AA accessibility compliance violations.
 */
async function checkAccessibility(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  console.log("  Checking accessibility (WCAG 2.1 AA)...");

  try {
    const response = await visionProvider.analyzeScreenshot(screenshot, ACCESSIBILITY_PROMPT);
    console.log(`    AI response length: ${response.length} chars`);
    console.log(`    AI response preview: ${response.substring(0, 200)}...`);
    
    const parsed = parseAIResponse<ParsedComplianceIssue>(response);
    console.log(`    Parsed ${parsed.length} issues from response`);

    const issues = parsed.map((issue) => ({
      severity: issue.severity || "info",
      category: "compliance" as const,
      complianceType: "accessibility" as const,
      standard: issue.standard,
      description: issue.description,
      page: url,
      location: issue.location,
      recommendation: issue.recommendation,
    }));

    console.log(`    Found ${issues.length} accessibility issues`);
    return issues;
  } catch (error) {
    console.error("    Error in accessibility analysis:", error);
    return [];
  }
}

/**
 * Check for legal compliance (privacy policy, terms, cookie consent).
 */
async function checkLegalCompliance(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  console.log("  Checking legal compliance...");

  try {
    const response = await visionProvider.analyzeScreenshot(screenshot, LEGAL_COMPLIANCE_PROMPT);
    console.log(`    AI response length: ${response.length} chars`);
    console.log(`    AI response preview: ${response.substring(0, 200)}...`);
    
    const parsed = parseAIResponse<ParsedComplianceIssue>(response);
    console.log(`    Parsed ${parsed.length} issues from response`);

    const issues = parsed.map((issue) => ({
      severity: issue.severity || "warning",
      category: "compliance" as const,
      complianceType: "legal" as const,
      standard: issue.standard,
      description: issue.description,
      page: url,
      recommendation: issue.recommendation,
    }));

    console.log(`    Found ${issues.length} legal compliance issues`);
    return issues;
  } catch (error) {
    console.error("    Error in legal compliance analysis:", error);
    return [];
  }
}

/**
 * Check for brand guidelines violations.
 */
async function checkBrandGuidelines(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider,
  brandGuidelines: string
): Promise<QaIssue[]> {
  console.log("  Checking brand guidelines compliance...");

  try {
    const prompt = createBrandGuidelinesPrompt(brandGuidelines);
    const response = await visionProvider.analyzeScreenshot(screenshot, prompt);
    const parsed = parseAIResponse<ParsedComplianceIssue>(response);

    const issues = parsed.map((issue) => ({
      severity: issue.severity || "info",
      category: "compliance" as const,
      complianceType: "brand" as const,
      description: issue.description,
      page: url,
      location: issue.location,
      recommendation: issue.recommendation,
    }));

    console.log(`    Found ${issues.length} brand guideline violations`);
    return issues;
  } catch (error) {
    console.error("    Error in brand compliance analysis:", error);
    return [];
  }
}

/**
 * Check for industry-specific regulatory compliance.
 */
async function checkRegulatoryCompliance(
  screenshot: Buffer,
  url: string,
  visionProvider: VisionProvider,
  industry: string
): Promise<QaIssue[]> {
  console.log(`  Checking ${industry} regulatory compliance...`);

  try {
    const prompt = getIndustrySpecificPrompt(industry);
    const response = await visionProvider.analyzeScreenshot(screenshot, prompt);
    const parsed = parseAIResponse<ParsedComplianceIssue>(response);

    const issues = parsed.map((issue) => ({
      severity: issue.severity || "warning",
      category: "compliance" as const,
      complianceType: "regulatory" as const,
      standard: issue.standard,
      description: issue.description,
      page: url,
      location: issue.location,
      recommendation: issue.recommendation,
    }));

    console.log(`    Found ${issues.length} regulatory compliance issues`);
    return issues;
  } catch (error) {
    console.error("    Error in regulatory compliance analysis:", error);
    return [];
  }
}
