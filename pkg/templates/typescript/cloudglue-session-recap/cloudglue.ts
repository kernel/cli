import { Cloudglue } from "@cloudglue/cloudglue-js";
import type { SegmentationConfig } from "@cloudglue/cloudglue-js";

const CLOUDGLUE_API_KEY = process.env.CLOUDGLUE_API_KEY;

if (!CLOUDGLUE_API_KEY) {
  throw new Error("CLOUDGLUE_API_KEY is not set");
}

const client = new Cloudglue({ apiKey: CLOUDGLUE_API_KEY });

export interface SegmentationOptions {
  maxSeconds?: number;
}

/** Build segmentation config with configurable max_seconds. */
function buildSegmentation(opts?: SegmentationOptions): SegmentationConfig {
  return {
    strategy: "shot-detector",
    shot_detector_config: {
      detector: "adaptive",
      min_seconds: 1,
      max_seconds: opts?.maxSeconds ?? 8,
      fill_gaps: true,
    },
  };
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

/**
 * Retry a function up to maxRetries times with backoff.
 */
async function withRetry<T>(
  fn: () => Promise<T>,
  label: string,
  maxRetries = 2,
  backoffMs = 20_000
): Promise<T> {
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : String(err);
      console.error(
        `${label}: attempt ${attempt}/${maxRetries} failed: ${msg}`
      );
      if (attempt < maxRetries) {
        console.log(`${label}: waiting ${backoffMs / 1000}s before retry...`);
        await sleep(backoffMs);
        console.log(`${label}: retrying (attempt ${attempt + 1})...`);
      } else {
        console.error(`${label}: all ${maxRetries} attempts exhausted.`);
        throw err;
      }
    }
  }
  // Unreachable, but TypeScript needs it
  throw new Error(`${label} failed`);
}

/**
 * Poll a describe job until complete, fetching full data with thumbnails.
 */
async function pollDescribe(
  jobId: string,
  intervalMs = 5000
): Promise<Record<string, unknown>> {
  while (true) {
    const job = await client.describe.getDescribe(jobId, {
      include_thumbnails: true,
      include_shots: true,
    });
    if (job.status === "completed") return job as Record<string, unknown>;
    if (job.status === "failed" || job.status === "not_applicable") {
      throw new Error(`Describe job ${jobId} failed: ${JSON.stringify(job)}`);
    }
    await new Promise((r) => setTimeout(r, intervalMs));
  }
}

/**
 * Poll an extract job until complete, fetching thumbnails.
 */
async function pollExtract(
  jobId: string,
  intervalMs = 5000
): Promise<Record<string, unknown>> {
  while (true) {
    const job = await client.extract.getExtract(jobId, {
      include_thumbnails: true,
      include_shots: true,
    });
    if (job.status === "completed") return job as Record<string, unknown>;
    if (job.status === "failed" || job.status === "not_applicable") {
      throw new Error(`Extract job ${jobId} failed: ${JSON.stringify(job)}`);
    }
    await new Promise((r) => setTimeout(r, intervalMs));
  }
}

/**
 * Describe a video recording via Cloudglue.
 * Retries up to 2 times with 20s backoff if the video isn't ready yet.
 */
export async function describeRecording(
  url: string,
  opts?: SegmentationOptions
): Promise<Record<string, unknown>> {
  return withRetry(async () => {
    const job = await client.describe.createDescribe(url, {
      enable_visual_scene_description: true,
      enable_scene_text: true,
      enable_speech: true,
      enable_summary: true,
      segmentation_config: buildSegmentation(opts),
      include_shots: true,
    });

    console.log(`Describe job created: ${job.job_id}`);
    return await pollDescribe(job.job_id);
  }, "Describe");
}

/**
 * Extract structured data from a video at the segment level.
 * Retries up to 2 times with 20s backoff if the video isn't ready yet.
 */
export async function extractSegmentLevel(
  url: string,
  schema: Record<string, unknown>,
  prompt: string,
  opts?: SegmentationOptions
): Promise<Record<string, unknown>> {
  return withRetry(async () => {
    const job = await client.extract.createExtract(url, {
      url,
      schema,
      prompt,
      enable_segment_level_entities: true,
      segmentation_config: buildSegmentation(opts),
      include_shots: true,
    });

    console.log(`Extract job created: ${job.job_id} (segment-level)`);
    return await pollExtract(job.job_id);
  }, "Extract");
}
