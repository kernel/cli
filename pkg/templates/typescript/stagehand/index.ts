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
    let settled = false;
    let messageId = 0;

    const finish = (error: Error | null, id?: string) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      clearInterval(poller);
      ws.close();
      error ? reject(error) : resolve(id!);
    };

    const poll = () => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ id: ++messageId, method: "Target.getTargets" }));
      }
    };
    const timer = setTimeout(
      () => finish(new Error("Timed out waiting for the Stagehand extension service worker")),
      timeoutMs,
    );
    const poller = setInterval(poll, 500);

    ws.on("open", poll);
    ws.on("error", (error: Error) => finish(error));
    ws.on("close", () => finish(new Error("CDP socket closed before the Stagehand extension was found")));
    ws.on("message", (buf: Buffer) => {
      let targets: Array<{ type: string; url: string }> | undefined;
      try {
        targets = JSON.parse(buf.toString()).result?.targetInfos;
      } catch {
        return;
      }
      const worker = targets?.find(
        (t) =>
          t.type === "service_worker" &&
          t.url.startsWith("chrome-extension://") &&
          t.url.includes("service-worker.js"),
      );
      if (worker) finish(null, worker.url.split("/")[2]);
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

    // Stagehand only closes browsers it launched, so we close the connection and
    // delete the Kernel browser ourselves, even if the automation throws.
    let stagehand: Awaited<ReturnType<typeof Stagehand.create>> | undefined;
    let browser: Awaited<ReturnType<typeof localBrowser.connect>> | undefined;
    try {
      const extensionId = await discoverExtensionId(kernelBrowser.cdp_ws_url);

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
