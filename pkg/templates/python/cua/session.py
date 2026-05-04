"""Kernel Browser Session Manager with optional replay recording."""

from __future__ import annotations

import asyncio
import time
from dataclasses import dataclass, field
from typing import Any

from kernel import Kernel


@dataclass
class SessionOptions:
    invocation_id: str | None = None
    stealth: bool = True
    timeout_seconds: int = 300
    record_replay: bool = False
    replay_grace_period: float = 5.0
    viewport_width: int = 1280
    viewport_height: int = 800
    proxy_id: str | None = None
    profile: dict | None = None
    extensions: list[dict] | None = None


@dataclass
class SessionInfo:
    session_id: str = ""
    live_view_url: str = ""
    replay_id: str | None = None
    replay_view_url: str | None = None
    viewport_width: int = 1280
    viewport_height: int = 800


class KernelBrowserSession:
    """Manages Kernel browser lifecycle with optional replay recording."""

    def __init__(self, kernel: Kernel, options: SessionOptions | None = None) -> None:
        self.kernel = kernel
        self.opts = options or SessionOptions()
        self._session_id: str | None = None
        self._live_view_url: str | None = None
        self._replay_id: str | None = None
        self._replay_view_url: str | None = None

    @property
    def session_id(self) -> str:
        if not self._session_id:
            raise RuntimeError("Session not started. Call start() first.")
        return self._session_id

    @property
    def live_view_url(self) -> str | None:
        return self._live_view_url

    @property
    def replay_view_url(self) -> str | None:
        return self._replay_view_url

    @property
    def info(self) -> SessionInfo:
        return SessionInfo(
            session_id=self.session_id,
            live_view_url=self._live_view_url or "",
            replay_id=self._replay_id,
            replay_view_url=self._replay_view_url,
            viewport_width=self.opts.viewport_width,
            viewport_height=self.opts.viewport_height,
        )

    async def start(self) -> SessionInfo:
        create_kwargs: dict = {
            "invocation_id": self.opts.invocation_id,
            "stealth": self.opts.stealth,
            "timeout_seconds": self.opts.timeout_seconds,
            "viewport": {
                "width": self.opts.viewport_width,
                "height": self.opts.viewport_height,
            },
        }
        if self.opts.proxy_id:
            create_kwargs["proxy_id"] = self.opts.proxy_id
        if self.opts.profile:
            create_kwargs["profile"] = self.opts.profile
        if self.opts.extensions:
            create_kwargs["extensions"] = self.opts.extensions

        browser = await asyncio.to_thread(
            self.kernel.browsers.create,
            **create_kwargs,
        )

        self._session_id = browser.session_id
        self._live_view_url = getattr(browser, "browser_live_view_url", None)

        print(f"Browser session: {self._session_id}")
        print(f"Live view: {self._live_view_url}")

        if self.opts.record_replay:
            try:
                replay = await asyncio.to_thread(
                    self.kernel.browsers.replays.start, self._session_id,
                )
                self._replay_id = replay.replay_id
                print(f"Replay recording started: {self._replay_id}")
            except Exception as exc:
                print(f"Warning: Failed to start replay: {exc}")

        return self.info

    async def stop(self) -> SessionInfo:
        info = self.info

        if self._session_id:
            session_id = self._session_id
            try:
                if self.opts.record_replay and self._replay_id:
                    if self.opts.replay_grace_period > 0:
                        await asyncio.sleep(self.opts.replay_grace_period)
                    await self._stop_replay()
                    info.replay_view_url = self._replay_view_url
            finally:
                # Reset state up front so that if browser deletion or a thrown replay
                # error propagates, a follow-up stop() call from the caller's error path
                # is a no-op instead of attempting to delete the same session twice.
                self._session_id = None
                self._live_view_url = None
                self._replay_id = None
                self._replay_view_url = None
                print(f"Destroying browser session: {session_id}")
                await asyncio.to_thread(
                    self.kernel.browsers.delete_by_id, session_id,
                )

        return info

    async def _stop_replay(self) -> None:
        if not self._session_id or not self._replay_id:
            return

        await asyncio.to_thread(
            self.kernel.browsers.replays.stop,
            self._replay_id,
            id=self._session_id,
        )
        await asyncio.sleep(2)

        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            try:
                replays = await asyncio.to_thread(
                    self.kernel.browsers.replays.list, self._session_id,
                )
                for r in replays:
                    if r.replay_id == self._replay_id:
                        self._replay_view_url = getattr(r, "replay_view_url", None)
                        if self._replay_view_url:
                            print(f"Replay URL: {self._replay_view_url}")
                        return
            except Exception:
                pass
            await asyncio.sleep(1)

        print("Warning: Replay may still be processing.")
