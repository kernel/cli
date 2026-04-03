import { Kernel, type KernelContext } from "@onkernel/sdk";
import { chromium } from "playwright-core";
import { describeRecording, extractSegmentLevel } from "./cloudglue.js";

const kernel = new Kernel();

const app = kernel.app("ts-cloudglue-session-recap");

// --- Types ---

interface Scene {
  timestamp: string;
  thumbnail_url: string | null;
  description: string;
  user_action: string;
  screen_description: string;
}

interface RecapInput {
  /** URL of the video recording to analyze */
  recording_url: string;
  /** Custom title for the recap (auto-generated if not provided) */
  title?: string;
  /** Maximum length in seconds for each scene analyzed (2-60, defaults to 8) */
  max_seconds?: number;
}

interface RecapOutput {
  title: string;
  recording_url: string;
  summary: string;
  duration_seconds: number | null;
  scene_count: number;
  scenes: Scene[];
  markdown: string;
}

interface BrowseAndRecapInput {
  /** URL to open in the browser */
  url: string;
  /** Placeholder for template — replace with your browser agent input/logic */
  task?: string;
  /** Custom title for the recap (auto-generated if not provided) */
  title?: string;
  /** Maximum length in seconds for each scene analyzed (2-60, defaults to 8) */
  max_seconds?: number;
}

interface BrowseAndRecapOutput extends RecapOutput {
  replay_view_url: string;
}

// --- Extract schema ---

const SCENE_SCHEMA = {
  type: "object",
  properties: {
    user_action: {
      type: "string",
      description:
        "What the user did in this segment, e.g. 'Clicked the Sign In button', 'Scrolled down the page', 'Typed search query'. Use 'N/A' if no clear user action.",
    },
    screen_description: {
      type: "string",
      description:
        "Brief description of what is visible on screen: the page layout, key UI elements, content shown. Do NOT invent or guess text that is too small to read.",
    },
  },
  required: ["user_action", "screen_description"],
};

const EXTRACT_PROMPT =
  "For each segment of this recording, extract the user action and a description of what is on screen. " +
  "Be specific about user interactions (clicks, typing, scrolling, navigating). " +
  "For user_action, put 'N/A' if the user is not actively doing anything (e.g. a static screen). " +
  "IMPORTANT: Do NOT guess or invent text you cannot clearly read on screen. " +
  "Do NOT guess URLs — only report what you can confidently read.";

// --- Helpers ---

