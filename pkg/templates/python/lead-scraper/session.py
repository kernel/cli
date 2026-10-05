"""
Kernel Browser Session Manager.

Provides an async context manager for managing Kernel browser lifecycle
with optional video replay recording.
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

    Creates a browser session on entry and cleans it up on exit.
    Optionally records a video replay of the entire session.
    Provides session_id to computer tools.

    Usage:
        async with KernelBrowserSession(record_replay=True) as session:
            # Use session.session_id and session.kernel for operations
            pass
        # Browser is automatically cleaned up, replay URL available in session.replay_view_url
    """

    stealth: bool = True
    timeout_seconds: int = 300

    # Replay recording options
    record_replay: bool = False
    replay_grace_period: float = 5.0  # Seconds to wait before stopping replay

    # Set after browser creation
    session_id: Optional[str] = field(default=None, init=False)
    live_view_url: Optional[str] = field(default=None, init=False)
    replay_id: Optional[str] = field(default=None, init=False)
    replay_view_url: Optional[str] = field(default=None, init=False)
    _kernel: Optional[Kernel] = field(default=None, init=False)

    async def __aenter__(self) -> "KernelBrowserSession":
        """Create a Kernel browser session and optionally start recording."""
        self._kernel = Kernel()

        # Create browser with specified settings
        browser = self._kernel.browsers.create(
            stealth=self.stealth,
            timeout_seconds=self.timeout_seconds,
            viewport={
                "width": 1024,
                "height": 768,
                "refresh_rate": 60,
            },
        )

        self.session_id = browser.session_id
        self.live_view_url = browser.browser_live_view_url

        print(f"Kernel browser created: {self.session_id}")
        print(f"Live view URL: {self.live_view_url}")

        # Start replay recording if enabled
        if self.record_replay:
            try:
                await self._start_replay()
            except Exception as e:
                print(f"Warning: Failed to start replay recording: {e}")
                print("Continuing without replay recording.")

        return self

    async def _start_replay(self) -> None:
        """Start recording a replay of the browser session."""
        if not self._kernel or not self.session_id:
            return

        print("Starting replay recording...")
        replay = self._kernel.browsers.replays.start(self.session_id)
        self.replay_id = replay.replay_id
        print(f"Replay recording started: {self.replay_id}")

    async def _stop_and_get_replay_url(self) -> None:
        """Stop recording and get the replay URL."""
        if not self._kernel or not self.session_id or not self.replay_id:
            return

        print("Stopping replay recording...")
        self._kernel.browsers.replays.stop(
            replay_id=self.replay_id,
            id=self.session_id,
        )
        print("Replay recording stopped. Processing video...")

        # Wait a moment for processing
        await asyncio.sleep(2)

        # Poll for replay to be ready (with timeout)
        max_wait = 60  # seconds
        start_time = time.time()
        replay_ready = False

        while time.time() - start_time < max_wait:
            try:
                replays = self._kernel.browsers.replays.list(self.session_id)
                for replay in replays:
                    if replay.replay_id == self.replay_id:
                        self.replay_view_url = replay.replay_view_url
                        replay_ready = True
                        break
                if replay_ready:
                    break
            except Exception:
                pass
            await asyncio.sleep(1)

        if not replay_ready:
            print("Warning: Replay may still be processing")
        elif self.replay_view_url:
            print(f"Replay view URL: {self.replay_view_url}")

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        """Stop recording and delete the browser session."""
        if self._kernel and self.session_id:
            try:
                # Stop replay if recording was enabled
                if self.record_replay and self.replay_id:
                    # Wait grace period before stopping to capture final state
                    if self.replay_grace_period > 0:
                        print(f"Waiting {self.replay_grace_period}s grace period...")
                        await asyncio.sleep(self.replay_grace_period)
                    await self._stop_and_get_replay_url()
            finally:
                print(f"Destroying browser session: {self.session_id}")
                self._kernel.browsers.delete_by_id(self.session_id)
                print("Browser session destroyed.")

        self._kernel = None

    @property
    def kernel(self) -> Kernel:
        """Get the Kernel client instance."""
        if self._kernel is None:
            raise RuntimeError("Session not initialized. Use async with context.")
        return self._kernel
