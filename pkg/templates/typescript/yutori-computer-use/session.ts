/**
 * Kernel Browser Session Manager.
 * 
 * Provides a class for managing Kernel browser lifecycle
 * with optional video replay recording.
 */

import type { Kernel } from '@onkernel/sdk';

export interface SessionOptions {
  /** Enable stealth mode to avoid bot detection */
  stealth?: boolean;
  /** Browser session timeout in seconds */
  timeoutSeconds?: number;
  /** Enable replay recording (requires paid plan) */
  recordReplay?: boolean;
  /** Grace period in seconds before stopping replay */
  replayGracePeriod?: number;
  /** Viewport width (default: 1200, closest to Yutori's 1280 recommendation) */
  viewportWidth?: number;
  /** Viewport height (default: 800 per Yutori recommendation) */
  viewportHeight?: number;
}

export interface SessionInfo {
  sessionId: string;
  liveViewUrl: string;
  cdpWsUrl: string;
  replayId?: string;
  replayViewUrl?: string;
  viewportWidth: number;
  viewportHeight: number;
}

const DEFAULT_OPTIONS: Required<SessionOptions> = {
  stealth: true,
  timeoutSeconds: 300,
  recordReplay: false,
  replayGracePeriod: 5.0,
  viewportWidth: 1200,
  viewportHeight: 800,
};

/**
 * Manages Kernel browser lifecycle with optional replay recording.
 * 
 * Usage:
 * ```typescript
 * const session = new KernelBrowserSession(kernel, options);
 * await session.start();
 * try {
 *   // Use session.sessionId for computer controls
 * } finally {
 *   await session.stop();
 * }
 * ```
 */
export class KernelBrowserSession {
  private kernel: Kernel;
  private options: Required<SessionOptions>;
  
  // Session state
  private _sessionId: string | null = null;
  private _liveViewUrl: string | null = null;
  private _cdpWsUrl: string | null = null;
  private _replayId: string | null = null;
  private _replayViewUrl: string | null = null;

  constructor(kernel: Kernel, options: SessionOptions = {}) {
    this.kernel = kernel;
    this.options = { ...DEFAULT_OPTIONS, ...options };
  }

  get sessionId(): string {
    if (!this._sessionId) {
      throw new Error('Session not started. Call start() first.');
    }
    return this._sessionId;
  }

  get liveViewUrl(): string | null {
    return this._liveViewUrl;
  }

  get cdpWsUrl(): string | null {
    return this._cdpWsUrl;
  }

  get replayViewUrl(): string | null {
    return this._replayViewUrl;
  }

  get viewportWidth(): number {
    return this.options.viewportWidth;
  }

  get viewportHeight(): number {
    return this.options.viewportHeight;
  }

  get info(): SessionInfo {
    return {
      sessionId: this.sessionId,
      liveViewUrl: this._liveViewUrl || '',
      cdpWsUrl: this._cdpWsUrl || '',
      replayId: this._replayId || undefined,
      replayViewUrl: this._replayViewUrl || undefined,
      viewportWidth: this.options.viewportWidth,
      viewportHeight: this.options.viewportHeight,
    };
  }

  async start(): Promise<SessionInfo> {
    const browser = await this.kernel.browsers.create({
      stealth: this.options.stealth,
      timeout_seconds: this.options.timeoutSeconds,
      viewport: {
        width: this.options.viewportWidth,
        height: this.options.viewportHeight,
      },
    });

    this._sessionId = browser.session_id;
    this._liveViewUrl = browser.browser_live_view_url;
    this._cdpWsUrl = browser.cdp_ws_url;

    console.log(`Kernel browser created: ${this._sessionId}`);
    console.log(`Live view URL: ${this._liveViewUrl}`);

    // Start replay recording if enabled
    if (this.options.recordReplay) {
      try {
        await this.startReplay();
      } catch (error) {
        console.warn(`Warning: Failed to start replay recording: ${error}`);
        console.warn('Continuing without replay recording.');
      }
    }

    return this.info;
  }

  private async startReplay(): Promise<void> {
    if (!this._sessionId) {
      return;
    }

    console.log('Starting replay recording...');
    const replay = await this.kernel.browsers.replays.start(this._sessionId);
    this._replayId = replay.replay_id;
    console.log(`Replay recording started: ${this._replayId}`);
  }

  private async stopReplay(): Promise<void> {
    if (!this._sessionId || !this._replayId) {
      return;
    }

    console.log('Stopping replay recording...');
    await this.kernel.browsers.replays.stop(this._replayId, {
      id: this._sessionId,
    });
    console.log('Replay recording stopped. Processing video...');

    // Wait a moment for processing
    await this.sleep(2000);

    // Poll for replay to be ready (with timeout)
    const maxWait = 60000; // 60 seconds
    const startTime = Date.now();
    let replayReady = false;

    while (Date.now() - startTime < maxWait) {
      try {
        const replays = await this.kernel.browsers.replays.list(this._sessionId);
        for (const replay of replays) {
          if (replay.replay_id === this._replayId) {
            this._replayViewUrl = replay.replay_view_url;
            replayReady = true;
            break;
          }
        }
        if (replayReady) {
          break;
        }
      } catch {
        // Ignore errors while polling
      }
      await this.sleep(1000);
    }

    if (!replayReady) {
      console.log('Warning: Replay may still be processing');
    } else if (this._replayViewUrl) {
      console.log(`Replay view URL: ${this._replayViewUrl}`);
    }
  }

  async stop(): Promise<SessionInfo> {
    const info = this.info;

    if (this._sessionId) {
      try {
        // Stop replay if recording was enabled
        if (this.options.recordReplay && this._replayId) {
          // Wait grace period before stopping to capture final state
          if (this.options.replayGracePeriod > 0) {
            console.log(`Waiting ${this.options.replayGracePeriod}s grace period...`);
            await this.sleep(this.options.replayGracePeriod * 1000);
          }
          await this.stopReplay();
          info.replayViewUrl = this._replayViewUrl || undefined;
        }
      } finally {
        console.log(`Destroying browser session: ${this._sessionId}`);
        await this.kernel.browsers.deleteByID(this._sessionId);
        console.log('Browser session destroyed.');
      }
    }

    // Reset state
    this._sessionId = null;
    this._liveViewUrl = null;
    this._cdpWsUrl = null;
    this._replayId = null;
    this._replayViewUrl = null;

    return info;
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
