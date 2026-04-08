/**
 * Kernel Browser Session Manager.
 *
 * Manages browser lifecycle with optional video replay recording.
 */

import type { Kernel } from '@onkernel/sdk';

export interface SessionOptions {
  invocationId?: string;
  stealth?: boolean;
  timeoutSeconds?: number;
  recordReplay?: boolean;
  replayGracePeriod?: number;
  viewportWidth?: number;
  viewportHeight?: number;
}

export interface SessionInfo {
  sessionId: string;
  liveViewUrl: string;
  replayId?: string;
  replayViewUrl?: string;
  viewportWidth: number;
  viewportHeight: number;
}

const DEFAULTS = {
  stealth: true,
  timeoutSeconds: 300,
  recordReplay: false,
  replayGracePeriod: 5.0,
  viewportWidth: 1280,
  viewportHeight: 800,
};

export class KernelBrowserSession {
  private kernel: Kernel;
  private opts: Required<Omit<SessionOptions, 'invocationId'>> & Pick<SessionOptions, 'invocationId'>;

  private _sessionId: string | null = null;
  private _liveViewUrl: string | null = null;
  private _replayId: string | null = null;
  private _replayViewUrl: string | null = null;

  constructor(kernel: Kernel, options: SessionOptions = {}) {
    this.kernel = kernel;
    this.opts = { ...DEFAULTS, ...options };
  }

  get sessionId(): string {
    if (!this._sessionId) throw new Error('Session not started. Call start() first.');
    return this._sessionId;
  }

  get liveViewUrl(): string | null { return this._liveViewUrl; }
  get replayViewUrl(): string | null { return this._replayViewUrl; }
  get viewportWidth(): number { return this.opts.viewportWidth; }
  get viewportHeight(): number { return this.opts.viewportHeight; }

  get info(): SessionInfo {
    return {
      sessionId: this.sessionId,
      liveViewUrl: this._liveViewUrl || '',
      replayId: this._replayId || undefined,
      replayViewUrl: this._replayViewUrl || undefined,
      viewportWidth: this.opts.viewportWidth,
      viewportHeight: this.opts.viewportHeight,
    };
  }

  async start(): Promise<SessionInfo> {
    const browser = await this.kernel.browsers.create({
      invocation_id: this.opts.invocationId,
      stealth: this.opts.stealth,
      timeout_seconds: this.opts.timeoutSeconds,
      viewport: { width: this.opts.viewportWidth, height: this.opts.viewportHeight },
    });

    this._sessionId = browser.session_id;
    this._liveViewUrl = browser.browser_live_view_url ?? null;

    console.log(`Browser session: ${this._sessionId}`);
    console.log(`Live view: ${this._liveViewUrl}`);

    if (this.opts.recordReplay) {
      try {
        const replay = await this.kernel.browsers.replays.start(this._sessionId);
        this._replayId = replay.replay_id;
        console.log(`Replay recording started: ${this._replayId}`);
      } catch (error) {
        console.warn(`Failed to start replay: ${error}`);
      }
    }

    return this.info;
  }

  async stop(): Promise<SessionInfo> {
    const info = this.info;

    if (this._sessionId) {
      try {
        if (this.opts.recordReplay && this._replayId) {
          if (this.opts.replayGracePeriod > 0) {
            await sleep(this.opts.replayGracePeriod * 1000);
          }
          await this.stopReplay();
          info.replayViewUrl = this._replayViewUrl || undefined;
        }
      } finally {
        console.log(`Destroying browser session: ${this._sessionId}`);
        await this.kernel.browsers.deleteByID(this._sessionId);
      }
    }

    this._sessionId = null;
    this._liveViewUrl = null;
    this._replayId = null;
    this._replayViewUrl = null;

    return info;
  }

  private async stopReplay(): Promise<void> {
    if (!this._sessionId || !this._replayId) return;

    await this.kernel.browsers.replays.stop(this._replayId, { id: this._sessionId });
    await sleep(2000);

    // Poll for replay URL
    const deadline = Date.now() + 60_000;
    while (Date.now() < deadline) {
      try {
        const replays = await this.kernel.browsers.replays.list(this._sessionId!);
        const match = replays.find(r => r.replay_id === this._replayId);
        if (match) {
          this._replayViewUrl = match.replay_view_url ?? null;
          if (this._replayViewUrl) {
            console.log(`Replay URL: ${this._replayViewUrl}`);
          }
          return;
        }
      } catch { /* polling */ }
      await sleep(1000);
    }
    console.warn('Replay may still be processing.');
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}
