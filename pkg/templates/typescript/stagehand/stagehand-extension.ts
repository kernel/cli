import { Kernel } from "@onkernel/sdk";
import { createReadStream } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// Stagehand v4 runs as a Chrome extension alongside the browser. When
// `localBrowser.connect` is called without an `extensionId`, Stagehand loads the
// extension into the running browser over CDP via `Extensions.loadUnpacked`,
// reading it from an absolute path on the *browser's* filesystem. Both the
// unpacked extension and its archive ship inside the installed
// @browserbasehq/stagehand package.
const stagehandDist = dirname(fileURLToPath(import.meta.resolve("@browserbasehq/stagehand")));
const STAGEHAND_EXTENSION_ZIP = join(stagehandDist, "assets/stagehand-extension.zip");
const STAGEHAND_EXTENSION_DIR = join(stagehandDist, "extension");

// Mirror the Stagehand extension onto the running browser's filesystem at the
// exact path Stagehand loads it from, so a subsequent
// `localBrowser.connect({ cdpUrl })` (no `extensionId`) finds it.
export async function loadStagehandExtension(kernel: Kernel, sessionId: string): Promise<void> {
  await kernel.browsers.fs.uploadZip(sessionId, {
    dest_path: STAGEHAND_EXTENSION_DIR,
    zip_file: createReadStream(STAGEHAND_EXTENSION_ZIP),
  });
}
