/**
 * Yutori n1 Playwright Computer Tool
 * 
 * Maps n1 action format to Playwright methods via CDP WebSocket connection.
 * Uses viewport-only screenshots optimized for Yutori n1's training data.
 * 
 * @see https://docs.yutori.com/reference/n1#screenshot-requirements
 */

import { chromium, type Browser, type BrowserContext, type Page } from 'playwright-core';
import type { ToolResult, N1Action } from './computer';
import { ToolError } from './computer';

const SCREENSHOT_DELAY_MS = 300;

// Key mappings from n1 output format to Playwright format
const KEY_MAP: Record<string, string> = {
  'Return': 'Enter',
  'BackSpace': 'Backspace',
  'Page_Up': 'PageUp',
  'Page_Down': 'PageDown',
};

const MODIFIER_MAP: Record<string, string> = {
  'ctrl': 'Control',
  'super': 'Meta',
  'command': 'Meta',
  'cmd': 'Meta',
};

/**
 * Computer tool for Yutori n1 actions using Playwright via CDP connection.
 * Provides viewport-only screenshots optimized for n1 model performance.
 */
export class PlaywrightComputerTool {
  private cdpWsUrl: string;
  private width: number;
  private height: number;
  private browser: Browser | null = null;
  private context: BrowserContext | null = null;
  private page: Page | null = null;

  constructor(cdpWsUrl: string, width = 1200, height = 800) {
    this.cdpWsUrl = cdpWsUrl;
    this.width = width;
    this.height = height;
  }

  /**
   * Connect to the browser via CDP WebSocket.
   * Must be called before executing any actions.
   */
  async connect(): Promise<void> {
    if (this.browser) {
      return; // Already connected
    }

    this.browser = await chromium.connectOverCDP(this.cdpWsUrl);

    // Get existing context or create new one
    this.context = this.browser.contexts()[0];
    if (!this.context) {
      this.context = await this.browser.newContext();
    }

    // Handle new page events
    this.context.on('page', this.handleNewPage.bind(this));

    // Get existing page or create new one
    this.page = this.context.pages()[0];
    if (!this.page) {
      this.page = await this.context.newPage();
    }

    // Set viewport size to Yutori's recommended dimensions
    await this.page.setViewportSize({ width: this.width, height: this.height });
    this.page.on('close', this.handlePageClose.bind(this));
  }

  /**
   * Disconnect from the browser.
   */
  async disconnect(): Promise<void> {
    if (this.browser) {
      // Don't close the browser itself - just disconnect the CDP connection
      // The browser lifecycle is managed by Kernel
      this.browser = null;
      this.context = null;
      this.page = null;
    }
  }

  private handleNewPage(page: Page): void {
    console.log('New page created');
    this.page = page;
    page.on('close', this.handlePageClose.bind(this));
  }

  private handlePageClose(closedPage: Page): void {
    console.log('Page closed');
    if (this.page === closedPage && this.context) {
      const pages = this.context.pages();
      if (pages.length > 0) {
        this.page = pages[pages.length - 1];
      } else {
        console.warn('Warning: All pages have been closed.');
        this.page = null;
      }
    }
  }

  private assertPage(): asserts this is { page: Page } {
    if (!this.page) {
      throw new ToolError('Page not available. Did you call connect()?');
    }
  }

  /**
   * Execute an n1 action and return the result.
   */
  async execute(action: N1Action): Promise<ToolResult> {
    this.assertPage();
    const { action_type } = action;

    switch (action_type) {
      case 'click':
        return this.handleClick(action);
      case 'scroll':
        return this.handleScroll(action);
      case 'type':
        return this.handleType(action);
      case 'key_press':
        return this.handleKeyPress(action);
      case 'hover':
        return this.handleHover(action);
      case 'drag':
        return this.handleDrag(action);
      case 'wait':
        return this.handleWait();
      case 'refresh':
        return this.handleRefresh();
      case 'go_back':
        return this.handleGoBack();
      case 'goto_url':
        return this.handleGotoUrl(action);
      case 'read_texts_and_links':
        return this.handleReadTextsAndLinks();
      case 'stop':
        return this.handleStop(action);
      default:
        throw new ToolError(`Unknown action type: ${action_type}`);
    }
  }

