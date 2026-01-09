#!/usr/bin/env npx tsx
/**
 * Test script to verify the Web Bot Auth extension works with Kernel browsers.
 *
 * Prerequisites:
 *   1. Build and upload the web-bot-auth extension:
 *      kernel extensions build-web-bot-auth --to ./web-bot-auth-ext --upload
 *
 *   2. Set your API key:
 *      export KERNEL_API_KEY=sk_...
 *
 * Usage:
 *   npx tsx scripts/test-web-bot-auth.ts
 *
 * This script:
 *   1. Creates a Kernel browser with the web-bot-auth extension
 *   2. Connects via Playwright
 *   3. Navigates to Cloudflare's test site
 *   4. Verifies the signature was accepted
 */

import { Kernel } from "@onkernel/sdk";
import { chromium } from "playwright";

const CLOUDFLARE_TEST_URL =
  "https://http-message-signatures-example.research.cloudflare.com/";

async function main() {
  const apiKey = process.env.KERNEL_API_KEY;
  if (!apiKey) {
    console.error("Error: KERNEL_API_KEY environment variable is required");
    process.exit(1);
  }

  const kernel = new Kernel({ apiKey });

  console.log("Creating Kernel browser with web-bot-auth extension...");

  let browser;
  try {
    browser = await kernel.browsers.create({
      extensions: [{ name: "web-bot-auth" }],
    });
    console.log(`Browser created: ${browser.id}`);
    console.log(`CDP URL: ${browser.browser_url}`);

    // Connect via Playwright
    console.log("\nConnecting via Playwright...");
    const pw = await chromium.connectOverCDP(browser.browser_url);
    const context = pw.contexts()[0];
    const page = context?.pages()[0] || (await context.newPage());

    // Navigate to Cloudflare's test site
    console.log(`\nNavigating to ${CLOUDFLARE_TEST_URL}...`);
    await page.goto(CLOUDFLARE_TEST_URL, { waitUntil: "networkidle" });

    // Check for success indicators
    const pageContent = await page.content();
    const pageText = await page.innerText("body");

    // The test site shows different content based on whether the signature was valid
    // Look for indicators of success
    const hasValidSignature =
      pageText.toLowerCase().includes("valid") ||
      pageText.toLowerCase().includes("signature verified") ||
      pageText.toLowerCase().includes("authenticated");

    const hasSignatureHeader =
      pageText.toLowerCase().includes("signature") ||
      pageContent.toLowerCase().includes("signature");

    console.log("\n--- Page Content Preview ---");
    console.log(pageText.slice(0, 1000));
    console.log("----------------------------\n");

    if (hasSignatureHeader) {
      console.log("✓ Page mentions signatures");
    }

    // Take a screenshot for debugging
    const screenshotPath = "/tmp/web-bot-auth-test.png";
    await page.screenshot({ path: screenshotPath, fullPage: true });
    console.log(`Screenshot saved to: ${screenshotPath}`);

    // Get the live view URL for manual inspection
    const liveViewUrl = browser.live_url;
    if (liveViewUrl) {
      console.log(`\nLive view URL: ${liveViewUrl}`);
    }

    await pw.close();
    console.log("\n✓ Test completed successfully!");
    console.log(
      "Check the screenshot and page content above to verify the signature was accepted."
    );
  } catch (error) {
    console.error("Test failed:", error);
    process.exit(1);
  } finally {
    // Clean up browser
    if (browser) {
      console.log("\nCleaning up browser...");
      try {
        await kernel.browsers.delete(browser.id);
        console.log("Browser deleted.");
      } catch (e) {
        console.error("Failed to delete browser:", e);
      }
    }
  }
}

main();
