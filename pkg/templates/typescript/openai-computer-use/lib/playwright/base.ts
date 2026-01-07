import type { Browser, Page, Request, Response, Route } from 'playwright-core';
import sharp from 'sharp';
import utils from '../utils';

// CUA key -> Playwright key mapping
const KEY_MAP: Record<string, string> = {
  '/': '/',
  '\\': '\\',
  alt: 'Alt',
  arrowdown: 'ArrowDown',
  arrowleft: 'ArrowLeft',
  arrowright: 'ArrowRight',
  arrowup: 'ArrowUp',
  backspace: 'Backspace',
  capslock: 'CapsLock',
  cmd: 'Meta',
  ctrl: 'Control',
  delete: 'Delete',
  end: 'End',
  enter: 'Enter',
  esc: 'Escape',
  home: 'Home',
  insert: 'Insert',
  option: 'Alt',
  pagedown: 'PageDown',
  pageup: 'PageUp',
  shift: 'Shift',
  space: ' ',
  super: 'Meta',
  tab: 'Tab',
  win: 'Meta',
};

interface Point {
  x: number;
  y: number;
}

export class BasePlaywrightComputer {
  protected _browser: Browser | null = null;
  protected _page: Page | null = null;

  constructor() {
    this._browser = null;
    this._page = null;
  }

  /**
   * Type guard to assert that this._page is present and is a Playwright Page.
   * Throws an error if not present.
   */
  protected _assertPage(): asserts this is { _page: Page } {
    if (!this._page) {
      throw new Error('Playwright Page is not initialized. Did you forget to call enter()?');
    }
  }

  protected _handleNewPage = (page: Page): void => {
    /** Handle the creation of a new page. */
    console.log('New page created');
    this._page = page;
    page.on('close', this._handlePageClose.bind(this));
  };

  protected _handlePageClose = (page: Page): void => {
    /** Handle the closure of a page. */
    console.log('Page closed');
    try {
      this._assertPage();
    } catch {
      return;
    }
    if (this._page !== page) return;

    const browser = this._browser;
    if (!browser || typeof browser.contexts !== 'function') {
      console.log('Warning: Browser or context not available.');
      this._page = undefined as unknown as Page;
      return;
    }

    const contexts = browser.contexts();
    if (!contexts.length) {
      console.log('Warning: No browser contexts available.');
      this._page = undefined as unknown as Page;
      return;
    }

    const context = contexts[0];
    if (!context || typeof context.pages !== 'function') {
      console.log('Warning: Context pages not available.');
      this._page = undefined as unknown as Page;
      return;
    }

    const pages = context.pages();
    if (pages.length) {
      this._page = pages[pages.length - 1] as Page;
    } else {
      console.log('Warning: All pages have been closed.');
      this._page = undefined as unknown as Page;
    }
  };

  // Subclass hook
  protected _getBrowserAndPage = async (): Promise<[Browser, Page]> => {
    // Subclasses must implement, returning [Browser, Page]
    throw new Error('Subclasses must implement _getBrowserAndPage()');
  };

  getEnvironment = (): 'windows' | 'mac' | 'linux' | 'ubuntu' | 'browser' => {
    return 'browser';
  };

  getDimensions = (): [number, number] => {
    return [1024, 768];
  };

  enter = async (): Promise<this> => {
    // Call the subclass hook for getting browser/page
    [this._browser, this._page] = await this._getBrowserAndPage();

    // Set up network interception to flag URLs matching domains in BLOCKED_DOMAINS
    const handleRoute = (route: Route, request: Request): void => {
      const url = request.url();
      if (utils.checkBlocklistedUrl(url)) {
        console.log(`Flagging blocked domain: ${url}`);
        route.abort();
      } else {
        route.continue();
      }
    };

    this._assertPage();
    await this._page.route('**/*', handleRoute);
    return this;
  };

  exit = async (): Promise<void> => {
    if (this._browser) await this._browser.close();
  };

  getCurrentUrl = (): string => {
    this._assertPage();
    return this._page.url();
  };

  screenshot = async (): Promise<string> => {
    this._assertPage();
    const buf = await this._page.screenshot({ fullPage: false });
    const webp = await sharp(buf).webp().toBuffer();
    return webp.toString('base64');
  };

  click = async (
    button: 'left' | 'right' | 'back' | 'forward' | 'wheel',
    x: number,
    y: number,
  ): Promise<void> => {
    this._assertPage();
    switch (button) {
      case 'back':
        await this.back();
        return;
      case 'forward':
        await this.forward();
        return;
      case 'wheel':
        await this._page.mouse.wheel(x, y);
        return;
      default: {
        const btn = button === 'right' ? 'right' : 'left';
        await this._page.mouse.click(x, y, { button: btn });
        return;
      }
    }
  };

  doubleClick = async (x: number, y: number): Promise<void> => {
    this._assertPage();
    await this._page.mouse.dblclick(x, y);
  };

  scroll = async (x: number, y: number, scrollX: number, scrollY: number): Promise<void> => {
    this._assertPage();
    await this._page.mouse.move(x, y);
    await this._page.evaluate(
      (params: { dx: number; dy: number }) => window.scrollBy(params.dx, params.dy),
      { dx: scrollX, dy: scrollY },
    );
  };

  type = async (text: string): Promise<void> => {
    this._assertPage();
    await this._page.keyboard.type(text);
  };

  keypress = async (keys: string[]): Promise<void> => {
    this._assertPage();
    const mapped = keys.map((k) => KEY_MAP[k.toLowerCase()] ?? k);
    for (const k of mapped) await this._page.keyboard.down(k);
    for (const k of [...mapped].reverse()) await this._page.keyboard.up(k);
  };

  wait = async (ms = 1000): Promise<void> => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  };

  move = async (x: number, y: number): Promise<void> => {
    this._assertPage();
    await this._page.mouse.move(x, y);
  };

  drag = async (path: Point[]): Promise<void> => {
    this._assertPage();
    const first = path[0];
    if (!first) return;
    await this._page.mouse.move(first.x, first.y);
    await this._page.mouse.down();
    for (const pt of path.slice(1)) await this._page.mouse.move(pt.x, pt.y);
    await this._page.mouse.up();
  };

  goto = async (url: string): Promise<Response | null> => {
    this._assertPage();
    try {
      return await this._page.goto(url);
    } catch {
      return null;
    }
  };

  back = async (): Promise<Response | null> => {
    this._assertPage();
    return (await this._page.goBack()) || null;
  };

  forward = async (): Promise<Response | null> => {
    this._assertPage();
    return (await this._page.goForward()) || null;
  };
}
