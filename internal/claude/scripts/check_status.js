// Check the status of the Claude extension.
// This script is executed via Kernel's Playwright API.
// The side panel should already be open (via OpenSidePanel click).
//
// Output:
//   - Returns JSON with extension status information

const EXTENSION_ID = 'fcoeoabgfenejglbffodgkkbkcdhcgfn';

const status = {
  extensionLoaded: false,
  authenticated: false,
  error: null,
  hasConversation: false,
};

try {
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
    status.error = 'Side panel not found. Extension may not be loaded or pinned.';
    return status;
  }

  status.extensionLoaded = true;

  // Wait for the UI to initialize
  await sidepanel.waitForLoadState('networkidle');
  await sidepanel.waitForTimeout(1000);

  // Check for authentication indicators
  // Look for chat input (indicates authenticated) - Claude uses ProseMirror contenteditable
  const chatInput = await sidepanel.$('[contenteditable="true"].ProseMirror, textarea');
  if (chatInput) {
    status.authenticated = true;
  }

  // Look for login/sign-in elements (indicates not authenticated)
  const loginButton = await sidepanel.$('button:has-text("Sign in"), button:has-text("Log in"), a:has-text("Sign in")');
  if (loginButton) {
    status.authenticated = false;
  }

  // Check for any error messages
  const errorElement = await sidepanel.$('[class*="error"], [class*="Error"], [role="alert"]');
  if (errorElement) {
    const errorText = await errorElement.textContent();
    status.error = errorText?.trim() || 'Unknown error';
  }

  // Check if there are existing messages (conversation in progress)
  const responses = await sidepanel.$$('div.claude-response');
  status.hasConversation = responses.length > 0;

} catch (e) {
  status.error = e.message;
}

return status;
