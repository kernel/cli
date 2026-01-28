/**
 * HTML Report Generator
 *
 * Generates beautiful HTML reports from QA analysis results.
 */

import { escapeHtml } from "../helpers";
import type { QaIssue, ReportMetadata } from "../types";

/**
 * Generate an HTML report from QA issues.
 */
export function generateHtmlReport(issues: QaIssue[], metadata: ReportMetadata): string {
  const counts = {
    critical: issues.filter((i) => i.severity === "critical").length,
    warning: issues.filter((i) => i.severity === "warning").length,
    info: issues.filter((i) => i.severity === "info").length,
  };

  const issuesByCategory = {
    compliance: issues.filter((i) => i.category === "compliance"),
    policy: issues.filter((i) => i.category === "policy"),
    visual: issues.filter((i) => i.category === "visual"),
    functional: issues.filter((i) => i.category === "functional"),
    accessibility: issues.filter((i) => i.category === "accessibility"),
  };

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>QA Report - ${escapeHtml(metadata.url)}</title>
  ${getStyles()}
</head>
<body>
  <div class="container">
    ${renderHeader(metadata)}
    ${renderSummary(issues.length, counts)}
    ${issues.length === 0 ? '<div class="no-issues">✓ No issues found!</div>' : ""}
    ${renderComplianceSection(issuesByCategory.compliance)}
    ${renderPolicySection(issuesByCategory.policy)}
    ${renderVisualSection(issuesByCategory.visual)}
  </div>
</body>
</html>`;
}

/**
 * Get CSS styles for the report
 */
function getStyles(): string {
  return `<style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
      line-height: 1.6;
      color: #333;
      background: #f5f5f5;
      padding: 20px;
    }
    .container { max-width: 1200px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
    h1 { color: #2c3e50; margin-bottom: 10px; font-size: 32px; }
    .metadata { color: #7f8c8d; margin-bottom: 30px; font-size: 14px; }
    .summary {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
      gap: 20px;
      margin-bottom: 40px;
    }
    .summary-card {
      padding: 20px;
      border-radius: 8px;
      text-align: center;
      box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    }
    .summary-card.total { background: #3498db; color: white; }
    .summary-card.critical { background: #e74c3c; color: white; }
    .summary-card.warning { background: #f39c12; color: white; }
    .summary-card.info { background: #1abc9c; color: white; }
    .summary-card .number { font-size: 48px; font-weight: bold; }
    .summary-card .label { font-size: 14px; opacity: 0.9; text-transform: uppercase; letter-spacing: 1px; }
    .section { margin-bottom: 40px; }
    .section h2 {
      color: #2c3e50;
      margin-bottom: 20px;
      padding-bottom: 10px;
      border-bottom: 2px solid #ecf0f1;
      font-size: 24px;
    }
    .issue {
      background: #fff;
      border: 1px solid #ddd;
      border-left: 4px solid #3498db;
      padding: 20px;
      margin-bottom: 20px;
      border-radius: 4px;
    }
    .issue.severity-critical { border-left-color: #e74c3c; background: #fef5f5; }
    .issue.severity-warning { border-left-color: #f39c12; background: #fffbf5; }
    .issue.severity-info { border-left-color: #1abc9c; background: #f5fffe; }
    .issue-header { margin-bottom: 12px; }
    .badge {
      display: inline-block;
      padding: 4px 12px;
      border-radius: 4px;
      font-size: 12px;
      font-weight: 600;
      margin-right: 8px;
      text-transform: uppercase;
    }
    .badge-critical { background: #e74c3c; color: white; }
    .badge-warning { background: #f39c12; color: white; }
    .badge-info { background: #1abc9c; color: white; }
    .badge-category { background: #95a5a6; color: white; }
    .issue-description { font-size: 16px; margin-bottom: 10px; color: #2c3e50; }
    .issue-location, .issue-page { font-size: 14px; color: #7f8c8d; margin: 5px 0; }
    .screenshot-container { margin-top: 15px; }
    .screenshot-container summary {
      cursor: pointer;
      color: #3498db;
      font-weight: 500;
      padding: 8px;
      background: #ecf0f1;
      border-radius: 4px;
      user-select: none;
    }
    .screenshot-container summary:hover { background: #dfe6e9; }
    .screenshot-container img {
      max-width: 100%;
      margin-top: 15px;
      border: 1px solid #ddd;
      border-radius: 4px;
    }
    .no-issues {
      padding: 40px;
      text-align: center;
      color: #27ae60;
      font-size: 18px;
      background: #e8f8f5;
      border-radius: 8px;
    }
    .empty-category {
      padding: 20px;
      text-align: center;
      color: #7f8c8d;
      font-style: italic;
      background: #f9f9f9;
      border-radius: 4px;
    }
  </style>`;
}

/**
 * Render the report header
 */
function renderHeader(metadata: ReportMetadata): string {
  return `
    <h1>QA Report</h1>
    <div class="metadata">
      <p><strong>URL:</strong> ${escapeHtml(metadata.url)}</p>
      <p><strong>Model:</strong> ${escapeHtml(metadata.model)}</p>
      <p><strong>Generated:</strong> ${metadata.timestamp.toLocaleString()}</p>
      <p><strong>Powered by:</strong> Kernel QA Agent</p>
    </div>`;
}

/**
 * Render the summary cards
 */
function renderSummary(
  total: number,
  counts: { critical: number; warning: number; info: number }
): string {
  return `
    <div class="summary">
      <div class="summary-card total">
        <div class="number">${total}</div>
        <div class="label">Total Issues</div>
      </div>
      <div class="summary-card critical">
        <div class="number">${counts.critical}</div>
        <div class="label">Critical</div>
      </div>
      <div class="summary-card warning">
        <div class="number">${counts.warning}</div>
        <div class="label">Warnings</div>
      </div>
      <div class="summary-card info">
        <div class="number">${counts.info}</div>
        <div class="label">Info</div>
      </div>
    </div>`;
}

/**
 * Render a single issue
 */
function renderIssue(issue: QaIssue): string {
  return `
    <div class="issue severity-${issue.severity}">
      <div class="issue-header">
        <span class="badge badge-${issue.severity}">${issue.severity.toUpperCase()}</span>
        ${issue.riskLevel ? `<span class="badge badge-warning">RISK: ${issue.riskLevel.toUpperCase()}</span>` : ""}
        ${issue.standard ? `<span class="badge badge-info">${escapeHtml(issue.standard)}</span>` : ""}
      </div>
      <p class="issue-description">${escapeHtml(issue.description)}</p>
      ${issue.location ? `<p class="issue-location"><strong>Location:</strong> ${escapeHtml(issue.location)}</p>` : ""}
      ${issue.recommendation ? `<p class="issue-location" style="color: #27ae60;"><strong>Recommendation:</strong> ${escapeHtml(issue.recommendation)}</p>` : ""}
      ${issue.page ? `<p class="issue-page"><strong>Page:</strong> ${escapeHtml(issue.page)}</p>` : ""}
      ${issue.screenshot ? `
        <details class="screenshot-container">
          <summary>View Screenshot</summary>
          <img src="data:image/png;base64,${issue.screenshot}" alt="Issue screenshot" />
        </details>
      ` : ""}
    </div>`;
}

/**
 * Render compliance issues section
 */
function renderComplianceSection(issues: QaIssue[]): string {
  if (issues.length === 0) return "";

  const byType = {
    accessibility: issues.filter((i) => i.complianceType === "accessibility"),
    legal: issues.filter((i) => i.complianceType === "legal"),
    brand: issues.filter((i) => i.complianceType === "brand"),
    regulatory: issues.filter((i) => i.complianceType === "regulatory"),
  };

  return `
    <div class="section">
      <h2>Compliance Issues (${issues.length})</h2>
      
      ${byType.accessibility.length > 0 ? `
        <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Accessibility Compliance</h3>
        ${byType.accessibility.map(renderIssue).join("")}
      ` : ""}
      
      ${byType.legal.length > 0 ? `
        <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Legal Compliance</h3>
        ${byType.legal.map(renderIssue).join("")}
      ` : ""}
      
      ${byType.brand.length > 0 ? `
        <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Brand Guidelines</h3>
        ${byType.brand.map(renderIssue).join("")}
      ` : ""}
      
      ${byType.regulatory.length > 0 ? `
        <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Regulatory Compliance</h3>
        ${byType.regulatory.map(renderIssue).join("")}
      ` : ""}
    </div>`;
}

/**
 * Render policy violations section
 */
function renderPolicySection(issues: QaIssue[]): string {
  if (issues.length === 0) return "";

  const byType = {
    content: issues.filter((i) => i.violationType === "content"),
    security: issues.filter((i) => i.violationType === "security"),
  };

  return `
    <div class="section">
      <h2>Policy Violations (${issues.length})</h2>
      
      ${byType.content.length > 0 ? `
        <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Content Policy</h3>
        ${byType.content.map(renderIssue).join("")}
      ` : ""}
      
      ${byType.security.length > 0 ? `
        <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Security Issues</h3>
        ${byType.security.map(renderIssue).join("")}
      ` : ""}
    </div>`;
}

/**
 * Render visual/UI issues section
 */
function renderVisualSection(issues: QaIssue[]): string {
  if (issues.length === 0) return "";

  return `
    <div class="section">
      <h2>Broken UI Issues (${issues.length})</h2>
      ${issues.map(renderIssue).join("")}
    </div>`;
}
