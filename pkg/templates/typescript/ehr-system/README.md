# EHR System Automation Template

This template demonstrates how to use **Playwright** with **OpenAI's Computer Use** capabilities on Kernel to automate an Electronic Health Records (EHR) system workflow.

## Logic

The automation performs the following steps:
1.  Navigate to the local OpenEMR login page (served from `openEMR/index.html` in this template).
2.  Authenticate using valid credentials (any email/password works for this demo).
3.  Navigate to the **Reports** section in the dashboard.
4.  Click the **Export CSV** button to download the patient report.

This template uses an agentic loop where OpenAI Vision analyzes the page and directs Playwright to interact with elements.

## Usage

1.  **Deploy the app:**

    ```bash
    kernel deploy index.ts -e OPENAI_API_KEY=$OPENAI_API_KEY
    ```

2.  **Invoke the action:**

    ```bash
    kernel invoke ehr-system export-report
    ```

3.  **View logs:**

    ```bash
    kernel logs ehr-system --follow
    ```

## Requirements

-   OPENAI_API_KEY environment variable set.
-   Kernel CLI installed and authenticated.
