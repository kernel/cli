/**
 * JSON Report Generator
 *
 * Generates structured JSON reports from QA analysis results.
 */

import type { QaIssue, ReportMetadata } from "../types";

interface JsonReport {
  metadata: {
    url: string;
    model: string;
    timestamp: string;
    generatedBy: string;
  };
  summary: {
    totalIssues: number;
    critical: number;
    warnings: number;
    info: number;
  };
  issuesByCategory: {
    visual: number;
    functional: number;
    accessibility: number;
    compliance: number;
    policy: number;
  };
  issues: Array<{
    severity: string;
    category: string;
    description: string;
    page: string;
    location?: string;
    hasScreenshot: boolean;
  }>;
}

/**
 * Generate a JSON report from QA issues.
 */
export function generateJsonReport(issues: QaIssue[], metadata: ReportMetadata): string {
  const report: JsonReport = {
    metadata: {
      url: metadata.url,
      model: metadata.model,
      timestamp: metadata.timestamp.toISOString(),
      generatedBy: "Kernel QA Agent",
    },
    summary: {
      totalIssues: issues.length,
      critical: countBySeverity(issues, "critical"),
      warnings: countBySeverity(issues, "warning"),
      info: countBySeverity(issues, "info"),
    },
    issuesByCategory: {
      visual: countByCategory(issues, "visual"),
      functional: countByCategory(issues, "functional"),
      accessibility: countByCategory(issues, "accessibility"),
      compliance: countByCategory(issues, "compliance"),
      policy: countByCategory(issues, "policy"),
    },
    issues: issues.map((issue) => ({
      severity: issue.severity,
      category: issue.category,
      description: issue.description,
      page: issue.page,
      location: issue.location,
      hasScreenshot: !!issue.screenshot,
    })),
  };

  return JSON.stringify(report, null, 2);
}

/**
 * Count issues by severity level
 */
function countBySeverity(issues: QaIssue[], severity: string): number {
  return issues.filter((i) => i.severity === severity).length;
}

/**
 * Count issues by category
 */
function countByCategory(issues: QaIssue[], category: string): number {
  return issues.filter((i) => i.category === category).length;
}
