# Kernel QA Agent

An AI-powered quality assurance agent that uses **Anthropic Computer Use** to visually navigate websites and analyze them with vision models (Claude, GPT-4o, Gemini) for compliance issues, policy violations, broken UI, and design quality.

## What it does

The QA Agent uses **Anthropic Computer Use** to visually navigate websites like a human would, then performs comprehensive analysis:

- **Visual Navigation**: Claude sees the screen and navigates, scrolls, and dismisses popups automatically
- **Compliance Checking**: Validates accessibility (WCAG/ADA), legal requirements, brand guidelines, and industry regulations
- **Policy Violation Detection**: Identifies content policy violations and security issues
- **Broken UI Analysis**: Detects visual defects and design inconsistencies
- **AI-Powered Insights**: Uses vision models to identify compliance gaps that traditional tools miss
- **Comprehensive Reports**: Generates both JSON (machine-readable) and HTML (human-readable) reports with actionable recommendations

## How It Works

This template uses the **Computer Controls API adapter** pattern:

1. **Claude navigates** to the URL using Computer Use (sees the screen, clicks, types, scrolls)
2. **Claude captures screenshots** as it explores the page
3. **Vision models analyze** the screenshots for compliance, policy, and visual issues
4. **No brittle selectors** - Claude adapts to any UI layout

## Key Features

- **Multi-Domain Compliance**: Accessibility, legal, brand, and regulatory compliance checking
- **Industry-Specific Rules**: Pre-configured compliance checks for finance, healthcare, and e-commerce
- **Policy Enforcement**: Automated detection of content and security policy violations
- **Configurable Vision Models**: Choose between Claude (Anthropic), GPT-4o (OpenAI), or Gemini (Google)
- **Risk-Based Reporting**: Issues categorized by severity and risk level
- **Actionable Recommendations**: Specific guidance on how to fix each issue
- **Rich HTML Reports**: Beautiful, interactive reports with embedded screenshots and compliance standards
- **CI/CD Integration**: Structured JSON output for automated compliance pipelines

## Input

```json
{
  "url": "https://cash.app",
  "model": "claude",  // Options: "claude", "gpt4o", "gemini" (default: "claude")
  "checks": {
    "compliance": {
      "accessibility": true,   // WCAG/ADA compliance
      "legal": true,           // Legal requirements (privacy, terms, etc.)
      "brand": false,          // Brand guidelines (requires brandGuidelines)
      "regulatory": true       // Industry-specific regulations
    },
    "policyViolations": {
      "content": true,         // Content policy violations
      "security": true         // Security issues
    },
    "brokenUI": false          // Visual/UI issues (optional)
  },
  "context": {
    "industry": "finance",     // e.g., "finance", "healthcare", "ecommerce"
    "brandGuidelines": "...",  // Brand guidelines text (optional)
    "customPolicies": "..."    // Custom policies to enforce (optional)
  }
}
```

## Output

```json
{
  "success": true,
  "summary": {
    "totalIssues": 5,
    "criticalIssues": 1,
    "warnings": 3,
    "infos": 1
  },
  "issues": [
    {
      "severity": "critical",
      "category": "functional",
      "description": "Broken images detected",
      "page": "https://example.com",
      "location": "Main hero section",
      "screenshot": "base64_encoded_screenshot"
    }
  ],
  "jsonReport": "{ ... }",
  "htmlReport": "<!DOCTYPE html>..."
}
```

## Quick Start

### Option 1: Web UI (Easiest)

1. Install dependencies:
```bash
pnpm install
```

2. Create your `.env` file (copy from `env.example` and add your API keys)

3. Start the UI server:
```bash
pnpm ui
```

4. Open http://localhost:3000 in your browser and use the visual interface!

### Option 2: Command Line

Run directly from the command line (see Local Testing section below).

### Option 3: Deploy to Kernel

Deploy and invoke remotely (see Deploy section below).

## Setup

### 1. Install Dependencies

The template will automatically install dependencies when created. If needed, run:

```bash
pnpm install
```

### 2. Configure API Keys

Create a `.env` file in your project directory (you can copy from `env.example`):

