import { Kernel, type KernelContext } from "@onkernel/sdk";
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
  // Show domain + start of path
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

  // Deduplicate scenes with the same timestamp — merge extract data into the earlier scene
  const scenes: Scene[] = [];
  for (const scene of raw) {
    const prev = scenes[scenes.length - 1];
    if (prev && prev.timestamp === scene.timestamp) {
      // Merge: keep the one with a description, combine user actions
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

  // Header
  lines.push(`# ${title}`);
  lines.push("");
  lines.push(`- **Recording:** ${recordingUrl}`);
  if (durationSeconds != null) {
    lines.push(`- **Duration:** ${formatTimestamp(durationSeconds)}`);
  }
  lines.push(`- **Scenes:** ${scenes.length}`);
  lines.push("");

  // Summary with generated title as subheader
  lines.push("## Summary");
  lines.push("");
  lines.push(`### ${generatedTitle}`);
  lines.push("");
  lines.push(summary);
  lines.push("");

  // Thumbnail preview (4 columns)
  if (scenes.some((s) => s.thumbnail_url)) {
    lines.push("## Thumbnail Preview");
    lines.push("");

    for (let i = 0; i < scenes.length; i += 4) {
      const row = scenes.slice(i, i + 4);
      // Header row
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
      // Image row
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

  // Scene breakdown
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

    // Run describe and segment-level extract in parallel
    const [describeResult, extractResult] = await Promise.all([
      describeRecording(payload.recording_url, { maxSeconds }),
      extractSegmentLevel(
        payload.recording_url,
        SCENE_SCHEMA,
        EXTRACT_PROMPT,
        { maxSeconds }
      ),
    ]);

    const data = describeResult.data as Record<string, unknown> | undefined;
    const summary = (data?.summary as string) ?? "";
    const durationSeconds =
      (describeResult.duration_seconds as number) ?? null;

    const generatedTitle = (data?.title as string) || "Session Recap";

    // Title: user-provided, or "Session Recap of <trimmed url>"
    const title =
      payload.title || `Session Recap of ${trimUrl(payload.recording_url)}`;

    const scenes = buildScenes(describeResult, extractResult);
    const markdown = buildMarkdown(
      title,
      generatedTitle,
      payload.recording_url,
      summary,
      durationSeconds,
      scenes
    );

    return {
      title,
      recording_url: payload.recording_url,
      summary,
      duration_seconds: durationSeconds,
      scene_count: scenes.length,
      scenes,
      markdown,
    };
  }
);
