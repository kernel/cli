/**
 * Type definitions for the QA Agent
 */

// ============================================================================
// Input/Output Types
// ============================================================================

export interface QaTaskInput {
  url: string;
  model?: ModelType;
  dismissPopups?: boolean;
  checks?: QaChecks;
  context?: QaContext;
}

export interface QaTaskOutput {
  success: boolean;
  summary: QaSummary;
  issues: QaIssue[];
  jsonReport: string;
  htmlReport: string;
}

// ============================================================================
// Configuration Types
// ============================================================================

export type ModelType = "claude" | "gpt4o" | "gemini";

export interface QaChecks {
  compliance?: ComplianceChecks;
  policyViolations?: PolicyChecks;
  brokenUI?: boolean;
}

export interface ComplianceChecks {
  accessibility?: boolean;
  legal?: boolean;
  brand?: boolean;
  regulatory?: boolean;
}

export interface PolicyChecks {
  content?: boolean;
  security?: boolean;
}

export interface QaContext {
  industry?: string;
  brandGuidelines?: string;
  customPolicies?: string;
}

// ============================================================================
// Issue Types
// ============================================================================

export type IssueSeverity = "critical" | "warning" | "info";
export type IssueCategory = "visual" | "functional" | "accessibility" | "compliance" | "policy";
export type ComplianceType = "accessibility" | "legal" | "brand" | "regulatory";
export type ViolationType = "content" | "security";
export type RiskLevel = "high" | "medium" | "low";

export interface QaIssue {
  severity: IssueSeverity;
  category: IssueCategory;
  description: string;
  page: string;
  location?: string;
  screenshot?: string;
  complianceType?: ComplianceType;
  standard?: string;
  recommendation?: string;
  violationType?: ViolationType;
  riskLevel?: RiskLevel;
}

export interface QaSummary {
  totalIssues: number;
  criticalIssues: number;
  warnings: number;
  infos: number;
}

// ============================================================================
// Report Types
// ============================================================================

export interface ReportMetadata {
  url: string;
  model: string;
  timestamp: Date;
}

// ============================================================================
// Vision Provider Types
// ============================================================================

export interface VisionProvider {
  name: string;
  analyzeScreenshot(screenshot: Buffer, prompt: string): Promise<string>;
}

// ============================================================================
// Parsed AI Response Types
// ============================================================================

export interface ParsedComplianceIssue {
  severity?: IssueSeverity;
  standard?: string;
  description: string;
  location?: string;
  recommendation?: string;
}

export interface ParsedPolicyViolation {
  riskLevel?: RiskLevel;
  description: string;
  location?: string;
}

export interface ParsedVisualIssue {
  severity?: IssueSeverity;
  description: string;
  location?: string;
}

export interface ParsedFunctionalIssue {
  severity?: IssueSeverity;
  description: string;
  location?: string;
}
