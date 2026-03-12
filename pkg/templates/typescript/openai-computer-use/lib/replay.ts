import type { Kernel } from '@onkernel/sdk';
import type { AgentEvent } from './log-events';

const DEFAULT_REPLAY_GRACE_MS = 5000;
const REPLAY_PROCESSING_DELAY_MS = 2000;
const REPLAY_POLL_TIMEOUT_MS = 60000;
const REPLAY_POLL_INTERVAL_MS = 1000;

type EventLogger = (event: AgentEvent) => void;

export interface ReplayState {
  enabled: boolean;
  replayId: string | null;
  replayViewUrl: string | null;
}

export async function maybeStartReplay(
  kernel: Kernel,
  sessionId: string,
  opts?: {
    enabled?: boolean;
    onEvent?: EventLogger;
  },
): Promise<ReplayState> {
  const enabled = opts?.enabled ?? false;
  const state: ReplayState = {
    enabled,
    replayId: null,
    replayViewUrl: null,
  };

  if (!enabled) return state;

  const startedAtMs = Date.now();
  opts?.onEvent?.({ event: 'backend', data: { op: 'browsers.replays.start' } });
  try {
    const replay = await kernel.browsers.replays.start(sessionId);
    state.replayId = replay.replay_id ?? null;
    opts?.onEvent?.({
      event: 'backend',
      data: {
        op: 'browsers.replays.start.done',
        detail: state.replayId ?? '',
        elapsed_ms: Date.now() - startedAtMs,
      },
    });
  } catch (error) {
    console.warn(`Warning: failed to start replay recording: ${String(error)}`);
    console.warn('Continuing without replay recording.');
    state.enabled = false;
  }

  return state;
}

export async function maybeStopReplay(
  kernel: Kernel,
  sessionId: string,
  replay: ReplayState,
  opts?: {
    onEvent?: EventLogger;
    gracePeriodMs?: number;
  },
): Promise<string | null> {
  if (!replay.enabled || !replay.replayId) return replay.replayViewUrl;

  const gracePeriodMs = opts?.gracePeriodMs ?? DEFAULT_REPLAY_GRACE_MS;
  if (gracePeriodMs > 0) {
    await sleep(gracePeriodMs);
  }

  const startedAtMs = Date.now();
  opts?.onEvent?.({ event: 'backend', data: { op: 'browsers.replays.stop' } });
  try {
    await kernel.browsers.replays.stop(replay.replayId, { id: sessionId });
    await sleep(REPLAY_PROCESSING_DELAY_MS);

    const pollStartedAt = Date.now();
    while (Date.now() - pollStartedAt < REPLAY_POLL_TIMEOUT_MS) {
      try {
        const replays = await kernel.browsers.replays.list(sessionId);
        const matchingReplay = replays.find((item) => item.replay_id === replay.replayId);
        if (matchingReplay) {
          replay.replayViewUrl = matchingReplay.replay_view_url ?? null;
          break;
        }
      } catch {
        // Ignore transient polling errors while the replay finishes processing.
      }
      await sleep(REPLAY_POLL_INTERVAL_MS);
    }

    opts?.onEvent?.({
      event: 'backend',
      data: {
        op: 'browsers.replays.stop.done',
        detail: replay.replayViewUrl ?? replay.replayId ?? '',
        elapsed_ms: Date.now() - startedAtMs,
      },
    });

    if (!replay.replayViewUrl) {
      console.warn('Warning: replay may still be processing.');
    }
  } catch (error) {
    console.warn(`Warning: failed to stop replay recording cleanly: ${String(error)}`);
  }

  return replay.replayViewUrl;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