```env
# Required: Anthropic API key for Computer Use navigation
ANTHROPIC_API_KEY=your-anthropic-api-key

# Optional: For analysis (choose one or more)
OPENAI_API_KEY=your-openai-api-key        # For GPT-4o analysis
GOOGLE_API_KEY=your-google-api-key        # For Gemini analysis

# Optional: Kernel API key (if not using kernel login)
KERNEL_API_KEY=your-kernel-api-key
```

**Note**: `ANTHROPIC_API_KEY` is required for navigation (Computer Use). The analysis model can be Claude, GPT-4o, or Gemini.

### 3. Get API Keys

- **Anthropic (Claude)**: <https://console.anthropic.com/>
- **OpenAI (GPT-4o)**: <https://platform.openai.com/api-keys>
- **Google (Gemini)**: <https://aistudio.google.com/app/apikey>

## Testing

### Quick Test

1. **Local validation:**
   ```bash
   npm install
   npx tsc --noEmit
   ```

2. **Deploy and test:**
   ```bash
   kernel deploy index.ts --env-file .env
   kernel invoke ts-qa-agent qa-test \
     --payload '{"url": "https://example.com", "model": "claude", "checks": {"compliance": {"accessibility": true}}}'
   ```

3. **View logs:**
   ```bash
   kernel deploy logs <deployment_id> --follow
   ```

See [TESTING.md](./TESTING.md) for detailed testing instructions.

## Deploy

Deploy your QA agent to Kernel:

```bash
kernel login  # If you haven't already 
or
export KERNEL_API_KEY=<YOUR_API_KEY> # If you have an API key

kernel deploy index.ts --env-file .env
```

## Usage

### Financial Services Compliance (Cash App Example)

Check a financial services website for regulatory compliance:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://cash.app",
  "model": "claude",
  "checks": {
    "compliance": {
      "accessibility": true,
      "legal": true,
      "regulatory": true
    },
    "policyViolations": {
      "content": true,
      "security": true
    }
  },
  "context": {
    "industry": "finance"
  }
}'
```

### Healthcare Compliance

Check HIPAA and healthcare compliance:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://healthcare-example.com",
  "checks": {
    "compliance": {
      "accessibility": true,
      "legal": true,
      "regulatory": true
    },
    "policyViolations": {
      "security": true
    }
  },
  "context": {
    "industry": "healthcare"
  }
}'
```

### Brand Compliance Audit

Verify brand guidelines adherence:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://example.com",
  "checks": {
    "compliance": {
      "brand": true
    }
  },
  "context": {
    "brandGuidelines": "Logo must be in top-left, primary color #007bff, font family Inter, 16px minimum body text"
  }
}'
```

### Accessibility Only

Check WCAG 2.1 AA compliance:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://example.com",
  "checks": {
    "compliance": {
      "accessibility": true
    }
  }
}'
```

### Custom Policy Enforcement

Enforce custom content policies:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://example.com",
  "checks": {
    "policyViolations": {
      "content": true
    }
  },
  "context": {
    "customPolicies": "No medical claims without FDA approval. No income guarantees. All testimonials must include disclaimers."
  }
}'
```

### Web UI (Recommended)

The easiest way to use the QA agent is through the built-in web interface:

```bash
# Make sure you have your .env file configured
pnpm install  # First time only
pnpm ui
```

Then open http://localhost:3000 in your browser. You'll get a beautiful interface where you can:
- Enter the URL to test
- Select which compliance checks to run
- Choose the AI model
- Provide industry context
- View results with interactive reports
- Export HTML reports

### Command Line Testing

You can also run the agent from the command line:

```bash
# Make sure you have your .env file configured
npx tsx index.ts https://example.com claude
```

Arguments:
1. URL to test (default: https://cash.app)
2. Model to use (default: claude)

## Web UI Features

The built-in web interface (`pnpm ui`) provides:

- **Visual Form Builder**: Easy-to-use checkboxes and inputs for all options
- **Real-time Analysis**: See results immediately in the browser
- **Interactive Results**: Click to expand sections and view details
- **Export Reports**: Download full HTML reports with one click
- **Model Comparison**: Easily test different vision models
- **Industry Presets**: Quick selection for finance, healthcare, e-commerce
- **Custom Policies**: Define your own compliance rules
- **Brand Guidelines**: Input specific brand requirements

## Vision Model Comparison

| Model | Provider | Best For | Cost | Speed |
|-------|----------|----------|------|-------|
| Claude (Sonnet 3.5) | Anthropic | Most accurate compliance analysis | $$$ | Fast |
| GPT-4o | OpenAI | Good balance of quality and cost | $$ | Fast |
| Gemini (2.0 Flash) | Google | Cost-effective, high volume | $ | Very Fast |

**Recommendation**: Start with Claude for the most thorough compliance analysis, then switch to GPT-4o or Gemini for production/regular monitoring.

## Use Cases

### 1. Financial Services Compliance Monitoring

Monitor Cash App or other fintech sites for regulatory compliance:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://cash.app",
  "checks": {
    "compliance": {"accessibility": true, "legal": true, "regulatory": true},
    "policyViolations": {"security": true}
  },
  "context": {"industry": "finance"}
}'
```

