// Send a message to Claude and wait for the response.
// This script is executed via Kernel's Playwright API.
//
// Input (via environment/args):
//   - CLAUDE_MESSAGE: The message to send
//   - CLAUDE_TIMEOUT_MS: Timeout in milliseconds (default: 120000)
//
// Output:
//   - Returns JSON with { response: string, model?: string }

const EXTENSION_ID = 'fcoeoabgfenejglbffodgkkbkcdhcgfn';
const SIDEPANEL_URL = `chrome-extension://${EXTENSION_ID}/sidepanel.html?mode=window`;

async function run({ context }) {
  const message = process.env.CLAUDE_MESSAGE;
  const timeoutMs = parseInt(process.env.CLAUDE_TIMEOUT_MS || '120000', 10);
  
  if (!message) {
    throw new Error('CLAUDE_MESSAGE environment variable is required');
  }

  // Find or open the sidepanel page
  let sidepanel = context.pages().find(p => p.url().includes('sidepanel.html'));
  if (!sidepanel) {
    sidepanel = await context.newPage();
    await sidepanel.goto(SIDEPANEL_URL);
    await sidepanel.waitForLoadState('networkidle');
    // Wait for the UI to fully initialize
    await sidepanel.waitForTimeout(2000);
  }

  // Check if we're authenticated by looking for the chat input
  const textarea = await sidepanel.waitForSelector('textarea, [contenteditable="true"]', {
    timeout: 10000,
  }).catch(() => null);

  if (!textarea) {
    throw new Error('Could not find chat input. The extension may not be authenticated.');
  }

  // Clear any existing text and type the new message
  await textarea.click();
  await textarea.fill('');
  await textarea.fill(message);

  // Get the current number of message elements before sending
  const messagesBefore = await sidepanel.$$('[data-testid="message"], .message, [class*="Message"]');
  const countBefore = messagesBefore.length;

  // Press Enter to send
  await sidepanel.keyboard.press('Enter');

  // Wait for the response to appear and complete
  // We detect completion by waiting for the streaming to stop
  const startTime = Date.now();
  let lastContent = '';
  let stableCount = 0;
  const STABLE_THRESHOLD = 3; // Number of checks with same content to consider complete
  const CHECK_INTERVAL = 500; // Check every 500ms

  while (Date.now() - startTime < timeoutMs) {
    await sidepanel.waitForTimeout(CHECK_INTERVAL);

    // Find the latest assistant message
    const messages = await sidepanel.$$('[data-testid="message"], .message, [class*="Message"]');
    
    if (messages.length > countBefore) {
      // Get the last message (the response)
      const lastMessage = messages[messages.length - 1];
      const content = await lastMessage.textContent();
      
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
}

module.exports = run;
