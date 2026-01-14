import { chromium, type Browser, type Page } from 'playwright-core';
import { BasePlaywrightComputer } from './base';

/**
 * KernelPlaywrightComputer connects to a remote browser instance via CDP WebSocket URL.
 * Similar to LocalPlaywrightComputer but uses an existing browser instance instead of launching one.
 */
export class KernelPlaywrightComputer extends BasePlaywrightComputer {
  private cdp_ws_url: string;

  constructor(cdp_ws_url: string) {
    super();
    this.cdp_ws_url = cdp_ws_url;
  }

  _getBrowserAndPage = async (): Promise<[Browser, Page]> => {
    const [width, height] = this.getDimensions();

    // Connect to existing browser instance via CDP
    const browser = await chromium.connectOverCDP(this.cdp_ws_url);

    // Get existing context or create new one
    let context = browser.contexts()[0];
    if (!context) {
      context = await browser.newContext();
    }

    // Add event listeners for page creation and closure
    context.on('page', this._handleNewPage.bind(this));

    // Get existing page or create new one
    let page = context.pages()[0];
    if (!page) {
      page = await context.newPage();
    }

    // Set viewport size
    await page.setViewportSize({ width, height });
    page.on('close', this._handlePageClose.bind(this));

    return [browser, page];
  };
}