  private async handleClick(action: N1Action): Promise<ToolResult> {
    this.assertPage();
    const coords = this.getCoordinates(action.center_coordinates);

    await this.page.mouse.click(coords.x, coords.y);
    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleScroll(action: N1Action): Promise<ToolResult> {
    this.assertPage();
    const coords = this.getCoordinates(action.center_coordinates);
    const direction = action.direction;
    const amount = action.amount ?? 3;

    if (!direction || !['up', 'down', 'left', 'right'].includes(direction)) {
      throw new ToolError(`Invalid scroll direction: ${direction}`);
    }

    // Each scroll amount unit ≈ 10-15% of screen, roughly 100 pixels
    const scrollDelta = amount * 100;

    // Move mouse to position first
    await this.page.mouse.move(coords.x, coords.y);

    // Playwright's wheel method takes deltaX and deltaY
    let deltaX = 0;
    let deltaY = 0;

    switch (direction) {
      case 'up':
        deltaY = -scrollDelta;
        break;
      case 'down':
        deltaY = scrollDelta;
        break;
      case 'left':
        deltaX = -scrollDelta;
        break;
      case 'right':
        deltaX = scrollDelta;
        break;
    }

    await this.page.mouse.wheel(deltaX, deltaY);
    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleType(action: N1Action): Promise<ToolResult> {
    this.assertPage();
    const text = action.text;
    if (!text) {
      throw new ToolError('text is required for type action');
    }

    // Clear existing text if requested
    if (action.clear_before_typing) {
      await this.page.keyboard.press('Control+a');
      await this.sleep(100);
      await this.page.keyboard.press('Backspace');
      await this.sleep(100);
    }

    // Type the text
    await this.page.keyboard.type(text);

    // Press Enter if requested
    if (action.press_enter_after) {
      await this.sleep(100);
      await this.page.keyboard.press('Enter');
    }

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleKeyPress(action: N1Action): Promise<ToolResult> {
    this.assertPage();
    const keyComb = action.key_comb;
    if (!keyComb) {
      throw new ToolError('key_comb is required for key_press action');
    }

    const mappedKey = this.mapKeyToPlaywright(keyComb);
    await this.page.keyboard.press(mappedKey);

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleHover(action: N1Action): Promise<ToolResult> {
    this.assertPage();
    const coords = this.getCoordinates(action.center_coordinates);

    await this.page.mouse.move(coords.x, coords.y);

    await this.sleep(SCREENSHOT_DELAY_MS);
    return this.screenshot();
  }

  private async handleDrag(action: N1Action): Promise<ToolResult> {
    this.assertPage();
    const startCoords = this.getCoordinates(action.start_coordinates);
    const endCoords = this.getCoordinates(action.center_coordinates);

    // Move to start position
    await this.page.mouse.move(startCoords.x, startCoords.y);
    
    // Press mouse button and wait for dragstart event
    await this.page.mouse.down();
    await this.sleep(50);
    
    // Move gradually to end position using steps for proper drag-and-drop
    // The steps parameter makes Playwright simulate intermediate mouse positions
    // which is required for HTML5 drag-and-drop to work properly
    await this.page.mouse.move(endCoords.x, endCoords.y, { steps: 12 });
    
    // Release mouse button
    await this.page.mouse.up();

    await this.sleep(300);
    return this.screenshot();
  }

  private async handleWait(): Promise<ToolResult> {
    // Default wait of 2 seconds for UI to update
    await this.sleep(2000);
    return this.screenshot();
  }

  private async handleRefresh(): Promise<ToolResult> {
    this.assertPage();
    await this.page.reload();

    // Wait for page to reload
    await this.sleep(2000);
    return this.screenshot();
  }

  private async handleGoBack(): Promise<ToolResult> {
    this.assertPage();
    await this.page.goBack();

    // Wait for navigation
    await this.sleep(1500);
    return this.screenshot();
  }

  private async handleGotoUrl(action: N1Action): Promise<ToolResult> {
    this.assertPage();
    const url = action.url;
    if (!url) {
      throw new ToolError('url is required for goto_url action');
    }

    await this.page.goto(url);

    // Wait for page to load
    await this.sleep(2000);
    return this.screenshot();
  }

  /**
   * Read texts and links using Playwright's ariaSnapshot() API.
   * Returns accessibility tree representation of the page content.
   */
  private async handleReadTextsAndLinks(): Promise<ToolResult> {
    this.assertPage();
    try {
      // Use the public ariaSnapshot() API on the body locator
      // This provides an accessibility tree representation of the page
      const snapshot = await this.page.locator('body').ariaSnapshot();
      const url = this.page.url();
      const title = await this.page.title();

      // Get viewport-only screenshot
      const screenshotResult = await this.screenshot();

      return {
        base64Image: screenshotResult.base64Image,
        output: JSON.stringify({ url, title, snapshot }, null, 2),
      };
    } catch (error) {
      console.warn('read_texts_and_links failed:', error);
      return this.screenshot();
    }
  }

  private handleStop(action: N1Action): ToolResult {
    // Return the final answer without taking a screenshot
    return {
      output: action.answer || 'Task completed',
    };
  }

  /**
   * Take a viewport-only screenshot of the current browser state.
   * This captures only the browser content, not the OS UI or browser chrome.
   */
  async screenshot(): Promise<ToolResult> {
    this.assertPage();
    try {
      // fullPage: false captures only the viewport (browser content)
      const buffer = await this.page.screenshot({ fullPage: false });

      return {
        base64Image: buffer.toString('base64'),
      };
    } catch (error) {
      throw new ToolError(`Failed to take screenshot: ${error}`);
    }
  }

  /**
   * Get the current page URL.
   */
  getCurrentUrl(): string {
    this.assertPage();
    return this.page.url();
  }

  private getCoordinates(coords?: [number, number]): { x: number; y: number } {
    if (!coords || coords.length !== 2) {
      // Default to center of viewport
      return { x: this.width / 2, y: this.height / 2 };
    }

    const [x, y] = coords;
    if (typeof x !== 'number' || typeof y !== 'number' || x < 0 || y < 0) {
      throw new ToolError(`Invalid coordinates: ${JSON.stringify(coords)}`);
    }

    return { x, y };
  }

  /**
   * Map key names to Playwright format.
   * n1 outputs keys in Playwright format, but some may need adjustment.
   */
  private mapKeyToPlaywright(key: string): string {
    // Handle modifier combinations (e.g., "ctrl+a" -> "Control+a")
    if (key.includes('+')) {
      const parts = key.split('+');
      const mappedParts = parts.map((part) => {
        const trimmed = part.trim();
        const lower = trimmed.toLowerCase();

        // Map modifier names
        if (MODIFIER_MAP[lower]) {
          return MODIFIER_MAP[lower];
        }

        // Check KEY_MAP for special keys
        return KEY_MAP[trimmed] || trimmed;
      });
      return mappedParts.join('+');
    }

    return KEY_MAP[key] || key;
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
