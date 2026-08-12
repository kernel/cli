import { Stagehand, localBrowser, type ModelName } from "@browserbasehq/stagehand";
import { Kernel, type KernelContext } from "@onkernel/sdk";
import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
// Stagehand v4 uses zod v4 schema types.
import { z } from "zod";

const kernel = new Kernel();

const app = kernel.app("ts-stagehand");

interface CompanyInput {
  company: string;
}

interface TeamSizeOutput {
  teamSize: string;
}

// LLM API Keys are set in the environment during `kernel deploy <filename> -e MODEL_API_KEY=XXX`
// See https://www.kernel.sh/docs/apps/deploy#environment-variables
// Defaults to OpenAI. Override MODEL to use another provider (e.g. google/gemini-2.5-flash).
const MODEL = (process.env.MODEL ?? "openai/gpt-4.1") as ModelName;
const MODEL_API_KEY = process.env.MODEL_API_KEY ?? process.env.OPENAI_API_KEY;

if (!MODEL_API_KEY) {
  throw new Error("MODEL_API_KEY (or OPENAI_API_KEY) is not set");
}

// Stagehand v4 runs as a Chrome extension alongside the browser. When connecting
// to a remote Kernel browser over CDP we preload that extension at browser
// creation and hand its runtime id to `localBrowser.connect`.
const STAGEHAND_EXTENSION_NAME = "stagehand-runtime";

// The extension archive ships inside the installed @browserbasehq/stagehand package.
const stagehandDist = dirname(fileURLToPath(import.meta.resolve("@browserbasehq/stagehand")));
const STAGEHAND_EXTENSION_ZIP = join(stagehandDist, "assets/stagehand-extension.zip");

// Upload the Stagehand extension to the project once and reuse it thereafter,
// returning the Kernel extension id.
async function ensureStagehandExtension(): Promise<string> {
  const existing = await kernel.extensions.list();
  const extension = existing.find((ext) => ext.name === STAGEHAND_EXTENSION_NAME);
  if (extension) return extension.id;

  const uploaded = await kernel.extensions.upload({
    file: createReadStream(STAGEHAND_EXTENSION_ZIP),
    name: STAGEHAND_EXTENSION_NAME,
  });
  return uploaded.id;
}

// Chrome derives an unpacked extension's runtime id from the absolute path it is
// loaded from. Kernel extracts a preloaded extension to
// `/home/kernel/extensions/<kernel-extension-id>`, so we can compute the runtime
// id directly instead of discovering it over CDP: SHA-256 the path, take the
// first 16 bytes, and map each nibble to a..p. This holds because the Stagehand
// extension ships without a manifest `key` (a keyed manifest would pin the id).
function chromeExtensionId(kernelExtensionId: string): string {
  const extensionPath = `/home/kernel/extensions/${kernelExtensionId}`;
  const digest = createHash("sha256").update(extensionPath).digest().subarray(0, 16);
  return [...digest]
    .flatMap((byte) => [byte >> 4, byte & 0xf])
    .map((nibble) => String.fromCharCode("a".charCodeAt(0) + nibble))
    .join("");
}

app.action<CompanyInput, TeamSizeOutput>(
  "teamsize-task",
  async (ctx: KernelContext, payload?: CompanyInput): Promise<TeamSizeOutput> => {
    // A function that returns the team size of a Y Combinator startup

    // Args:
    //     ctx: Kernel context containing invocation information
    //     payload: A startup name to search for on YCombinator's website

    // Returns:
    //     output: The team size (number of employees) of the startup

    const company = payload?.company || "kernel";

    const kernelExtensionId = await ensureStagehandExtension();
    const extensionId = chromeExtensionId(kernelExtensionId);

    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
      stealth: true,
      extensions: [{ id: kernelExtensionId }],
    });

    console.log("Kernel browser live view url: ", kernelBrowser.browser_live_view_url);

    // Stagehand only closes browsers it launched, so we close the connection and
    // delete the Kernel browser ourselves, even if the automation throws.
    let stagehand: Awaited<ReturnType<typeof Stagehand.create>> | undefined;
    let browser: Awaited<ReturnType<typeof localBrowser.connect>> | undefined;
    try {
      browser = await localBrowser.connect({
        cdpUrl: kernelBrowser.cdp_ws_url,
        extensionId,
      });
      stagehand = await Stagehand.create({
        browser,
        model: { modelName: MODEL, apiKey: MODEL_API_KEY },
        logging: { level: "info" },
      });

      /////////////////////////////////////
      // Your Stagehand implementation here
      /////////////////////////////////////
      const page = await browser.context.activePage();
      if (!page) throw new Error("No active page in the Kernel browser");
      await page.goto("https://www.ycombinator.com/companies");

      await stagehand.act(`Type in ${company} into the search box`);
      await stagehand.act("Click on the first search result");

      // Extract team size from the YC startup page. Every v4 primitive returns { data }.
      const { data } = await stagehand.extract(
        "Extract the team size (number of employees) shown on this Y Combinator company page.",
        z.object({ teamSize: z.string() }),
      );

      return data;
    } finally {
      await stagehand?.close();
      await browser?.close();
      await kernel.browsers.deleteByID(kernelBrowser.session_id);
    }
  },
);
