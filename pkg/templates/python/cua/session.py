"""
Kernel Browser Session Manager.

Async context manager for browser lifecycle with optional replay recording.
Shared across all CUA providers.
"""

import asyncio
import time
from dataclasses import dataclass, field
from typing import Optional

from kernel import Kernel


@dataclass
class KernelBrowserSession:
    """
    Manages Kernel browser lifecycle as an async context manager.

    Usage:
        async with KernelBrowserSession(stealth=True) as session:
            # Use session.session_id and session.kernel
            pass
    """

    stealth: bool = True
    timeout_seconds: int = 300
    viewport_width: int = 1280
    viewport_height: int = 800
    record_replay: bool = False
    replay_grace_period: float = 5.0
    invocation_id: Optional[str] = None

    session_id: Optional[str] = field(default=None, init=False)
    live_view_url: Optional[str] = field(default=None, init=False)
    replay_id: Optional[str] = field(default=None, init=False)
    replay_view_url: Optional[str] = field(default=None, init=False)
    _kernel: Optional[Kernel] = field(default=None, init=False)

    async def __aenter__(self) -> "KernelBrowserSession":
        self._kernel = Kernel()

        browser = self._kernel.browsers.create(
            invocation_id=self.invocation_id,
            stealth=self.stealth,
            timeout_seconds=self.timeout_seconds,
            viewport={
                "width": self.viewport_width,
                "height": self.viewport_height,
            },
        )

        self.session_id = browser.session_id
        self.live_view_url = browser.browser_live_view_url

        print(f"Kernel browser created: {self.session_id}")
        print(f"Live view URL: {self.live_view_url}")

        if self.record_replay:
            try:
                replay = self._kernel.browsers.replays.start(self.session_id)
                self.replay_id = replay.replay_id
                print(f"Replay recording started: {self.replay_id}")
            except Exception as e:
                print(f"Warning: Failed to start replay recording: {e}")

        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        if self._kernel and self.session_id:
            try:
                if self.record_replay and self.replay_id:
                    if self.replay_grace_period > 0:
                        await asyncio.sleep(self.replay_grace_period)
                    await self._stop_replay()
            finally:
                print(f"Destroying browser session: {self.session_id}")
                self._kernel.browsers.delete_by_id(self.session_id)
                print("Browser session destroyed.")
        self._kernel = None

    async def _stop_replay(self) -> None:
        if not self._kernel or not self.session_id or not self.replay_id:
            return

        self._kernel.browsers.replays.stop(
            replay_id=self.replay_id, id=self.session_id,
        )
        await asyncio.sleep(2)

        max_wait = 60
        start_time = time.time()
        while time.time() - start_time < max_wait:
            try:
                replays = self._kernel.browsers.replays.list(self.session_id)
                for replay in replays:
                    if replay.replay_id == self.replay_id:
                        self.replay_view_url = replay.replay_view_url
                        return
            except Exception:
                pass
            await asyncio.sleep(1)

    @property
    def kernel(self) -> Kernel:
        if self._kernel is None:
            raise RuntimeError("Session not initialized. Use async with context.")
        return self._kernel
