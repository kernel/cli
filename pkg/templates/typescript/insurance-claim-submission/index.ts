import { Kernel, type KernelContext } from '@onkernel/sdk';
import { samplingLoop } from './loop';
import { KernelBrowserSession } from './session';

const kernel = new Kernel();

const app = kernel.app('insurance-claim-submission');

interface RecallSearchInput {
  username?: string;
  password?: string;
  record_replay?: boolean;
}

interface RecallSearchOutput {
  result: string;
  replay_url?: string;
}

// LLM API Keys are set in the environment during `kernel deploy <filename> -e ANTHROPIC_API_KEY=XXX`
// See https://www.kernel.sh/docs/launch/deploy#environment-variables
const ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;

if (!ANTHROPIC_API_KEY) {
  throw new Error('ANTHROPIC_API_KEY is not set');
}

// Default credentials for OpenEMR demo portal
const DEFAULT_USERNAME = process.env.OPENEMR_USERNAME || 'clinician';
const DEFAULT_PASSWORD = process.env.OPENEMR_PASSWORD || 'clinician';

app.action<RecallSearchInput, RecallSearchOutput>(
  'search-recall',
  async (ctx: KernelContext, payload?: RecallSearchInput): Promise<RecallSearchOutput> => {
    const username = payload?.username || DEFAULT_USERNAME;
    const password = payload?.password || DEFAULT_PASSWORD;

    // Build the task query for Claude
    const taskQuery = `
Your task is to login to the OpenEMR admin portal and search for a patient recall.

Follow these steps:

STEP 1 - LOGIN:
1. Navigate to https://demo.openemr.io/openemr/interface/login/login.php?site=default
2. Wait for the page to load completely
3. Find the username input field and enter: ${username}
4. Find the password input field and enter: ${password}
5. Click the "Login" button to submit the form
6. Wait for the login to complete and verify you are logged in successfully

STEP 2 - NAVIGATE TO RECALLS:
7. After login, you should see a navigation bar at the top with items like: Calendar, Finder, Flow, Recalls, Messages, Patient, Fees, etc.
8. Click on "Recalls" in the navigation bar
9. Wait for the Recall Board page to load

STEP 3 - CREATE NEW RECALL:
10. On the Recall Board page, click on the "+ New Recall" button (it's a blue button)
11. Wait for the New Recall form/dialog to appear

STEP 4 - SEARCH FOR PATIENT:
12. Click on the "Name" field which has the id "new_recall_name" - this should open a search popup/dialog
13. In the search popup, find the "Search by:" dropdown with id "searchby" and click on it
14. Select "ID" option from the dropdown
15. Find the input field with id "searchparm" (next to the dropdown, labeled "for:")
16. Type "1" in that input field
17. Click the "Search" button to search for the patient
18. Wait for search results to appear in the table below

STEP 5 - SELECT PATIENT:
19. In the search results table, click on the first patient row to select them
20. The search popup should close and the patient information should populate the form

STEP 6 - CONFIGURE RECALL:
21. Find the "Recall When" section with radio buttons (plus 1 year, plus 2 years, plus 3 years)
22. Click on the "plus 1 year" radio button to select it
23. Find the "Recall Reason" text field/input
24. Type "test reasons" in the Recall Reason field

STEP 7 - SCROLL:
25. Scroll down to the end of the screen to see the submit button

STEP 8 - SUBMIT RECALL:
26. Click on the "Add Recall" button to submit the recall
27. Wait for confirmation that the recall was added successfully

Important:
- Take a screenshot after each major step to verify the action was successful
- If you encounter any popups or dialogs, handle them appropriately
- If any step fails, report the error message shown
- Be patient with page loads - wait for elements to be visible before interacting
- Use element IDs when available for more reliable interactions
`;

    // Create browser session with optional replay recording
    const session = new KernelBrowserSession(kernel, {
      stealth: true,
      recordReplay: payload?.record_replay ?? false,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    try {
      // Run the sampling loop with Claude
      const finalMessages = await samplingLoop({
        model: 'claude-sonnet-4-5-20250929',
        messages: [{
          role: 'user',
          content: taskQuery,
        }],
        apiKey: ANTHROPIC_API_KEY,
        thinkingBudget: 1024,
        kernel,
        sessionId: session.sessionId,
      });

      // Extract the final result from the messages
      if (finalMessages.length === 0) {
        throw new Error('No messages were generated during the sampling loop');
      }

      const lastMessage = finalMessages[finalMessages.length - 1];
      if (!lastMessage) {
        throw new Error('Failed to get the last message from the sampling loop');
      }

      const result = typeof lastMessage.content === 'string'
        ? lastMessage.content
        : lastMessage.content.map(block =>
          block.type === 'text' ? block.text : ''
        ).join('');

      // Stop session and get replay URL if recording was enabled
      const sessionInfo = await session.stop();

      return {
        result,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      console.error('Error in sampling loop:', error);
      await session.stop();
      throw error;
    }
  },
);
