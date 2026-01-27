/**
 * Helper utilities for page manipulation and response parsing
 */

import type { Page } from "playwright-core";

// ============================================================================
// Page Manipulation Helpers
// ============================================================================

/**
 * Scroll through the page to trigger lazy loading of images.
 * Scrolls incrementally and returns to the top when complete.
 */
export async function scrollAndLoadImages(page: Page): Promise<void> {
  await page.evaluate(`
    new Promise((resolve) => {
      const scrollHeight = document.documentElement.scrollHeight;
      const viewportHeight = window.innerHeight;
      let currentPosition = 0;
      const scrollStep = viewportHeight;

      function scrollNext() {
        if (currentPosition < scrollHeight) {
          window.scrollTo(0, currentPosition);
          currentPosition += scrollStep;
          setTimeout(scrollNext, 300);
        } else {
          window.scrollTo(0, 0);
          setTimeout(resolve, 500);
        }
      }

      scrollNext();
    })
  `);
}

// ============================================================================
// Popup Dismissal
// ============================================================================

/** Selectors for identifying popup/modal containers */
const POPUP_CONTAINER_SELECTORS = [
  '[role="dialog"]',
  '[role="alertdialog"]',
  ".modal",
  ".popup",
  ".overlay",
  ".cookie-banner",
  ".cookie-consent",
  "#cookie-consent",
  ".notification",
  ".toast",
  ".alert",
  ".snackbar",
  '[class*="modal" i]',
  '[class*="popup" i]',
  '[class*="overlay" i]',
  '[class*="banner" i]',
  '[class*="cookie" i]',
  '[id*="cookie" i]',
  '[id*="modal" i]',
  '[id*="popup" i]',
];

/** Text patterns for accept/agree buttons */
const ACCEPT_BUTTON_TEXTS = [
  "Accept all",
  "Accept All",
  "I agree",
  "I Agree",
  "Allow all",
  "Allow All",
  "Accept cookies",
  "Accept Cookies",
];

/** CSS selectors for accept buttons */
const ACCEPT_BUTTON_SELECTORS = [
  '[aria-label*="accept" i]',
  '[aria-label*="agree" i]',
  '[aria-label*="allow" i]',
  ".accept-button",
  "#accept-cookies",
  'button[id*="accept" i]',
  'button[class*="accept" i]',
  '[data-action*="accept" i]',
];

/** Text patterns for close/dismiss buttons */
const CLOSE_BUTTON_TEXTS = [
  "Close",
  "×",
  "✕",
  "Dismiss",
  "No thanks",
  "No Thanks",
  "Maybe later",
  "Reject all",
  "Reject All",
  "Skip",
];

/** CSS selectors for close buttons */
const CLOSE_BUTTON_SELECTORS = [
  // Aria labels (accessibility)
  '[aria-label*="close" i]',
  '[aria-label*="dismiss" i]',
  '[aria-label*="remove" i]',
  // Common close button classes
  ".close-button",
  ".close-icon",
  ".modal-close",
  ".popup-close",
  ".dialog-close",
  "button.close",
  ".toast-close",
  ".notification-close",
  ".banner-close",
  // Data attributes
  '[data-dismiss="modal"]',
  '[data-dismiss="toast"]',
  '[data-dismiss="alert"]',
  '[data-action*="close" i]',
  '[data-action*="dismiss" i]',
  // Class name patterns
  'button[class*="close" i]',
  'button[class*="dismiss" i]',
  'span[class*="close" i]',
  'div[class*="close" i]',
  'a[class*="close" i]',
  // SVG close icons
  'svg[class*="close" i]',
  "button > svg",
  '[aria-label*="close" i] > svg',
  // ID patterns
  "#close-button",
  "#dismiss-button",
  'button[id*="close" i]',
  'button[id*="dismiss" i]',
];

/** Toast/notification selectors */
const TOAST_SELECTORS = [
  ".toast",
  ".notification",
  ".alert",
  ".snackbar",
  '[role="alert"]',
  '[role="status"]',
];

/**
 * Dismiss popups, modals, overlays, and toast notifications that may block content.
 * Uses multiple strategies to find and close common popup patterns.
 */