function formatTimestamp(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

function trimUrl(url: string, maxLen = 60): string {
  if (url.length <= maxLen) return url;
  try {
    const u = new URL(url);
    const trimmed = u.hostname + u.pathname;
    return trimmed.length > maxLen
      ? trimmed.slice(0, maxLen - 3) + "..."
      : trimmed;
  } catch {
    return url.slice(0, maxLen - 3) + "...";
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}



function buildScenes(
  describeResult: Record<string, unknown>,
  extractResult: Record<string, unknown>
): Scene[] {
  const descData = describeResult.data as Record<string, unknown> | undefined;
  const segmentSummary =
    (descData?.segment_summary as Array<Record<string, unknown>>) ?? [];
  const shots =
    (describeResult.shots as Array<Record<string, unknown>>) ?? [];
  const descSegments = segmentSummary.length ? segmentSummary : shots;

  const extData = extractResult.data as Record<string, unknown> | undefined;
  const segmentEntities =
    (extData?.segment_entities as Array<Record<string, unknown>>) ?? [];

  const maxLen = Math.max(descSegments.length, segmentEntities.length);
  if (maxLen === 0) return [];

  const raw: Scene[] = [];

  for (let i = 0; i < maxLen; i++) {
    const desc = descSegments[i];
    const ext = segmentEntities[i];
    const entities = (ext?.entities as Record<string, unknown>) ?? {};

    const description =
      (desc?.summary as string) ||
      (desc?.description as string) ||
      (desc?.title as string) ||
      "";

    raw.push({
      timestamp: formatTimestamp(
        Number(desc?.start_time ?? ext?.start_time ?? 0)
      ),
      thumbnail_url:
        (desc?.thumbnail_url as string) ??
        (ext?.thumbnail_url as string) ??
        null,
      description,
      user_action: (entities.user_action as string) ?? "N/A",
      screen_description: (entities.screen_description as string) ?? "",
    });
  }

  // Deduplicate scenes with the same timestamp
  const scenes: Scene[] = [];
  for (const scene of raw) {
    const prev = scenes[scenes.length - 1];
    if (prev && prev.timestamp === scene.timestamp) {
      if (!prev.description && scene.description) {
        prev.description = scene.description;
      }
      if (prev.user_action === "N/A" && scene.user_action !== "N/A") {
        prev.user_action = scene.user_action;
      } else if (prev.user_action !== "N/A" && scene.user_action !== "N/A") {
        prev.user_action += "; " + scene.user_action;
      }
      if (!prev.screen_description && scene.screen_description) {
        prev.screen_description = scene.screen_description;
      }
      if (!prev.thumbnail_url && scene.thumbnail_url) {
        prev.thumbnail_url = scene.thumbnail_url;
      }
    } else {
      scenes.push({ ...scene });
    }
  }

  return scenes;
}

function buildMarkdown(
  title: string,
  generatedTitle: string,
  recordingUrl: string,
  summary: string,
  durationSeconds: number | null,
  scenes: Scene[]
): string {
  const lines: string[] = [];

  lines.push(`# ${title}`);
  lines.push("");
  lines.push(`- **Recording:** ${recordingUrl}`);
  if (durationSeconds != null) {
    lines.push(`- **Duration:** ${formatTimestamp(durationSeconds)}`);
  }
  lines.push(`- **Scenes:** ${scenes.length}`);
  lines.push("");

  lines.push("## Summary");
  lines.push("");
  lines.push(`### ${generatedTitle}`);
  lines.push("");
  lines.push(summary);
  lines.push("");

  if (scenes.some((s) => s.thumbnail_url)) {
    lines.push("## Thumbnail Preview");
    lines.push("");

    for (let i = 0; i < scenes.length; i += 4) {
      const row = scenes.slice(i, i + 4);
      lines.push(
        "| " +
          row
            .map(
              (s) =>
                `[${s.timestamp}](#scene-${i + row.indexOf(s) + 1})`
            )
            .join(" | ") +
          " |"
      );
      lines.push("| " + row.map(() => "---").join(" | ") + " |");
      lines.push(
        "| " +
          row
            .map((s) =>
              s.thumbnail_url
                ? `<img src="${s.thumbnail_url}" width="200" />`
                : "*no thumbnail*"
            )
            .join(" | ") +
          " |"
      );
      lines.push("");
    }
  }

  lines.push("## Scene Breakdown");
  lines.push("");

  scenes.forEach((scene, i) => {
    const anchor = `scene-${i + 1}`;
    lines.push(
      `### <a id="${anchor}"></a>Scene ${i + 1} — ${scene.timestamp}`
    );
    lines.push("");
    if (scene.thumbnail_url) {
      lines.push(`![Scene ${i + 1}](${scene.thumbnail_url})`);
      lines.push("");
    }
    lines.push(`**Description:** ${scene.description}`);
    lines.push("");
    if (scene.user_action !== "N/A") {
      lines.push(`**User Action:** ${scene.user_action}`);
      lines.push("");
    }
    if (scene.screen_description) {
      lines.push(`**Screen:** ${scene.screen_description}`);
      lines.push("");
    }
    lines.push("---");
    lines.push("");
  });

  return lines.join("\n");
}

/**
 * Shared analysis logic: describe + extract a recording and build the recap.
 */
async function analyzeRecording(
  recordingUrl: string,
  opts: { title?: string; maxSeconds: number }
): Promise<RecapOutput> {
  // Run describe and extract in parallel (each retries independently)
  const [describeResult, extractResult] = await Promise.all([
    describeRecording(recordingUrl, { maxSeconds: opts.maxSeconds }),
    extractSegmentLevel(recordingUrl, SCENE_SCHEMA, EXTRACT_PROMPT, {
      maxSeconds: opts.maxSeconds,
    }),
  ]);

  const data = describeResult.data as Record<string, unknown> | undefined;
  const summary = (data?.summary as string) ?? "";
  const durationSeconds =
    (describeResult.duration_seconds as number) ?? null;
  const generatedTitle = (data?.title as string) || "Session Recap";
  const title =
    opts.title || `Session Recap of ${trimUrl(recordingUrl)}`;

  const scenes = buildScenes(describeResult, extractResult);
  const markdown = buildMarkdown(
    title,
    generatedTitle,
    recordingUrl,
    summary,
    durationSeconds,
    scenes
  );

  return {
    title,
    recording_url: recordingUrl,
    summary,
    duration_seconds: durationSeconds,
    scene_count: scenes.length,
    scenes,
    markdown,
  };
}

/**
 * Analyze a recording and return a visual scene-by-scene recap.
 *
 * Pass any video URL — a Kernel session recording, a screen capture, or any video file.
 * Returns a structured recap with thumbnails, descriptions, user actions,
 * and a complete markdown document.
 *
 * Parameters:
 *   recording_url (required) — URL of the video to analyze
 *   title (optional) — custom title; defaults to "Session Recap of <url>"
 *   max_seconds (optional) — max length per scene in seconds (2-60, default 8)
 *
 * Invoke:
 *   kernel invoke ts-cloudglue-session-recap session-recap \
 *     --payload '{"recording_url": "https://..."}'
 *
 *   kernel invoke ts-cloudglue-session-recap session-recap \
 *     --payload '{"recording_url": "https://...", "title": "Login Flow Test", "max_seconds": 15}'
 */
app.action<RecapInput, RecapOutput>(
  "session-recap",
  async (_ctx: KernelContext, payload?: RecapInput): Promise<RecapOutput> => {
    if (!payload?.recording_url) {
      throw new Error("recording_url is required");
    }

    const maxSeconds = payload.max_seconds ?? 8;
    if (maxSeconds < 2 || maxSeconds > 60) {
      throw new Error("max_seconds must be between 2 and 60");
    }

    console.log(
      `Analyzing recording: ${payload.recording_url} (max_seconds: ${maxSeconds})`
    );

    return analyzeRecording(payload.recording_url, {
      title: payload.title,
      maxSeconds,
    });
  }
);

/**
 * Open a URL in a Kernel browser, record the session, then analyze the recording.
 *
 * Creates a cloud browser, navigates to the URL, waits 10 seconds to capture
 * the page, then stops the recording and passes it to Cloudglue for analysis.
 *
 * The `task` parameter is a placeholder for future agent integration (e.g.
 * connecting an AI computer-use agent to execute the task). Currently the
 * browser only navigates to the URL and waits.
 *
 * Parameters:
 *   url (required) — URL to open in the browser
 *   task (optional) — task description (placeholder, not executed)
 *   title (optional) — custom title; defaults to "Session Recap of <url>"
 *   max_seconds (optional) — max length per scene in seconds (2-60, default 8)
 *
 * Invoke:
 *   kernel invoke ts-cloudglue-session-recap browse-and-recap \
 *     --payload '{"url": "https://news.ycombinator.com"}'
 *
 *   kernel invoke ts-cloudglue-session-recap browse-and-recap \
 *     --payload '{"url": "https://example.com", "task": "Click sign in", "title": "Sign In Flow"}'
 */
app.action<BrowseAndRecapInput, BrowseAndRecapOutput>(
  "browse-and-recap",
  async (
    ctx: KernelContext,
    payload?: BrowseAndRecapInput
  ): Promise<BrowseAndRecapOutput> => {
    if (!payload?.url) throw new Error("url is required");

    const maxSeconds = payload.max_seconds ?? 8;
    if (maxSeconds < 2 || maxSeconds > 60) {
      throw new Error("max_seconds must be between 2 and 60");
    }

    // Create a Kernel browser
    const kernelBrowser = await kernel.browsers.create({
      invocation_id: ctx.invocation_id,
      stealth: true,
    });

    const sessionId = kernelBrowser.session_id;
    console.log("Live view:", kernelBrowser.browser_live_view_url);

    // Start replay recording
    const replay = await kernel.browsers.replays.start(sessionId);
    const replayId = replay.replay_id;
    console.log(`Replay recording started: ${replayId}`);

    // Connect Playwright to the Kernel browser
    const browser = await chromium.connectOverCDP(kernelBrowser.cdp_ws_url);
    const context = browser.contexts()[0] || (await browser.newContext());
    const page = context.pages()[0] || (await context.newPage());

    try {
      console.log(`Navigating to ${payload.url}...`);
      await page.goto(payload.url, { waitUntil: "networkidle" });

      // ---------------------------------------------------------------
      // DEMO ONLY: This just waits 10 seconds to capture the page.
      // Replace this section with your browser automation logic.
      // ---------------------------------------------------------------
      await sleep(10_000);
    } finally {
      await browser.close();
    }

    // Stop recording and wait for replay URL
    await sleep(2000);
    await kernel.browsers.replays.stop(replayId, { id: sessionId });
    console.log("Replay recording stopped. Waiting for processing...");

    let replayViewUrl = "";
    const maxWait = 60_000;
    const start = Date.now();
    while (Date.now() - start < maxWait) {
      const replays = await kernel.browsers.replays.list(sessionId);
      const found = replays.find((r) => r.replay_id === replayId);
      if (found?.replay_view_url) {
        replayViewUrl = found.replay_view_url;
        break;
      }
      await sleep(2000);
    }

    if (!replayViewUrl) {
      throw new Error("Replay URL not available after processing.");
    }

    console.log(`Replay view URL: ${replayViewUrl}`);
    console.log("Analyzing recording with Cloudglue...");

    const recap = await analyzeRecording(replayViewUrl, {
      title: payload.title || `Session Recap of ${trimUrl(payload.url)}`,
      maxSeconds,
    });

    await kernel.browsers.deleteByID(sessionId);

    return {
      ...recap,
      replay_view_url: replayViewUrl,
    };
  }
);
