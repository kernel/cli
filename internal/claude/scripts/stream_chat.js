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

async function run({ context }) {
  // Open the sidepanel
  let sidepanel = context.pages().find(p => p.url().includes('sidepanel.html'));
  if (!sidepanel) {
    sidepanel = await context.newPage();
    await sidepanel.goto(SIDEPANEL_URL);
    await sidepanel.waitForLoadState('networkidle');
    await sidepanel.waitForTimeout(2000);
  }

  // Check if authenticated
  const textarea = await sidepanel.waitForSelector('textarea, [contenteditable="true"]', {
    timeout: 10000,
  }).catch(() => null);

  if (!textarea) {
    emit({ type: 'error', message: 'Claude extension not authenticated' });
    return;
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
}

async function sendMessage(page, message) {
  const textarea = await page.$('textarea, [contenteditable="true"]');
  if (!textarea) {
    emit({ type: 'error', message: 'Chat input not found' });
    return;
  }

  // Get current message count
  const messagesBefore = await page.$$('[data-testid="message"], .message, [class*="Message"]');
  const countBefore = messagesBefore.length;

  // Send the message
  await textarea.click();
  await textarea.fill('');
  await textarea.fill(message);
  await page.keyboard.press('Enter');

  // Stream the response
  let lastContent = '';
  let stableCount = 0;
  const STABLE_THRESHOLD = 5;
  const CHECK_INTERVAL = 200;
  const TIMEOUT = 300000; // 5 minutes
  const startTime = Date.now();

  while (Date.now() - startTime < TIMEOUT) {
    await page.waitForTimeout(CHECK_INTERVAL);

    const messages = await page.$$('[data-testid="message"], .message, [class*="Message"]');
    
    if (messages.length > countBefore) {
      const lastMessage = messages[messages.length - 1];
      const content = await lastMessage.textContent();
      
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

module.exports = run;
