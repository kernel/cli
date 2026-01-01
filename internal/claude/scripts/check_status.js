// Check the status of the Claude extension.
// This script is executed via Kernel's Playwright API.
//
// Output:
//   - Returns JSON with extension status information

const EXTENSION_ID = 'fcoeoabgfenejglbffodgkkbkcdhcgfn';
const SIDEPANEL_URL = `chrome-extension://${EXTENSION_ID}/sidepanel.html?mode=window`;

const status = {
  extensionLoaded: false,
  authenticated: false,
  error: null,
  hasConversation: false,
};

try {
  // Try to open the sidepanel
  const sidepanel = await context.newPage();
  
  try {
    await sidepanel.goto(SIDEPANEL_URL, { timeout: 10000 });
    await sidepanel.waitForLoadState('networkidle', { timeout: 10000 });
    status.extensionLoaded = true;
  } catch (e) {
    status.error = 'Extension not loaded or not accessible';
    return status;
  }

  // Wait a bit for the UI to initialize
  await sidepanel.waitForTimeout(2000);

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

  // Close the test page
  await sidepanel.close();

} catch (e) {
  status.error = e.message;
}

return status;