export async function dismissPopups(page: Page): Promise<void> {
  try {
    let dismissed = false;

    // First, identify if there's actually a popup/modal/overlay visible
    let popupContainer = null;
    for (const selector of POPUP_CONTAINER_SELECTORS) {
      try {
        const container = page.locator(selector).first();
        if (await container.isVisible({ timeout: 300 })) {
          popupContainer = container;
          console.log(`  Found popup container: ${selector}`);
          break;
        }
      } catch {
        // Continue to next selector
      }
    }

    // If no popup container found, try ESC key and exit
    if (!popupContainer) {
      try {
        await page.keyboard.press("Escape");
        await page.waitForTimeout(300);
        console.log("  Pressed ESC key (no visible popup container found)");
      } catch {
        // ESC didn't work
      }
      return;
    }

    // Strategy 1: Try Accept/OK/I Agree buttons WITHIN the popup container only
    dismissed = await tryClickButtonsByText(popupContainer, ACCEPT_BUTTON_TEXTS, page);

    // Strategy 2: Try CSS selectors for accept buttons WITHIN popup container
    if (!dismissed) {
      dismissed = await tryClickButtonsBySelectors(popupContainer, ACCEPT_BUTTON_SELECTORS, page);
    }

    // Strategy 3: Close buttons with text WITHIN popup container
    if (!dismissed) {
      dismissed = await tryClickButtonsByText(popupContainer, CLOSE_BUTTON_TEXTS, page);
    }

    // Strategy 4: Close icons and buttons by CSS WITHIN popup container
    if (!dismissed) {
      dismissed = await tryClickButtonsBySelectors(popupContainer, CLOSE_BUTTON_SELECTORS, page);
    }

    // Strategy 5: Look for toast/notification containers and dismiss them
    if (!dismissed) {
      dismissed = await tryDismissToasts(page);
    }

    // Strategy 6: Press ESC key (works for many modals)
    try {
      await page.keyboard.press("Escape");
      await page.waitForTimeout(300);
      console.log("  Pressed ESC key");
    } catch {
      // ESC didn't work
    }

    if (dismissed) {
      console.log("  ✓ Successfully dismissed popup/toast");
    } else {
      console.log("  No popups/toasts found to dismiss");
    }
  } catch (error) {
    console.log("  Error dismissing popups:", error instanceof Error ? error.message : String(error));
  }
}

/**
 * Try to click buttons by text content within a container
 */
async function tryClickButtonsByText(
  container: ReturnType<Page["locator"]>,
  texts: string[],
  page: Page
): Promise<boolean> {
  for (const text of texts) {
    try {
      const button = container
        .locator(
          `button:has-text("${text}"), a:has-text("${text}"), div[role="button"]:has-text("${text}"), span[role="button"]:has-text("${text}")`
        )
        .first();
      if (await button.isVisible({ timeout: 500 })) {
        console.log(`  Found button with text: "${text}"`);
        await button.click();
        await page.waitForTimeout(800);
        return true;
      }
    } catch {
      // Continue to next text
    }
  }
  return false;
}

/**
 * Try to click buttons by CSS selectors within a container
 */
async function tryClickButtonsBySelectors(
  container: ReturnType<Page["locator"]>,
  selectors: string[],
  page: Page
): Promise<boolean> {
  for (const selector of selectors) {
    try {
      const button = container.locator(selector).first();
      if (await button.isVisible({ timeout: 300 })) {
        console.log(`  Found element via selector: ${selector}`);
        await button.click();
        await page.waitForTimeout(800);
        return true;
      }
    } catch {
      // Continue to next selector
    }
  }
  return false;
}

/**
 * Try to dismiss toast/notification elements
 */
async function tryDismissToasts(page: Page): Promise<boolean> {
  for (const selector of TOAST_SELECTORS) {
    try {
      const toast = page.locator(selector).first();
      if (await toast.isVisible({ timeout: 300 })) {
        const closeButton = toast.locator('button, [role="button"], .close, [aria-label*="close" i]').first();
        if (await closeButton.isVisible({ timeout: 300 })) {
          console.log(`  Found close button in toast: ${selector}`);
          await closeButton.click();
          await page.waitForTimeout(500);
          return true;
        }
      }
    } catch {
      // Continue to next selector
    }
  }
  return false;
}

// ============================================================================
// Response Parsing Helpers
// ============================================================================

/**
 * Parse AI response to extract JSON array.
 * Handles cases where the response contains additional text around the JSON.
 */
export function parseAIResponse<T>(response: string): T[] {
  try {
    const jsonMatch = response.match(/\[[\s\S]*\]/);
    if (jsonMatch) {
      return JSON.parse(jsonMatch[0]);
    }
    return [];
  } catch (error) {
    console.error("Error parsing AI response:", error);
    return [];
  }
}

// ============================================================================
// HTML Escaping
// ============================================================================

const HTML_ESCAPE_MAP: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#039;",
};

/**
 * Escape HTML special characters to prevent XSS in generated reports.
 */
export function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (char) => HTML_ESCAPE_MAP[char] || char);
}

// ============================================================================
// URL Utilities
// ============================================================================

/**
 * Normalize and validate a URL.
 * Adds https:// prefix if missing and validates the URL format.
 *
 * @throws Error if the URL is invalid
 */
export function normalizeUrl(url: string): string {
  let normalizedUrl = url;

  if (!normalizedUrl.startsWith("http://") && !normalizedUrl.startsWith("https://")) {
    normalizedUrl = `https://${normalizedUrl}`;
  }

  // Validate URL
  try {
    new URL(normalizedUrl);
  } catch {
    throw new Error(`Invalid URL: ${url}`);
  }

  return normalizedUrl;
}
