import Anthropic from "@anthropic-ai/sdk";
import { GoogleGenerativeAI } from "@google/generative-ai";
import { Kernel, type KernelContext } from "@onkernel/sdk";
import { config } from "dotenv";
import OpenAI from "openai";
import { chromium, type Page } from "playwright-core";

// Load environment variables from .env file
config();

const kernel = new Kernel();
const app = kernel.app("ts-qa-agent");

// ============================================================================
// Type Definitions
// ============================================================================

interface QaTaskInput {
  url: string;
  model?: "claude" | "gpt4o" | "gemini";
  dismissPopups?: boolean;
  checks?: {
    compliance?: {
      accessibility?: boolean;
      legal?: boolean;
      brand?: boolean;
      regulatory?: boolean;
    };
    policyViolations?: {
      content?: boolean;
      security?: boolean;
    };
    brokenUI?: boolean;
  };
  context?: {
    industry?: string;
    brandGuidelines?: string;
    customPolicies?: string;
  };
}

interface QaIssue {
  severity: "critical" | "warning" | "info";
  category: "visual" | "functional" | "accessibility" | "compliance" | "policy";
  description: string;
  page: string;
  location?: string;
  screenshot?: string;
  complianceType?: "accessibility" | "legal" | "brand" | "regulatory";
  standard?: string;
  recommendation?: string;
  violationType?: "content" | "security";
  riskLevel?: "high" | "medium" | "low";
}

interface QaTaskOutput {
  success: boolean;
  summary: {
    totalIssues: number;
    criticalIssues: number;
    warnings: number;
    infos: number;
  };
  issues: QaIssue[];
  jsonReport: string;
  htmlReport: string;
}

// ============================================================================
// Vision Provider Interface & Implementations
// ============================================================================

interface VisionProvider {
  name: string;
  analyzeScreenshot(screenshot: Buffer, prompt: string): Promise<string>;
}

class ClaudeVisionProvider implements VisionProvider {
  name = "Claude (Anthropic)";
  private client: Anthropic;

  constructor(apiKey: string) {
    this.client = new Anthropic({ apiKey });
  }

  async analyzeScreenshot(screenshot: Buffer, prompt: string): Promise<string> {
    const base64Image = screenshot.toString("base64");

    const response = await this.client.messages.create({
      model: "claude-3-5-sonnet-20241022",
      max_tokens: 2048,
      messages: [
        {
          role: "user",
          content: [
            {
              type: "image",
              source: {
                type: "base64",
                media_type: "image/png",
                data: base64Image,
              },
            },
            {
              type: "text",
              text: prompt,
            },
          ],
        },
      ],
    });

    const textContent = response.content.find((block) => block.type === "text");
    return textContent && textContent.type === "text" ? textContent.text : "";
  }
}

class GPT4oVisionProvider implements VisionProvider {
  name = "GPT-4o (OpenAI)";
  private client: OpenAI;

  constructor(apiKey: string) {
    this.client = new OpenAI({ apiKey });
  }

  async analyzeScreenshot(screenshot: Buffer, prompt: string): Promise<string> {
    const base64Image = screenshot.toString("base64");

    const response = await this.client.chat.completions.create({
      model: "gpt-4o",
      max_tokens: 2048,
      messages: [
        {
          role: "user",
          content: [
            {
              type: "image_url",
              image_url: {
                url: `data:image/png;base64,${base64Image}`,
              },
            },
            {
              type: "text",
              text: prompt,
            },
          ],
        },
      ],
    });

    return response.choices[0]?.message?.content || "";
  }
}

class GeminiVisionProvider implements VisionProvider {
  name = "Gemini (Google)";
  private client: GoogleGenerativeAI;

  constructor(apiKey: string) {
    this.client = new GoogleGenerativeAI(apiKey);
  }

  async analyzeScreenshot(screenshot: Buffer, prompt: string): Promise<string> {
    const model = this.client.getGenerativeModel({ model: "gemini-2.0-flash-exp" });

    const result = await model.generateContent([
      {
        inlineData: {
          mimeType: "image/png",
          data: screenshot.toString("base64"),
        },
      },
      prompt,
    ]);

    return result.response.text();
  }
}

