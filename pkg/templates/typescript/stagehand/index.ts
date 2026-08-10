import { Stagehand, localBrowser, type ModelName } from "@browserbasehq/stagehand";
import { Kernel, type KernelContext } from "@onkernel/sdk";
import { createReadStream } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { WebSocket } from "ws";
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

// Upload the Stagehand extension to the project once and reuse it thereafter.
async function ensureStagehandExtension(): Promise<string> {
  const existing = await kernel.extensions.list();
  for (const ext of existing) {
    if (ext.name === STAGEHAND_EXTENSION_NAME) return STAGEHAND_EXTENSION_NAME;
  }
  await kernel.extensions.upload({
    file: createReadStream(STAGEHAND_EXTENSION_ZIP),
    name: STAGEHAND_EXTENSION_NAME,
  });
  return STAGEHAND_EXTENSION_NAME;
}

// Chrome assigns the preloaded extension a runtime id. Read it off the extension's
// service worker target so we can attach Stagehand to it.
async function discoverExtensionId(cdpUrl: string, timeoutMs = 20_000): Promise<string> {
  return await new Promise<string>((resolve, reject) => {
    const ws = new WebSocket(cdpUrl);
    const deadline = Date.now() + timeoutMs;
    let messageId = 0;

    const poll = () => ws.send(JSON.stringify({ id: ++messageId, method: "Target.getTargets" }));

    ws.on("open", poll);
    ws.on("error", reject);
    ws.on("message", (buf: Buffer) => {
      const targets = JSON.parse(buf.toString()).result?.targetInfos;
      if (!targets) return;
      const worker = targets.find(
        (t: { type: string; url: string }) =>
          t.type === "service_worker" &&
          t.url.startsWith("chrome-extension://") &&
          t.url.includes("service-worker.js"),
      );
      if (worker) {
        ws.close();
        resolve(worker.url.split("/")[2]);
      } else if (Date.now() > deadline) {
        ws.close();
        reject(new Error("Timed out waiting for the Stagehand extension service worker"));
      } else {
        setTimeout(poll, 500);
      }
    });
  });
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

    const extensionName = await ensureStagehandExtension();

    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
      stealth: true,
      extensions: [{ name: extensionName }],
    });

    console.log("Kernel browser live view url: ", kernelBrowser.browser_live_view_url);

    const extensionId = await discoverExtensionId(kernelBrowser.cdp_ws_url);

    const browser = await localBrowser.connect({
      cdpUrl: kernelBrowser.cdp_ws_url,
      extensionId,
    });
    const stagehand = await Stagehand.create({
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

    // Stagehand only closes browsers it launched, so close the connection and
    // delete the Kernel browser ourselves.
    await stagehand.close();
    await browser.close();
    await kernel.browsers.deleteByID(kernelBrowser.session_id);

    return data;
  },
);
