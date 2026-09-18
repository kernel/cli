# EHR System Automation Template

This template demonstrates how to run an agentic browser workflow on Kernel to automate an Electronic Health Records (EHR) portal. It uses an Anthropic Computer Use loop with Kernel's Computer Controls API.

## Logic

The automation performs the following steps:
1.  Navigate to the EHR login page (`https://ehr-system-six.vercel.app/login`).
2.  Authenticate using valid credentials (any email/password works for this demo).
3.  Navigate to the **Medical Reports** section in the dashboard.
4.  Click the **Download Summary of Care** button to download the report.

## Quickstart

Deploy:

```bash
kernel deploy index.ts -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
```

Invoke:

```bash
kernel invoke ehr-system export-report
```

View logs:

```bash
kernel logs ehr-system --follow
```

## Notes

- The login page must be publicly reachable from the Kernel browser session.
- Update the URL in `pkg/templates/typescript/ehr-system/index.ts` if you host the portal elsewhere.

## Requirements

- ANTHROPIC_API_KEY environment variable set.
- Kernel CLI installed and authenticated.