function createVisionProvider(model: string): VisionProvider {
  switch (model) {
    case "claude":
      const anthropicKey = process.env.ANTHROPIC_API_KEY;
      if (!anthropicKey) {
        throw new Error("ANTHROPIC_API_KEY is required for Claude model");
      }
      return new ClaudeVisionProvider(anthropicKey);

    case "gpt4o":
      const openaiKey = process.env.OPENAI_API_KEY;
      if (!openaiKey) {
        throw new Error("OPENAI_API_KEY is required for GPT-4o model");
      }
      return new GPT4oVisionProvider(openaiKey);

    case "gemini":
      const googleKey = process.env.GOOGLE_API_KEY;
      if (!googleKey) {
        throw new Error("GOOGLE_API_KEY is required for Gemini model");
      }
      return new GeminiVisionProvider(googleKey);

    default:
      throw new Error(`Unknown model: ${model}`);
  }
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Scroll through the page to trigger lazy loading of images
 */
async function scrollAndLoadImages(page: Page): Promise<void> {
  await page.evaluate(`
    new Promise((resolve) => {
      const scrollHeight = document.documentElement.scrollHeight;
      const viewportHeight = window.innerHeight;
      let currentPosition = 0;
      const scrollStep = viewportHeight;

      function scrollNext() {
        if (currentPosition < scrollHeight) {
          window.scrollTo(0, currentPosition);
          currentPosition += scrollStep;
          setTimeout(scrollNext, 300);
        } else {
          window.scrollTo(0, 0);
          setTimeout(resolve, 500);
        }
      }

      scrollNext();
    })
  `);
}

/**
 * Dismiss popups, modals, overlays, and toast notifications that may block content
 */
async function dismissPopups(page: Page): Promise<void> {
  try {
    let dismissed = false;

    // First, identify if there's actually a popup/modal/overlay visible
    const popupContainerSelectors = [
      '[role="dialog"]',
      '[role="alertdialog"]',
      '.modal',
      '.popup',
      '.overlay',
      '.cookie-banner',
      '.cookie-consent',
      '#cookie-consent',
      '.notification',
      '.toast',
      '.alert',
      '.snackbar',
      '[class*="modal" i]',
      '[class*="popup" i]',
      '[class*="overlay" i]',
      '[class*="banner" i]',
      '[class*="cookie" i]',
      '[id*="cookie" i]',
      '[id*="modal" i]',
      '[id*="popup" i]',
    ];

    let popupContainer = null;
    for (const selector of popupContainerSelectors) {
      try {
        const container = page.locator(selector).first();
        if (await container.isVisible({ timeout: 300 })) {
          popupContainer = container;
          console.log(`  Found popup container: ${selector}`);
          break;
        }
      } catch (e) {
        // Continue
      }
    }

    // If no popup container found, try ESC key and exit (don't click random buttons)
    if (!popupContainer) {
      try {
        await page.keyboard.press('Escape');
        await page.waitForTimeout(300);
        console.log('  Pressed ESC key (no visible popup container found)');
      } catch (e) {
        // ESC didn't work
      }
      return;
    }

    // Strategy 1: Try Accept/OK/I Agree buttons WITHIN the popup container only
    const acceptTexts = ['Accept all', 'Accept All', 'I agree', 'I Agree', 'Allow all', 'Allow All', 'Accept cookies', 'Accept Cookies'];

    for (const text of acceptTexts) {
      try {
        // Only look for buttons WITHIN the popup container
        const button = popupContainer.locator(`button:has-text("${text}"), a:has-text("${text}"), div[role="button"]:has-text("${text}")`).first();
        if (await button.isVisible({ timeout: 500 })) {
          console.log(`  Found accept button in popup: "${text}"`);
          await button.click();
          await page.waitForTimeout(800);
          dismissed = true;
          break;
        }
      } catch (e) {
        // Continue
      }
    }

    // Strategy 2: Try CSS selectors for accept buttons WITHIN popup container
    if (!dismissed) {
      const acceptSelectors = [
        '[aria-label*="accept" i]',
        '[aria-label*="agree" i]',
        '[aria-label*="allow" i]',
        '.accept-button',
        '#accept-cookies',
        'button[id*="accept" i]',
        'button[class*="accept" i]',
        '[data-action*="accept" i]',
      ];

      for (const selector of acceptSelectors) {
        try {
          // Only search within popup container
          const button = popupContainer.locator(selector).first();
          if (await button.isVisible({ timeout: 300 })) {
            console.log(`  Found accept via selector in popup: ${selector}`);
            await button.click();
            await page.waitForTimeout(800);
            dismissed = true;
            break;
          }
        } catch (e) {
          // Continue
        }
      }
    }

    // Strategy 3: Close buttons with text WITHIN popup container
    if (!dismissed) {
      const closeTexts = ['Close', '×', '✕', 'Dismiss', 'No thanks', 'No Thanks', 'Maybe later', 'Reject all', 'Reject All', 'Skip'];

      for (const text of closeTexts) {
        try {
          // Only look within popup container
          const element = popupContainer.locator(`button:has-text("${text}"), a:has-text("${text}"), span[role="button"]:has-text("${text}"), div[role="button"]:has-text("${text}")`).first();
          if (await element.isVisible({ timeout: 500 })) {
            console.log(`  Found close element in popup: "${text}"`);
            await element.click();
            await page.waitForTimeout(800);
            dismissed = true;
            break;
          }
        } catch (e) {
          // Continue
        }
      }
    }

    // Strategy 4: Close icons and buttons by CSS WITHIN popup container
    if (!dismissed) {
      const closeSelectors = [
        // Aria labels (accessibility)
        '[aria-label*="close" i]',
        '[aria-label*="dismiss" i]',
        '[aria-label*="remove" i]',

        // Common close button classes
        '.close-button',
        '.close-icon',
        '.modal-close',
        '.popup-close',
        '.dialog-close',
        'button.close',
        '.toast-close',
        '.notification-close',
        '.banner-close',

        // Data attributes
        '[data-dismiss="modal"]',
        '[data-dismiss="toast"]',
        '[data-dismiss="alert"]',
        '[data-action*="close" i]',
        '[data-action*="dismiss" i]',

        // Class name patterns
        'button[class*="close" i]',
        'button[class*="dismiss" i]',
        'span[class*="close" i]',
        'div[class*="close" i]',
        'a[class*="close" i]',

        // SVG close icons (often used in toasts)
        'svg[class*="close" i]',
        'button > svg',
        '[aria-label*="close" i] > svg',

        // ID patterns
        '#close-button',
        '#dismiss-button',
        'button[id*="close" i]',
        'button[id*="dismiss" i]',
      ];

      for (const selector of closeSelectors) {
        try {
          // Only search within popup container
          const element = popupContainer.locator(selector).first();
          if (await element.isVisible({ timeout: 300 })) {
            console.log(`  Found close via selector in popup: ${selector}`);
            await element.click();
            await page.waitForTimeout(800);
            dismissed = true;
            break;
          }
        } catch (e) {
          // Continue
        }
      }
    }

    // Strategy 5: Look for common toast/notification containers and dismiss them
    if (!dismissed) {
      const toastSelectors = [
        '.toast',
        '.notification',
        '.alert',
        '.snackbar',
        '[role="alert"]',
        '[role="status"]',
      ];

      for (const selector of toastSelectors) {
        try {
          const toast = page.locator(selector).first();
          if (await toast.isVisible({ timeout: 300 })) {
            // Try to find a close button within the toast
            const closeButton = toast.locator('button, [role="button"], .close, [aria-label*="close" i]').first();
            if (await closeButton.isVisible({ timeout: 300 })) {
              console.log(`  Found close button in toast: ${selector}`);
              await closeButton.click();
              await page.waitForTimeout(500);
              dismissed = true;
              break;
            }
          }
        } catch (e) {
          // Continue
        }
      }
    }

    // Strategy 6: Press ESC key (works for many modals)
    try {
      await page.keyboard.press('Escape');
      await page.waitForTimeout(300);
      console.log('  Pressed ESC key');
    } catch (e) {
      // ESC didn't work
    }

    if (dismissed) {
      console.log('  ✓ Successfully dismissed popup/toast');
    } else {
      console.log('  No popups/toasts found to dismiss');
    }

  } catch (error) {
    console.log('  Error dismissing popups:', error instanceof Error ? error.message : String(error));
  }
}

function parseAIResponse(response: string): any[] {
  try {
    const jsonMatch = response.match(/\[[\s\S]*\]/);
    if (jsonMatch) {
      return JSON.parse(jsonMatch[0]);
    }
    return [];
  } catch (error) {
    console.error("Error parsing AI response:", error);
    return [];
  }
}

function getIndustrySpecificPrompt(industry: string): string {
  const prompts: Record<string, string> = {
    finance: `You are a QA expert testing financial websites. Only report MISSING critical regulatory elements.

CRITICAL (must report):
- NO risk disclosures on investment/trading pages (stocks, crypto, loans)
- NO "Member FDIC" notice when claiming FDIC insurance
- NO APR disclosure on credit card/loan offers
- Credit products with NO terms or fee information visible
- Financial transactions over HTTP (not HTTPS)

IGNORE (standard practices):
- Risk disclosures in footer or fine print (this is normal and acceptable)
- General disclaimers (standard legal protection)
- Links to full terms (users can click through)
- Small text for disclosures (legally compliant if present)
- Modern minimal designs with collapsible sections

Major financial institutions (JPMorgan, Goldman Sachs, etc.) are generally compliant. Only flag MISSING required elements.

Severity:
- "critical": Required disclosure completely absent
- "warning": Disclosure exists but hard to find

Return JSON: [{"severity": "...", "standard": "...", "description": "...", "location": "...", "recommendation": "..."}]
If standard financial disclosures are present, return: []`,

    healthcare: `You are a QA expert testing healthcare websites. Only report MISSING critical health compliance elements.

CRITICAL (must report):
- NO HIPAA privacy notice anywhere on patient portal
- NO provider credentials/licensing info on medical advice pages
- Health data collection with NO privacy disclosure
- Telehealth services with NO security/encryption notice
- Medical advice with NO disclaimer

IGNORE (standard practices):
- Privacy policies in footer (standard location)
- Generic health disclaimers ("consult your doctor")
- Standard HIPAA notices in patient portals
- Links to full privacy practices
- Professional medical websites with standard layouts

Major healthcare providers are typically compliant. Only flag ACTUALLY MISSING elements.

Severity:
- "critical": Required HIPAA/health element missing entirely
- "warning": Element exists but not prominent

Return JSON: [{"severity": "...", "standard": "...", "description": "...", "location": "...", "recommendation": "..."}]
If standard healthcare compliance elements exist, return: []`,

    ecommerce: `You are a QA expert testing e-commerce websites. Only report MISSING critical consumer protection elements.

CRITICAL (must report):
- NO pricing shown on product pages
- Checkout with NO shipping cost disclosed before payment
- NO return/refund policy anywhere findable
- NO contact information (email, phone, address)
- Payment page over HTTP (not HTTPS)

IGNORE (standard practices):
- Return policies in footer links (standard)
- Shipping costs shown at checkout (acceptable)
- Privacy/terms in footer (normal placement)
- Contact page vs contact on every page
- Modern e-commerce checkout flows

Major retailers (Amazon, Target, etc.) set the standard. Only flag what's ACTUALLY MISSING.

Severity:
- "critical": Required consumer protection element absent
- "warning": Hard to find but exists

Return JSON: [{"severity": "...", "standard": "...", "description": "...", "recommendation": "..."}]
If standard e-commerce elements are present, return: []`,
  };

  return (
    prompts[industry.toLowerCase()] ||
    `You are a QA expert. Only report CRITICAL missing regulatory elements for this industry.
Focus on what's ACTUALLY MISSING that creates legal risk. Ignore standard industry practices.

Return JSON: [{"severity": "...", "standard": "...", "description": "...", "recommendation": "..."}]
If no clear violations, return: []`
  );
}

// ============================================================================
// Compliance Checking
// ============================================================================

async function performComplianceChecks(
  page: Page,
  url: string,
  visionProvider: VisionProvider,
  checks: {
    accessibility?: boolean;
    legal?: boolean;
    brand?: boolean;
    regulatory?: boolean;
  },
  context?: { industry?: string; brandGuidelines?: string }
): Promise<QaIssue[]> {
  const issues: QaIssue[] = [];

  // Scroll through page to load all lazy-loaded images
  await scrollAndLoadImages(page);

  const screenshot = await page.screenshot({ fullPage: true });

  console.log("Performing compliance checks...");

  // Accessibility Compliance Check
  if (checks.accessibility) {
    console.log("  Checking accessibility (WCAG 2.1 AA)...");
    const accessibilityPrompt = `You are an accessibility QA expert. Only report ACTUAL violations that would fail WCAG 2.1 AA compliance testing.

CRITICAL ISSUES ONLY (must report):
- Text contrast BELOW 4.5:1 (3:1 for 18pt+ text) - must be clearly unreadable
- Images with actual content but NO alt text visible (ignore decorative images, icons with adjacent text)
- Form inputs with NO label AND NO placeholder visible
- Interactive elements completely invisible or unreachable

IGNORE these common practices (NOT violations):
- Small font sizes if readable
- Cookie banners, privacy notices (standard UX patterns)
- Decorative images, background images, icons with text labels
- Modern design patterns like cards, hero sections
- "Learn more" or "Shop now" buttons (acceptable with context)
- Hamburger menus, dropdown menus (standard patterns)

Be VERY conservative. Only report issues that would genuinely block users with disabilities.

Severity levels:
- "critical": Completely unusable for users with disabilities (missing alt text on content images, text completely unreadable)
- "warning": Difficult but not impossible to use (borderline contrast, missing labels with placeholders)

Return JSON array: [{"severity": "...", "standard": "...", "description": "...", "location": "...", "recommendation": "..."}]
If NO CLEAR violations found, return: []`;

    try {
      const response = await visionProvider.analyzeScreenshot(screenshot, accessibilityPrompt);
      const parsed = parseAIResponse(response);

      for (const issue of parsed) {
        issues.push({
          severity: issue.severity || "info",
          category: "compliance",
          complianceType: "accessibility",
          standard: issue.standard,
          description: issue.description,
          page: url,
          location: issue.location,
          recommendation: issue.recommendation,
        });
      }

      console.log(`    Found ${parsed.length} accessibility issues`);
    } catch (error) {
      console.error("    Error in accessibility analysis:", error);
    }
  }

  // Legal Compliance Check
  if (checks.legal) {
    console.log("  Checking legal compliance...");
    const legalPrompt = `You are a legal compliance QA expert. Only report MISSING required legal elements that would create actual legal risk.

CRITICAL ISSUES (must report):
- NO privacy policy link anywhere visible (footer, header, menu)
- NO cookie consent mechanism when cookies are clearly being used
- NO terms of service for sites collecting user data or processing transactions
- Financial/health sites with NO disclaimers or regulatory notices
- E-commerce with NO refund/return policy information

IGNORE these common patterns (NOT violations):
- Small footer links (standard practice - "Privacy", "Terms", etc.)
- Cookie banners that appear after page load (common delay)
- Copyright notice in small footer text (completely normal)
- Privacy links not on homepage if in footer/menu
- Modern minimal footer designs (as long as legal links exist)
- "Learn more" links that lead to full policies

Most major company websites (Apple, Google, Amazon, etc.) are compliant even if policies aren't prominently displayed.

Only report if something is COMPLETELY MISSING or would create ACTUAL legal exposure.

Severity:
- "critical": Required element completely missing (no privacy policy at all)
- "warning": Hard to find but exists somewhere

Return JSON array: [{"severity": "...", "standard": "...", "description": "...", "recommendation": "..."}]
If legal elements exist (even if small), return: []`;

    try {
      const response = await visionProvider.analyzeScreenshot(screenshot, legalPrompt);
      const parsed = parseAIResponse(response);

      for (const issue of parsed) {
        issues.push({
          severity: issue.severity || "warning",
          category: "compliance",
          complianceType: "legal",
          standard: issue.standard,
          description: issue.description,
          page: url,
          recommendation: issue.recommendation,
        });
      }

      console.log(`    Found ${parsed.length} legal compliance issues`);
    } catch (error) {
      console.error("    Error in legal compliance analysis:", error);
    }
  }

  // Brand Guidelines Check
  if (checks.brand && context?.brandGuidelines) {
    console.log("  Checking brand guidelines compliance...");
    const brandPrompt = `You are a brand QA expert. Check if this website violates these SPECIFIC brand guidelines:

=== BRAND GUIDELINES TO ENFORCE ===
${context.brandGuidelines}
=== END GUIDELINES ===

Your job: Find any elements that VIOLATE the guidelines listed above.

Be specific:
- Cite WHICH guideline is being violated
- Describe HOW it's being violated
- Only flag clear violations of the stated rules

For each violation:
- severity: "warning" (clear violation of stated guideline) | "info" (minor deviation)
- description: WHICH guideline violated and HOW (be specific)
- location: where on the page
- recommendation: how to fix to match the guideline

Return JSON array: [{"severity": "...", "description": "...", "location": "...", "recommendation": "..."}]
If the page follows the guidelines above, return: []`;

    try {
      const response = await visionProvider.analyzeScreenshot(screenshot, brandPrompt);
      const parsed = parseAIResponse(response);

      for (const issue of parsed) {
        issues.push({
          severity: issue.severity || "info",
          category: "compliance",
          complianceType: "brand",
          description: issue.description,
          page: url,
          location: issue.location,
          recommendation: issue.recommendation,
        });
      }

      console.log(`    Found ${parsed.length} brand guideline violations`);
    } catch (error) {
      console.error("    Error in brand compliance analysis:", error);
    }
  }

  // Regulatory Compliance (Industry-Specific)
  if (checks.regulatory && context?.industry) {
    console.log(`  Checking ${context.industry} regulatory compliance...`);
    const regulatoryPrompt = getIndustrySpecificPrompt(context.industry);

    try {
      const response = await visionProvider.analyzeScreenshot(screenshot, regulatoryPrompt);
      const parsed = parseAIResponse(response);

      for (const issue of parsed) {
        issues.push({
          severity: issue.severity || "warning",
          category: "compliance",
          complianceType: "regulatory",
          standard: issue.standard,
          description: issue.description,
          page: url,
          location: issue.location,
          recommendation: issue.recommendation,
        });
      }

      console.log(`    Found ${parsed.length} regulatory compliance issues`);
    } catch (error) {
      console.error("    Error in regulatory compliance analysis:", error);
    }
  }

  return issues;
}

// ============================================================================
// Policy Violation Detection
// ============================================================================

async function detectPolicyViolations(
  page: Page,
  url: string,
  visionProvider: VisionProvider,
  checks: { content?: boolean; security?: boolean },
  customPolicies?: string
): Promise<QaIssue[]> {
  const violations: QaIssue[] = [];

  // Scroll through page to load all lazy-loaded images
  await scrollAndLoadImages(page);

  const screenshot = await page.screenshot({ fullPage: true });

  console.log("Detecting policy violations...");

  // Content Policy Violations
  if (checks.content) {
    console.log("  Checking content policy...");

    let contentPrompt;
    if (customPolicies && customPolicies.trim()) {
      // Custom policies provided - make them the PRIMARY focus
      contentPrompt = `You are a content policy enforcement expert. You must check for violations of these SPECIFIC CUSTOM POLICIES:

=== CUSTOM POLICIES (PRIMARY CHECK) ===
${customPolicies}
=== END CUSTOM POLICIES ===

Your job is to find ANY content on the page that violates the policies listed above. Be thorough and specific.

For each violation found:
- riskLevel: "high" (clear violation of stated policy) | "medium" (borderline/unclear) | "low" (minor concern)
- description: WHICH SPECIFIC POLICY is violated and HOW
- location: where on the page

Return JSON array: [{"riskLevel": "...", "description": "...", "location": "..."}]
If no violations of the CUSTOM POLICIES above, return: []`;
    } else {
      // No custom policies - use standard checks
      contentPrompt = `You are a content moderation expert. Check for common policy violations (be conservative - only flag clear issues):

Check for:
- Inappropriate or offensive content (hate speech, discriminatory language)
- Misleading claims or false advertising (unsubstantiated claims, fake testimonials)
- Unverified health/medical claims (unapproved treatments, miracle cures)
- Deceptive practices (hidden fees, fake urgency, bait-and-switch)

For each violation found:
- riskLevel: "high" (immediate action required) | "medium" (review needed) | "low" (minor concern)
- description: what violates policy and why
- location: where on the page

Return JSON array: [{"riskLevel": "...", "description": "...", "location": "..."}]
If no violations found, return: []`;
    }

    try {
      const response = await visionProvider.analyzeScreenshot(screenshot, contentPrompt);
      const parsed = parseAIResponse(response);

      for (const violation of parsed) {
        const severity =
          violation.riskLevel === "high"
            ? "critical"
            : violation.riskLevel === "medium"
              ? "warning"
              : "info";

        violations.push({
          severity,
          category: "policy",
          violationType: "content",
          riskLevel: violation.riskLevel,
          description: violation.description,
          page: url,
          location: violation.location,
        });
      }

      console.log(`    Found ${parsed.length} content policy violations`);
    } catch (error) {
      console.error("    Error in content policy analysis:", error);
    }
  }

  // Security Issues
  if (checks.security) {
    console.log("  Checking security issues...");
    const securityPrompt = `You are a security expert analyzing a website for visible security issues.

Check for:
- Exposed personal data: Email addresses, phone numbers, or personal info in plain text where it shouldn't be
- Missing HTTPS indicators: Forms collecting sensitive data without visible security indicators
- Insecure payment displays: Credit card numbers or payment info visible
- Exposed API keys or tokens: Any visible credentials, keys, or tokens in the interface
- Weak password requirements: Password fields showing very weak requirements (e.g., "password", "123456")
- Missing security badges: Checkout or payment pages without trust indicators
- Suspicious external links: Links to unverified or suspicious domains
- Data exposure: Session IDs, user IDs, or internal data visible to users
- Insecure file uploads: Upload forms without file type restrictions visible

For each security issue found:
- riskLevel: "high" (data exposure, immediate security risk) | "medium" (security concern, should be addressed) | "low" (best practice improvement)
- description: what the security issue is and the potential risk
- location: where the issue is visible on the page

Return JSON array: [{"riskLevel": "...", "description": "...", "location": "..."}]
If no issues found, return: []`;

    try {
      const response = await visionProvider.analyzeScreenshot(screenshot, securityPrompt);
      const parsed = parseAIResponse(response);

      for (const issue of parsed) {
        const severity =
          issue.riskLevel === "high"
            ? "critical"
            : issue.riskLevel === "medium"
              ? "warning"
              : "info";

        violations.push({
          severity,
          category: "policy",
          violationType: "security",
          riskLevel: issue.riskLevel,
          description: issue.description,
          page: url,
          location: issue.location,
        });
      }

      console.log(`    Found ${parsed.length} security issues`);
    } catch (error) {
      console.error("    Error in security analysis:", error);
    }
  }

  return violations;
}

// ============================================================================
// Visual QA Checks
// ============================================================================

async function performVisualChecks(
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

  // Visual Analysis Prompt
  const visualPrompt = `You are a UI/UX QA expert analyzing this website for visual and design issues. Report BOTH broken functionality AND poor design quality.

CRITICAL ISSUES (completely broken):
- Broken images (404 icons, missing images)
- Text overlapping and unreadable
- Content overflowing containers causing horizontal scroll
- Buttons or links that appear broken or non-functional
- Elements positioned incorrectly (overlapping, off-screen)
- Completely broken or chaotic layouts

WARNING ISSUES (poor design/UX):
- Excessive visual clutter or overwhelming layouts
- Very poor color choices (clashing colors, eye-straining combinations)
- Inconsistent typography (random font sizes, too many fonts)
- Poor spacing and alignment (cramped, uneven, messy)
- Confusing navigation or unclear information hierarchy
- Text readability issues (too small, poor contrast, bad line height)
- Unprofessional appearance (amateurish, outdated 1990s style)
- Too many competing visual elements
- Garish or ugly color schemes
- Layout chaos (everything everywhere, no structure)

INFO ISSUES (minor problems):
- Small inconsistencies in spacing or alignment
- Minor typography issues
- Could benefit from better visual hierarchy

Be honest and critical. If the website looks unprofessional, cluttered, or poorly designed, SAY SO. Don't hold back on ugly or amateurish designs.

Severity guidelines:
- "critical": Broken functionality or completely unusable
- "warning": Poor design quality, unprofessional appearance, bad UX
- "info": Minor polish issues

Return JSON array:
[{"severity": "...", "description": "...", "location": "..."}]

If the page is well-designed and professional, return: []
If the page is ugly or poorly designed, report the specific issues.`;

  try {
    const response = await visionProvider.analyzeScreenshot(screenshot, visualPrompt);

    // Parse the AI response
    const jsonMatch = response.match(/\[[\s\S]*\]/);
    if (jsonMatch) {
      const parsedIssues = JSON.parse(jsonMatch[0]);

      for (const issue of parsedIssues) {
        issues.push({
          severity: issue.severity || "info",
          category: "visual",
          description: issue.description,
          page: url,
          location: issue.location,
          screenshot: screenshotBase64,
        });
      }
    }
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

// ============================================================================
// Functional QA Checks
// ============================================================================

async function performFunctionalChecks(
  page: Page,
  url: string,
  visionProvider: VisionProvider
): Promise<QaIssue[]> {
  const issues: QaIssue[] = [];

  console.log(`Performing functional checks on ${url}...`);

  // Check for JavaScript errors
  const jsErrors: string[] = [];
  page.on("pageerror", (error) => {
    jsErrors.push(error.message);
  });

  // Check for console errors
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
    issues.push({
      severity: "critical",
      category: "functional",
      description: `JavaScript errors detected: ${jsErrors.slice(0, 3).join("; ")}${jsErrors.length > 3 ? ` (and ${jsErrors.length - 3} more)` : ""}`,
      page: url,
    });
  }

  // Report console errors (filter out common non-critical ones)
  const significantConsoleErrors = consoleErrors.filter(
    (err) => !err.includes("favicon") && !err.includes("analytics")
  );
  if (significantConsoleErrors.length > 0) {
    issues.push({
      severity: "warning",
      category: "functional",
      description: `Console errors: ${significantConsoleErrors.slice(0, 2).join("; ")}`,
      page: url,
    });
  }

  // Check for broken images
  const brokenImages = await page.evaluate(() => {
    const images = Array.from(document.querySelectorAll("img"));
    return images
      .filter((img) => !img.complete || img.naturalHeight === 0)
      .map((img) => img.src || img.alt || "unknown")
      .slice(0, 5);
  });

  if (brokenImages.length > 0) {
    issues.push({
      severity: "critical",
      category: "functional",
      description: `Broken images detected: ${brokenImages.join(", ")}`,
      page: url,
    });
  }

  // Scroll through page to load all lazy-loaded images
  await scrollAndLoadImages(page);

  // Analyze interactive elements with AI
  const screenshot = await page.screenshot({ fullPage: true });

  const functionalPrompt = `You are a QA engineer analyzing interactive elements on a website. Look at this screenshot and identify potential functional issues.

Check for:
1. Buttons that appear non-clickable or broken
2. Form elements that look disabled or malformed
3. Links that appear broken or improperly styled
4. Interactive elements with unclear purpose
5. Accessibility issues (missing labels, poor focus indicators)

For each issue found, provide:
- Severity: critical, warning, or info
- Description: What's wrong and why it matters
- Location: Where the element is located

Format as JSON array:
[
  {
    "severity": "critical|warning|info",
    "description": "Brief description",
    "location": "Element location"
  }
]

If no issues, return: []`;

  try {
    const response = await visionProvider.analyzeScreenshot(screenshot, functionalPrompt);

    const jsonMatch = response.match(/\[[\s\S]*\]/);
    if (jsonMatch) {
      const parsedIssues = JSON.parse(jsonMatch[0]);

      for (const issue of parsedIssues) {
        issues.push({
          severity: issue.severity || "info",
          category: "functional",
          description: issue.description,
          page: url,
          location: issue.location,
        });
      }
    }
  } catch (error) {
    console.error("Error in functional analysis:", error);
  }

  return issues;
}

// ============================================================================
// Report Generation
// ============================================================================

function generateJsonReport(issues: QaIssue[], metadata: { url: string; model: string; timestamp: Date }): string {
  return JSON.stringify(
    {
      metadata: {
        url: metadata.url,
        model: metadata.model,
        timestamp: metadata.timestamp.toISOString(),
        generatedBy: "Kernel QA Agent",
      },
      summary: {
        totalIssues: issues.length,
        critical: issues.filter((i) => i.severity === "critical").length,
        warnings: issues.filter((i) => i.severity === "warning").length,
        info: issues.filter((i) => i.severity === "info").length,
      },
      issuesByCategory: {
        visual: issues.filter((i) => i.category === "visual").length,
        functional: issues.filter((i) => i.category === "functional").length,
        accessibility: issues.filter((i) => i.category === "accessibility").length,
      },
      issues: issues.map((issue) => ({
        severity: issue.severity,
        category: issue.category,
        description: issue.description,
        page: issue.page,
        location: issue.location,
        hasScreenshot: !!issue.screenshot,
      })),
    },
    null,
    2
  );
}

function generateHtmlReport(issues: QaIssue[], metadata: { url: string; model: string; timestamp: Date }): string {
  const criticalCount = issues.filter((i) => i.severity === "critical").length;
  const warningCount = issues.filter((i) => i.severity === "warning").length;
  const infoCount = issues.filter((i) => i.severity === "info").length;

  const issuesByCategory = {
    compliance: issues.filter((i) => i.category === "compliance"),
    policy: issues.filter((i) => i.category === "policy"),
    visual: issues.filter((i) => i.category === "visual"),
    functional: issues.filter((i) => i.category === "functional"),
    accessibility: issues.filter((i) => i.category === "accessibility"),
  };

  const renderIssue = (issue: QaIssue, index: number) => `
    <div class="issue severity-${issue.severity}">
      <div class="issue-header">
        <span class="badge badge-${issue.severity}">${issue.severity.toUpperCase()}</span>
        ${issue.riskLevel ? `<span class="badge badge-warning">RISK: ${issue.riskLevel.toUpperCase()}</span>` : ''}
        ${issue.standard ? `<span class="badge badge-info">${escapeHtml(issue.standard)}</span>` : ''}
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
    </div>
  `;

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>QA Report - ${escapeHtml(metadata.url)}</title>
  <style>
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
  </style>
</head>
<body>
  <div class="container">
    <h1>QA Report</h1>
    <div class="metadata">
      <p><strong>URL:</strong> ${escapeHtml(metadata.url)}</p>
      <p><strong>Model:</strong> ${escapeHtml(metadata.model)}</p>
      <p><strong>Generated:</strong> ${metadata.timestamp.toLocaleString()}</p>
      <p><strong>Powered by:</strong> Kernel QA Agent</p>
    </div>

    <div class="summary">
      <div class="summary-card total">
        <div class="number">${issues.length}</div>
        <div class="label">Total Issues</div>
      </div>
      <div class="summary-card critical">
        <div class="number">${criticalCount}</div>
        <div class="label">Critical</div>
      </div>
      <div class="summary-card warning">
        <div class="number">${warningCount}</div>
        <div class="label">Warnings</div>
      </div>
      <div class="summary-card info">
        <div class="number">${infoCount}</div>
        <div class="label">Info</div>
      </div>
    </div>

    ${issues.length === 0 ? '<div class="no-issues">✓ No issues found!</div>' : ''}

    ${issuesByCategory.compliance.length > 0 ? `
      <div class="section">
        <h2>Compliance Issues (${issuesByCategory.compliance.length})</h2>
        
        ${issuesByCategory.compliance.filter(i => i.complianceType === 'accessibility').length > 0 ? `
          <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Accessibility Compliance</h3>
          ${issuesByCategory.compliance.filter(i => i.complianceType === 'accessibility').map(renderIssue).join('')}
        ` : ''}
        
        ${issuesByCategory.compliance.filter(i => i.complianceType === 'legal').length > 0 ? `
          <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Legal Compliance</h3>
          ${issuesByCategory.compliance.filter(i => i.complianceType === 'legal').map(renderIssue).join('')}
        ` : ''}
        
        ${issuesByCategory.compliance.filter(i => i.complianceType === 'brand').length > 0 ? `
          <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Brand Guidelines</h3>
          ${issuesByCategory.compliance.filter(i => i.complianceType === 'brand').map(renderIssue).join('')}
        ` : ''}
        
        ${issuesByCategory.compliance.filter(i => i.complianceType === 'regulatory').length > 0 ? `
          <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Regulatory Compliance</h3>
          ${issuesByCategory.compliance.filter(i => i.complianceType === 'regulatory').map(renderIssue).join('')}
        ` : ''}
      </div>
    ` : ''}

    ${issuesByCategory.policy.length > 0 ? `
      <div class="section">
        <h2>Policy Violations (${issuesByCategory.policy.length})</h2>
        
        ${issuesByCategory.policy.filter(i => i.violationType === 'content').length > 0 ? `
          <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Content Policy</h3>
          ${issuesByCategory.policy.filter(i => i.violationType === 'content').map(renderIssue).join('')}
        ` : ''}
        
        ${issuesByCategory.policy.filter(i => i.violationType === 'security').length > 0 ? `
          <h3 style="color: #2c3e50; font-size: 18px; margin: 20px 0 10px 0;">Security Issues</h3>
          ${issuesByCategory.policy.filter(i => i.violationType === 'security').map(renderIssue).join('')}
        ` : ''}
      </div>
    ` : ''}

    ${issuesByCategory.visual.length > 0 ? `
      <div class="section">
        <h2>Broken UI Issues (${issuesByCategory.visual.length})</h2>
        ${issuesByCategory.visual.map(renderIssue).join('')}
      </div>
    ` : ''}
  </div>
</body>
</html>`;
}

function escapeHtml(text: string): string {
  const map: Record<string, string> = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#039;",
  };
  return text.replace(/[&<>"']/g, (m) => map[m] || m);
}

// ============================================================================
// Main QA Task Handler
// ============================================================================

async function runQaTask(
  invocationId: string | undefined,
  input: QaTaskInput,
  progressCallback?: (step: string, message: string) => void
): Promise<QaTaskOutput> {
  const progress = progressCallback || ((step, msg) => console.log(`[${step}] ${msg}`));
  const url = input.url;
  const model = input.model || "claude";

  console.log(`Starting QA analysis for: ${url}`);
  console.log(`Model: ${model}`);

  if (input.checks?.compliance) {
    console.log(`Compliance checks: Accessibility=${!!input.checks.compliance.accessibility}, Legal=${!!input.checks.compliance.legal}, Brand=${!!input.checks.compliance.brand}, Regulatory=${!!input.checks.compliance.regulatory}`);
  }
  if (input.checks?.policyViolations) {
    console.log(`Policy checks: Content=${!!input.checks.policyViolations.content}, Security=${!!input.checks.policyViolations.security}`);
  }
  if (input.checks?.brokenUI) {
    console.log(`UI checks: Broken UI=${!!input.checks.brokenUI}`);
  }

  // Create vision provider
  progress('model', `Using ${model.toUpperCase()} model for analysis`);
  const visionProvider = createVisionProvider(model);
  console.log(`Using ${visionProvider.name} for analysis`);

  // Create Kernel browser
  progress('browser', 'Launching browser...');
  const kernelBrowser = await kernel.browsers.create({
    invocation_id: invocationId,
    stealth: true,
    viewport: {
      width: 1440,
      height: 900,
    },
  });

  console.log("Kernel browser live view url:", kernelBrowser.browser_live_view_url);

  const browser = await chromium.connectOverCDP(kernelBrowser.cdp_ws_url);

  try {
    const context = browser.contexts()[0] || (await browser.newContext());
    const page = context.pages()[0] || (await context.newPage());

    // Navigate to the target URL
    progress('navigate', `Navigating to ${url}...`);
    console.log(`\nNavigating to ${url}...`);
    await page.goto(url, { waitUntil: "load", timeout: 60000 });

    // Wait for page to settle and dynamic content to load
    await page.waitForTimeout(3000);
    console.log("Page loaded successfully");

    // Dismiss popups if enabled (before scrolling)
    if (input.dismissPopups) {
      progress('popups', 'Dismissing popups and overlays...');
      console.log('\nDismissing popups and overlays...');
      await dismissPopups(page);
    }

    // Scroll through page to load all lazy-loaded images
    progress('scroll', 'Loading all page content...');
    console.log(`\nScrolling through page to load all images...`);
    await scrollAndLoadImages(page);

    // Dismiss popups again (some appear after scroll)
    if (input.dismissPopups) {
      console.log('Dismissing any popups that appeared after scrolling...');
      await dismissPopups(page);
    }

    const allIssues: QaIssue[] = [];

    // Compliance Checks
    if (input.checks?.compliance) {
      const checkTypes = Object.entries(input.checks.compliance || {})
        .filter(([_, enabled]) => enabled)
        .map(([type]) => type);
      if (checkTypes.length > 0) {
        progress('compliance', `Running compliance checks (${checkTypes.join(', ')})...`);
      }
      const complianceIssues = await performComplianceChecks(
        page,
        url,
        visionProvider,
        input.checks.compliance,
        input.context
      );
      allIssues.push(...complianceIssues);
      console.log(`\nCompliance analysis: Found ${complianceIssues.length} issues`);
    }

    // Policy Violation Checks
    if (input.checks?.policyViolations) {
      const checkTypes = Object.entries(input.checks.policyViolations || {})
        .filter(([_, enabled]) => enabled)
        .map(([type]) => type);
      if (checkTypes.length > 0) {
        progress('policy', `Detecting policy violations (${checkTypes.join(', ')})...`);
      }
      const policyViolations = await detectPolicyViolations(
        page,
        url,
        visionProvider,
        input.checks.policyViolations,
        input.context?.customPolicies
      );
      allIssues.push(...policyViolations);
      console.log(`\nPolicy analysis: Found ${policyViolations.length} violations`);
    }

    // Broken UI Check
    if (input.checks?.brokenUI) {
      progress('ui', 'Checking for broken UI elements...');
      const uiIssues = await performVisualChecks(page, url, visionProvider);
      allIssues.push(...uiIssues);
      console.log(`\nUI analysis: Found ${uiIssues.length} UI issues`);
    }

    // Generate reports
    progress('generating', 'Generating reports...');
    const metadata = {
      url,
      model: visionProvider.name,
      timestamp: new Date(),
    };

    const jsonReport = generateJsonReport(allIssues, metadata);
    const htmlReport = generateHtmlReport(allIssues, metadata);

    progress('complete', `Analysis complete! Found ${allIssues.length} issues.`);
    console.log(`\nQA Analysis Complete!`);
    console.log(`Total issues found: ${allIssues.length}`);
    console.log(`- Critical: ${allIssues.filter((i) => i.severity === "critical").length}`);
    console.log(`- Warnings: ${allIssues.filter((i) => i.severity === "warning").length}`);
    console.log(`- Info: ${allIssues.filter((i) => i.severity === "info").length}`);

    return {
      success: true,
      summary: {
        totalIssues: allIssues.length,
        criticalIssues: allIssues.filter((i) => i.severity === "critical").length,
        warnings: allIssues.filter((i) => i.severity === "warning").length,
        infos: allIssues.filter((i) => i.severity === "info").length,
      },
      issues: allIssues,
      jsonReport,
      htmlReport,
    };
  } catch (error) {
    console.error("QA analysis failed:", error);
    throw error;
  } finally {
    await browser.close();
    await kernel.browsers.deleteByID(kernelBrowser.session_id);
  }
}

// ============================================================================
// Kernel Action Registration
// ============================================================================

app.action<QaTaskInput, QaTaskOutput>(
  "qa-test",
  async (ctx: KernelContext, payload?: QaTaskInput): Promise<QaTaskOutput> => {
    if (!payload?.url) {
      throw new Error("URL is required");
    }

    // Normalize URL
    let url = payload.url;
    if (!url.startsWith("http://") && !url.startsWith("https://")) {
      url = `https://${url}`;
    }

    // Validate URL
    try {
      new URL(url);
    } catch {
      throw new Error(`Invalid URL: ${url}`);
    }

    return runQaTask(ctx.invocation_id, { ...payload, url });
  }
);

// ============================================================================
// Export for UI Server
// ============================================================================

export default runQaTask;

// ============================================================================
// Local Execution Support
// ============================================================================

if (import.meta.url === `file://${process.argv[1]}`) {
  const testUrl = process.argv[2] || "https://cash.app";
  const testModel = (process.argv[3] as "claude" | "gpt4o" | "gemini") || "claude";

  console.log("Running QA Agent locally...");

  runQaTask(undefined, {
    url: testUrl,
    model: testModel,
  })
    .then((result) => {
      console.log("\n" + "=".repeat(80));
      console.log("JSON REPORT:");
      console.log("=".repeat(80));
      console.log(result.jsonReport);

      console.log("\n" + "=".repeat(80));
      console.log("HTML REPORT:");
      console.log("=".repeat(80));
      console.log("HTML report generated (view in browser)");
      console.log("=".repeat(80));

      process.exit(0);
    })
    .catch((error) => {
      console.error("Local execution failed:", error);
      process.exit(1);
    });
}
