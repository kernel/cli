import time
from dataclasses import dataclass, field
from typing import Optional

from kernel import Kernel


@dataclass
class KernelBrowserSession:
    kernel: Kernel
    stealth: bool = True
    timeout_seconds: int = 300
    width: int = 1024
    height: int = 768
    invocation_id: Optional[str] = None
    record_replay: bool = False
    replay_grace_period: float = 5.0
    session_id: Optional[str] = field(default=None, init=False)
    live_view_url: Optional[str] = field(default=None, init=False)
    replay_id: Optional[str] = field(default=None, init=False)
    replay_view_url: Optional[str] = field(default=None, init=False)

    def __enter__(self) -> "KernelBrowserSession":
        browser = self.kernel.browsers.create(
            stealth=self.stealth,
            timeout_seconds=self.timeout_seconds,
            invocation_id=self.invocation_id,
            viewport={
                "width": self.width,
                "height": self.height,
                "refresh_rate": 60,
            },
        )

        self.session_id = browser.session_id
        self.live_view_url = browser.browser_live_view_url

        print(f"Kernel browser created: {self.session_id}")
        print(f"Live view URL: {self.live_view_url}")

        if self.record_replay:
            try:
                self._start_replay()
            except Exception as e:
                print(f"Warning: Failed to start replay recording: {e}")
                print("Continuing without replay recording.")

        return self

    def _start_replay(self) -> None:
        if not self.session_id:
            return

        print("Starting replay recording...")
        replay = self.kernel.browsers.replays.start(self.session_id)
        self.replay_id = replay.replay_id
        print(f"Replay recording started: {self.replay_id}")

    def _stop_and_get_replay_url(self) -> None:
        if not self.session_id or not self.replay_id:
            return

        print("Stopping replay recording...")
        self.kernel.browsers.replays.stop(
            replay_id=self.replay_id,
            id=self.session_id,
        )
        print("Replay recording stopped. Processing video...")
        time.sleep(2)

        max_wait = 60
        start_time = time.time()
        replay_ready = False

        while time.time() - start_time < max_wait:
            try:
                replays = self.kernel.browsers.replays.list(self.session_id)
                for replay in replays:
                    if replay.replay_id == self.replay_id:
                        self.replay_view_url = replay.replay_view_url
                        replay_ready = True
                        break
                if replay_ready:
                    break
            except Exception:
                pass
            time.sleep(1)

        if not replay_ready:
            print("Warning: Replay may still be processing")
        elif self.replay_view_url:
            print(f"Replay view URL: {self.replay_view_url}")

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        if self.session_id:
            try:
                if self.record_replay and self.replay_id:
                    if self.replay_grace_period > 0:
                        print(f"Waiting {self.replay_grace_period}s grace period...")
                        time.sleep(self.replay_grace_period)
                    self._stop_and_get_replay_url()
            finally:
                print(f"Destroying browser session: {self.session_id}")
                self.kernel.browsers.delete_by_id(self.session_id)
                print("Browser session destroyed.")