**Catches**: Missing risk disclaimers, APR disclosure issues, FDIC notice problems, accessibility violations

### 2. Pre-Launch Compliance Audit

Verify compliance before launching a new product or feature:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://staging.myapp.com/new-feature",
  "checks": {
    "compliance": {"accessibility": true, "legal": true}
  }
}'
```

**Catches**: Missing legal notices, WCAG violations, required disclosures

### 3. Brand Consistency Enforcement

Ensure marketing pages follow brand guidelines:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://myapp.com/landing",
  "checks": {"compliance": {"brand": true}},
  "context": {"brandGuidelines": "Logo top-left, #007bff primary, Inter font"}
}'
```

**Catches**: Logo misuse, off-brand colors, typography violations

### 4. Healthcare HIPAA Compliance

Check healthcare websites for HIPAA compliance:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://healthcareapp.com",
  "checks": {
    "compliance": {"legal": true, "regulatory": true},
    "policyViolations": {"security": true}
  },
  "context": {"industry": "healthcare"}
}'
```

**Catches**: Missing privacy notices, exposed patient data, insecure forms

### 5. Content Moderation

Detect policy-violating content:

```bash
kernel invoke ts-qa-agent qa-test --payload '{
  "url": "https://example.com",
  "checks": {"policyViolations": {"content": true}},
  "context": {"customPolicies": "No unverified medical claims"}
}'
```

**Catches**: Misleading claims, prohibited content, policy violations

## Report Formats

### JSON Report

Machine-readable format perfect for CI/CD integration:

```json
{
  "metadata": {
    "url": "https://example.com",
    "model": "Claude (Anthropic)",
    "timestamp": "2026-01-20T10:30:00.000Z",
    "generatedBy": "Kernel QA Agent"
  },
  "summary": {
    "totalIssues": 5,
    "critical": 1,
    "warnings": 3,
    "info": 1
  },
  "issuesByCategory": {
    "visual": 3,
    "functional": 2,
    "accessibility": 0
  },
  "issues": [...]
}
```

### HTML Report

Beautiful, interactive report with:
- Executive summary dashboard
- Issues grouped by severity and category
- Embedded screenshots for visual issues
- Responsive design for viewing on any device

The HTML report is included in the response as `htmlReport` field. Save it to a file to view in your browser:

```javascript
// In your integration code
const result = await invoke("qa-test", { url: "https://example.com" });
fs.writeFileSync("qa-report.html", result.htmlReport);
```

## What Issues Does It Detect?

### Compliance Issues

#### Accessibility (WCAG 2.1 AA)
- ✓ Color contrast violations (text readability)
- ✓ Missing alt text indicators
- ✓ Form labels and ARIA attributes
- ✓ Focus indicators on interactive elements
- ✓ Heading hierarchy issues
- ✓ Font size and readability problems

#### Legal Compliance
- ✓ Missing privacy policy links
- ✓ Missing terms of service
- ✓ Cookie consent banner (GDPR)
- ✓ Required disclaimers
- ✓ Data collection notices
- ✓ Age restriction warnings
- ✓ Copyright notices

#### Brand Guidelines
- ✓ Logo usage and placement
- ✓ Color palette adherence
- ✓ Typography inconsistencies
- ✓ Spacing and layout violations
- ✓ Imagery style mismatches

#### Regulatory (Industry-Specific)
- **Finance**: Risk disclaimers, APR display, fee disclosures, FDIC notices
- **Healthcare**: HIPAA notices, privacy practices, provider credentials
- **E-commerce**: Pricing transparency, return policies, shipping costs

### Policy Violations

#### Content Policy
- ✓ Inappropriate or offensive content
- ✓ Misleading claims or false advertising
- ✓ Unverified health/medical claims
- ✓ Get-rich-quick schemes
- ✓ Age-inappropriate content
- ✓ Prohibited products/services
- ✓ Copyright infringement

#### Security Issues
- ✓ Exposed personal data
- ✓ Missing HTTPS indicators on forms
- ✓ Insecure payment displays
- ✓ Exposed API keys or tokens
- ✓ Weak password requirements
- ✓ Missing security badges
- ✓ Suspicious external links

### Broken UI (Optional)
- ✓ Layout problems
- ✓ Spacing inconsistencies
- ✓ Broken or missing images
- ✓ Design inconsistencies

## Architecture

The QA Agent uses a two-stage approach:

1. **Navigation Stage (Anthropic Computer Use)**:
   - Claude visually navigates to the URL
   - Scrolls through the page to load all content
   - Dismisses popups and modals automatically
   - Captures screenshots of the fully-loaded page

2. **Analysis Stage (Vision Models)**:
   - Screenshots are analyzed by the selected vision model (Claude/GPT-4o/Gemini)
   - Compliance, policy, and visual checks are performed
   - Issues are categorized and reported

This architecture provides:
- **Robust Navigation**: Claude adapts to any UI layout
- **Complete Coverage**: All lazy-loaded content is captured
- **Flexible Analysis**: Choose the best vision model for your needs

## Limitations

- AI accuracy depends on the chosen model and screenshot quality
- Cannot verify actual HTTPS connection (only visible indicators)
- Functional checks (JS errors, console errors) are not available with Computer Use
- Brand guideline checking requires explicit guidelines in context
- Industry regulations are based on common requirements, not exhaustive legal analysis

## Tips for Best Results

1. **Be Specific with Industry**: Provide accurate industry context for best regulatory checks
2. **Use Staging First**: Test on staging to avoid affecting production analytics
3. **Start with Claude**: Use Claude for most accurate compliance analysis
4. **Provide Brand Guidelines**: Include specific, measurable brand rules for best results
5. **Custom Policies**: Write clear, specific custom policies for content checking
6. **Review AI Findings**: AI suggestions should be reviewed by compliance experts
7. **Regular Monitoring**: Run compliance checks regularly as part of CI/CD
8. **Combine with Tools**: This complements (not replaces) traditional compliance tools

## Troubleshooting

### "API key not found" Error

Make sure your `.env` file is properly configured and you're deploying with `--env-file .env`.

### "Navigation timeout" Error

The target website may be slow or blocking automated browsers. Try:
- Increasing the timeout in the code
- Using Kernel's stealth mode (already enabled by default)
- Testing with a different URL

### AI Returns No Issues on Problematic Page

Try:
- Using a different vision model (Claude is most thorough)
- Enabling/disabling specific check types
- Checking if the page loaded correctly in the live view URL

## Example Output

```
Starting QA analysis for: https://cash.app
Model: claude
Compliance checks: Accessibility=true, Legal=true, Brand=false, Regulatory=true
Policy checks: Content=true, Security=true
Using Claude (Anthropic) for analysis
Kernel browser live view url: https://kernel.sh/view/...

Performing compliance checks...
  Checking accessibility (WCAG 2.1 AA)...
    Found 3 accessibility issues
  Checking legal compliance...
    Found 1 legal compliance issues
  Checking finance regulatory compliance...
    Found 2 regulatory compliance issues

Compliance analysis: Found 6 issues

Detecting policy violations...
  Checking content policy...
    Found 0 content policy violations
  Checking security issues...
    Found 1 security issues

Policy analysis: Found 1 violations

QA Analysis Complete!
Total issues found: 7
- Critical: 1
- Warnings: 4
- Info: 2
```

## Learn More

- [Kernel Documentation](https://kernel.sh/docs)
- [Anthropic Claude Vision](https://docs.anthropic.com/claude/docs/vision)
- [OpenAI GPT-4o Vision](https://platform.openai.com/docs/guides/vision)
- [Google Gemini Multimodal](https://ai.google.dev/gemini-api/docs/vision)

## Support

For issues or questions:
- [Kernel Discord](https://discord.gg/kernel)
- [GitHub Issues](https://github.com/kernel/cli/issues)
