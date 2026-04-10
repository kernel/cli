import datetime
import time
from dataclasses import dataclass
from typing import Callable

from kernel import Kernel

DEFAULT_REPLAY_GRACE_SECONDS = 5.0
REPLAY_PROCESSING_DELAY_SECONDS = 2.0
REPLAY_POLL_TIMEOUT_SECONDS = 60.0
REPLAY_POLL_INTERVAL_SECONDS = 1.0


@dataclass
class ReplayState:
    enabled: bool
    replay_id: str | None = None
    replay_view_url: str | None = None


def maybe_start_replay(
    client: Kernel,
    session_id: str,
    enabled: bool = False,
    on_event: Callable[[dict], None] | None = None,
) -> ReplayState:
    state = ReplayState(enabled=enabled)
    if not enabled:
        return state

    started_at = datetime.datetime.now()
    if on_event:
        on_event({"event": "backend", "data": {"op": "browsers.replays.start"}})

    try:
        replay = client.browsers.replays.start(session_id)
        state.replay_id = replay.replay_id
        if on_event:
            on_event(
                {
                    "event": "backend",
                    "data": {
                        "op": "browsers.replays.start.done",
                        "detail": state.replay_id or "",
                        "elapsed_ms": int(
                            (datetime.datetime.now() - started_at).total_seconds() * 1000
                        ),
                    },
                }
            )
    except Exception as exc:
        print(f"Warning: failed to start replay recording: {exc}")
        print("Continuing without replay recording.")
        state.enabled = False

    return state


def maybe_stop_replay(
    client: Kernel,
    session_id: str,
    replay: ReplayState,
    on_event: Callable[[dict], None] | None = None,
    grace_period_seconds: float = DEFAULT_REPLAY_GRACE_SECONDS,
) -> str | None:
    if not replay.enabled or not replay.replay_id:
        return replay.replay_view_url

    if grace_period_seconds > 0:
        time.sleep(grace_period_seconds)

    started_at = datetime.datetime.now()
    if on_event:
        on_event({"event": "backend", "data": {"op": "browsers.replays.stop"}})

    try:
        client.browsers.replays.stop(replay_id=replay.replay_id, id=session_id)
        time.sleep(REPLAY_PROCESSING_DELAY_SECONDS)

        deadline = time.time() + REPLAY_POLL_TIMEOUT_SECONDS
        while time.time() < deadline:
            try:
                replays = client.browsers.replays.list(session_id)
                for replay_item in replays:
                    if replay_item.replay_id == replay.replay_id:
                        replay.replay_view_url = replay_item.replay_view_url
                        break
                if replay.replay_view_url:
                    break
            except Exception:
                pass

            time.sleep(REPLAY_POLL_INTERVAL_SECONDS)

        if on_event:
            on_event(
                {
                    "event": "backend",
                    "data": {
                        "op": "browsers.replays.stop.done",
                        "detail": replay.replay_view_url or replay.replay_id or "",
                        "elapsed_ms": int(
                            (datetime.datetime.now() - started_at).total_seconds() * 1000
                        ),
                    },
                }
            )

        if not replay.replay_view_url:
            print("Warning: replay may still be processing")
    except Exception as exc:
        print(f"Warning: failed to stop replay recording cleanly: {exc}")

    return replay.replay_view_url
