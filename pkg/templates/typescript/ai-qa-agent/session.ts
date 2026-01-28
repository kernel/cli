/**
 * Kernel Browser Session Manager for QA Agent.
 */

import type { Kernel } from '@onkernel/sdk';

export interface SessionOptions {
  stealth?: boolean;
  timeoutSeconds?: number;
  recordReplay?: boolean;
}

export interface SessionInfo {
  sessionId: string;
  liveViewUrl: string;
  replayViewUrl?: string;
}

const DEFAULT_OPTIONS: Required<SessionOptions> = {
  stealth: true,
  timeoutSeconds: 300,
  recordReplay: false,
};

export class KernelBrowserSession {
  private kernel: Kernel;
  private options: Required<SessionOptions>;
  
  private _sessionId: string | null = null;
  private _liveViewUrl: string | null = null;
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

  get info(): SessionInfo {
    return {
      sessionId: this.sessionId,
      liveViewUrl: this._liveViewUrl || '',
      replayViewUrl: this._replayViewUrl || undefined,
    };
  }

  async start(): Promise<SessionInfo> {
    const browser = await this.kernel.browsers.create({
      stealth: this.options.stealth,
      timeout_seconds: this.options.timeoutSeconds,
      viewport: { width: 1440, height: 900, refresh_rate: 25 },
    });

    this._sessionId = browser.session_id ?? null;
    this._liveViewUrl = browser.browser_live_view_url ?? null;

    console.log(`Browser session created: ${this._sessionId}`);
    console.log(`Live view URL: ${this._liveViewUrl}`);

    if (this.options.recordReplay) {
      try {
        const replay = await this.kernel.browsers.replays.start(this._sessionId);
        this._replayId = replay.replay_id ?? null;
        console.log(`Replay recording started: ${this._replayId}`);
      } catch (error) {
        console.warn(`Warning: Failed to start replay recording: ${error}`);
      }
    }

    return this.info;
  }

  async stop(): Promise<SessionInfo> {
    const info = this.info;

    if (this._sessionId) {
      try {
        if (this.options.recordReplay && this._replayId) {
          await this.sleep(3000);
          await this.kernel.browsers.replays.stop(this._replayId, { id: this._sessionId });
          await this.sleep(2000);
          
          const replays = await this.kernel.browsers.replays.list(this._sessionId);
          for (const replay of replays) {
            if (replay.replay_id === this._replayId) {
              this._replayViewUrl = replay.replay_view_url ? replay.replay_view_url : null;
              info.replayViewUrl = this._replayViewUrl ? this._replayViewUrl : undefined;
              break;
            }
          }
        }
      } finally {
        console.log(`Destroying browser session: ${this._sessionId}`);
        await this.kernel.browsers.deleteByID(this._sessionId);
      }
    }

    this._sessionId = null;
    this._liveViewUrl = null;
    this._replayId = null;

    return info;
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
