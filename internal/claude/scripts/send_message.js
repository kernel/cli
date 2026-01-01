// Send a message to Claude and wait for the response.
// This script is executed via Kernel's Playwright API.
// The side panel should already be open (via OpenSidePanel click).
//
// Input (via environment variables set before this script):
//   - process.env.CLAUDE_MESSAGE: The message to send
//   - process.env.CLAUDE_TIMEOUT_MS: Timeout in milliseconds (default: 120000)
//
// Output:
//   - Returns JSON with { response: string, model?: string }

const EXTENSION_ID = 'fcoeoabgfenejglbffodgkkbkcdhcgfn';

const message = process.env.CLAUDE_MESSAGE;
const timeoutMs = parseInt(process.env.CLAUDE_TIMEOUT_MS || '120000', 10);

if (!message) {
  throw new Error('CLAUDE_MESSAGE environment variable is required');
}

// Wait for the side panel to appear (it was opened by clicking the extension icon)
let sidepanel = null;
const maxWaitMs = 10000;
const startWait = Date.now();
while (Date.now() - startWait < maxWaitMs) {
  sidepanel = context.pages().find(p => p.url().includes('sidepanel.html'));
  if (sidepanel) break;
  await new Promise(r => setTimeout(r, 500));
}

if (!sidepanel) {
  throw new Error('Side panel not found. Make sure the extension is loaded and pinned.');
}

// Wait for the UI to fully initialize
await sidepanel.waitForLoadState('networkidle');
await sidepanel.waitForTimeout(1000);

// Check if we're authenticated by looking for the chat input
// Claude uses a contenteditable div with ProseMirror
const inputSelector = '[contenteditable="true"].ProseMirror, textarea';
const input = await sidepanel.waitForSelector(inputSelector, {
  timeout: 10000,
}).catch(() => null);

if (!input) {
  throw new Error('Could not find chat input. The extension may not be authenticated.');
}

// Get the current number of Claude responses before sending
const responsesBefore = await sidepanel.$$('div.claude-response');
const countBefore = responsesBefore.length;

// Clear any existing text and type the new message
await input.click();
await input.fill('');
await input.fill(message);

// Press Enter to send
await sidepanel.keyboard.press('Enter');

// Slash commands need an extra Enter to confirm
if (message.startsWith('/')) {
  await sidepanel.waitForTimeout(500);
  await sidepanel.keyboard.press('Enter');
}

// Wait for the response to appear and complete
// We detect completion by waiting for the streaming to stop
const startTime = Date.now();
let lastContent = '';
let stableCount = 0;
const STABLE_THRESHOLD = 3; // Number of checks with same content to consider complete
const CHECK_INTERVAL = 500; // Check every 500ms

while (Date.now() - startTime < timeoutMs) {
  await sidepanel.waitForTimeout(CHECK_INTERVAL);

  // Find Claude responses
  const responses = await sidepanel.$$('div.claude-response');
  
  if (responses.length > countBefore) {
    // Get the last response
    const lastResponse = responses[responses.length - 1];
    const content = await lastResponse.textContent();
    
    // Check if content has stabilized (streaming complete)
    if (content === lastContent && content.length > 0) {
      stableCount++;
      if (stableCount >= STABLE_THRESHOLD) {
        // Response is complete
        return {
          response: content.trim(),
        };
      }
    } else {
      stableCount = 0;
      lastContent = content;
    }
  }
}

// Timeout - return whatever we have
if (lastContent) {
  return {
    response: lastContent.trim(),
    warning: 'Response may be incomplete (timeout)',
  };
}

throw new Error('Timeout waiting for response');
