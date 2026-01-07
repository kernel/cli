import { chromium, type Browser, type Page } from 'playwright-core';
import { BasePlaywrightComputer } from './base';

/**
 * Launches a local Chromium instance using Playwright.
 */
export class LocalPlaywrightComputer extends BasePlaywrightComputer {
  private headless: boolean;

  constructor(headless = false) {
    super();
    this.headless = headless;
  }

  _getBrowserAndPage = async (): Promise<[Browser, Page]> => {
    const [width, height] = this.getDimensions();
    const launchArgs = [
      `--window-size=${width},${height}`,
      '--disable-extensions',
      '--disable-file-system',
    ];

    const browser = await chromium.launch({
      headless: this.headless,
      args: launchArgs,
      env: { DISPLAY: ':0' },
    });

    const context = await browser.newContext();

    // Add event listeners for page creation and closure
    context.on('page', this._handleNewPage.bind(this));

    const page = await context.newPage();
    await page.setViewportSize({ width, height });
    page.on('close', this._handlePageClose.bind(this));

    await page.goto('https://duckduckgo.com');

    // console.dir({debug_getBrowserAndPage: [browser, page]});
    return [browser, page];
  };
}
