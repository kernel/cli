// Interactive streaming chat with Claude.
// This script is executed via Kernel's Playwright API.
//
// Communication protocol:
// - Reads JSON commands from stdin: { "type": "message", "content": "..." }
// - Writes JSON events to stdout:
//   - { "type": "ready" } - Chat is ready for input
//   - { "type": "chunk", "content": "..." } - Streaming response chunk
//   - { "type": "complete", "content": "..." } - Full response complete
//   - { "type": "error", "message": "..." } - Error occurred

const EXTENSION_ID = 'fcoeoabgfenejglbffodgkkbkcdhcgfn';
const SIDEPANEL_URL = `chrome-extension://${EXTENSION_ID}/sidepanel.html?mode=window`;

function emit(event) {
  console.log(JSON.stringify(event));
}

async function sendMessage(page, message) {
  const input = await page.$('[contenteditable="true"].ProseMirror, textarea');
  if (!input) {
    emit({ type: 'error', message: 'Chat input not found' });
    return;
  }

  // Get current response count
  const responsesBefore = await page.$$('div.claude-response');
  const countBefore = responsesBefore.length;

  // Send the message
  await input.click();
  await input.fill('');
  await input.fill(message);
  await page.keyboard.press('Enter');

  // Slash commands need an extra Enter to confirm
  if (message.startsWith('/')) {
    await page.waitForTimeout(500);
    await page.keyboard.press('Enter');
  }

  // Stream the response
  let lastContent = '';
  let stableCount = 0;
  const STABLE_THRESHOLD = 5;
  const CHECK_INTERVAL = 200;
  const TIMEOUT = 300000; // 5 minutes
  const startTime = Date.now();

  while (Date.now() - startTime < TIMEOUT) {
    await page.waitForTimeout(CHECK_INTERVAL);

    const responses = await page.$$('div.claude-response');
    
    if (responses.length > countBefore) {
      const lastResponse = responses[responses.length - 1];
      const content = await lastResponse.textContent();
      
      // Emit chunk if content changed
      if (content !== lastContent) {
        const newContent = content.slice(lastContent.length);
        if (newContent) {
          emit({ type: 'chunk', content: newContent });
        }
        lastContent = content;
        stableCount = 0;
      } else if (content.length > 0) {
        stableCount++;
        if (stableCount >= STABLE_THRESHOLD) {
          emit({ type: 'complete', content: content.trim() });
          return;
        }
      }
    }
  }

  emit({ type: 'error', message: 'Response timeout' });
}

// Open the sidepanel
let sidepanel = context.pages().find(p => p.url().includes('sidepanel.html'));
if (!sidepanel) {
  sidepanel = await context.newPage();
  await sidepanel.goto(SIDEPANEL_URL);
  await sidepanel.waitForLoadState('networkidle');
  await sidepanel.waitForTimeout(2000);
}

// Check if authenticated
const inputSelector = '[contenteditable="true"].ProseMirror, textarea';
const input = await sidepanel.waitForSelector(inputSelector, {
  timeout: 10000,
}).catch(() => null);

if (!input) {
  emit({ type: 'error', message: 'Claude extension not authenticated' });
  return { error: 'not authenticated' };
}

emit({ type: 'ready' });

// Set up stdin listener for messages
const readline = require('readline');
const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
  terminal: false,
});

rl.on('line', async (line) => {
  try {
    const command = JSON.parse(line);
    
    if (command.type === 'message' && command.content) {
      await sendMessage(sidepanel, command.content);
    } else if (command.type === 'quit') {
      rl.close();
      process.exit(0);
    }
  } catch (e) {
    emit({ type: 'error', message: e.message });
  }
});

// Keep the script running
await new Promise(() => {});
