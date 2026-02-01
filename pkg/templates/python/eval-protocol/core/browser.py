"""
Kernel Browser Adapters for Computer Use Actions.

Provides adapters for executing agent actions via Kernel's browser API:
- KernelBrowserAdapter: Direct browser control via session ID or browser object
- acquired_browser: Context manager for pool-based browser acquisition

Browser Pools are a key feature for RL training, enabling efficient
browser acquisition/release across many parallel environments.

See: https://docs.onkernel.com/features/browser-pools
"""

from __future__ import annotations

import asyncio
import io
import json
import logging
import random
import ssl
import threading
import time
from contextlib import contextmanager
from typing import TYPE_CHECKING, Any, Callable, Iterator, Union, cast

import websockets
from PIL import Image
from rich.console import Console

from .actions import (
    Action,
    DoubleClickAction,
    KeyAction,
    LeftClickAction,
    LeftClickDragAction,
    MiddleClickAction,
    MouseMoveAction,
    RightClickAction,
    ScrollAction,
    TerminateAction,
    TripleClickAction,
    TypeTextAction,
    WaitAction,
)
from .utils import compute_image_similarity

if TYPE_CHECKING:
    from kernel import Kernel
    from kernel.types import BrowserCreateResponse
    from kernel.types.browser_pool_acquire_response import BrowserPoolAcquireResponse

    BrowserInfo = Union[BrowserCreateResponse, BrowserPoolAcquireResponse]

logger = logging.getLogger(__name__)
console = Console()

# Type for custom action handlers: (adapter, action) -> should_continue
ActionHandler = Callable[["KernelBrowserAdapter", Action], bool]


class KernelBrowserAdapter:
    """
    Adapter for executing computer use actions via Kernel's browser API.

    Handles:
    - Coordinate conversion from normalized (0-999) to pixel space
    - Screenshot capture
    - Action execution via Kernel's computer control API
    - Heartbeat for keeping browser alive during long VLM inference
    - Extensible custom action handlers

    Usage:
        kernel = Kernel()
        browser = kernel.browsers.create(stealth=True)
        adapter = KernelBrowserAdapter(kernel, browser)

        adapter.navigate("https://example.com")
        screenshot = adapter.capture_screenshot()
        adapter.execute_action(LeftClickAction(x=500, y=300))
    """

    def __init__(
        self,
        kernel: Kernel,
        browser: BrowserInfo,
        viewport_width: int = 1920,
        viewport_height: int = 1080,
        heartbeat_interval: int = 10,
        reset_on_init: bool = False,
    ):
        """
        Initialize the adapter.

        Args:
            kernel: Kernel SDK client instance
            browser: Browser object from kernel.browsers.create() or
                kernel.browser_pools.acquire().
            viewport_width: Browser viewport width in pixels (default: 1920)
            viewport_height: Browser viewport height in pixels (default: 1080)
            heartbeat_interval: Seconds between CDP heartbeats (default: 10).
                Set to 0 to disable heartbeat capability.
            reset_on_init: If True, reset the browser to a clean state on init
                (closes popups, navigates to about:blank). Default: False.
        """
        self.kernel = kernel
        self.session_id = browser.session_id
        self.cdp_ws_url: str | None = getattr(browser, "cdp_ws_url", None)
        self.live_view_url: str | None = getattr(browser, "browser_live_view_url", None)
        self.viewport_width = viewport_width
        self.viewport_height = viewport_height
        self.heartbeat_interval = heartbeat_interval
        self._custom_handlers: dict[str, ActionHandler] = {}
        self._heartbeat: BrowserHeartbeat | None = None
        self._should_not_reuse: bool = False

        if reset_on_init:
            # Wait a moment for browser to be fully ready
            time.sleep(2)
            self.reset_browser()

    def reset_browser(self) -> None:
        """Reset the browser to a clean state."""
        cleanup_code = """
const pages = context.pages();

for (let i = 1; i < pages.length; i++) {
    await pages[i].close();
}

if (pages.length > 0) {
    pages[0].on('dialog', async (dialog) => {
        await dialog.dismiss();
    });
    await pages[0].goto('chrome://newtab', { waitUntil: 'load' });
}

return { closedPages: pages.length - 1 };
"""
        result = self.kernel.browsers.playwright.execute(
            id=self.session_id,
            code=cleanup_code,
            timeout_sec=15,
        )

        if not result.success:
            logger.warning(
                f"Browser reset failed for session {self.session_id}: {result.error}"
                + (f" (result: {result.result})" if result.result else "")
            )
            self._should_not_reuse = True
        else:
            result_data = cast(dict[str, Any], result.result) if result.result else {}
            closed = result_data.get("closedPages", 0)
            if closed > 0:
                console.print(f"[dim]Browser reset: closed {closed} extra page(s)[/]")

    def register_handler(
        self,
        action_type: str,
        handler: ActionHandler,
    ) -> None:
        """Register a custom handler for an action type."""
        self._custom_handlers[action_type] = handler

    def normalized_to_pixel(self, norm_x: int, norm_y: int) -> tuple[int, int]:
        """Convert normalized coordinates (0-999) to pixel coordinates."""
        pixel_x = int(norm_x * self.viewport_width / 999)
        pixel_y = int(norm_y * self.viewport_height / 999)
        return pixel_x, pixel_y

    def pixel_to_normalized(self, pixel_x: int, pixel_y: int) -> tuple[int, int]:
        """Convert pixel coordinates to normalized coordinates (0-999)."""
        norm_x = int(pixel_x * 999 / self.viewport_width)
        norm_y = int(pixel_y * 999 / self.viewport_height)
        return norm_x, norm_y

    def capture_screenshot(self) -> Image.Image:
        """Capture a screenshot of the current browser state."""
        image_data = self.kernel.browsers.computer.capture_screenshot(id=self.session_id)
        return Image.open(io.BytesIO(image_data.read()))

    def wait_for_screen_settle(
        self,
        baseline: Image.Image,
        change_threshold: float = 0.95,
        stability_threshold: float = 0.99,
        stability_count: int = 2,
        poll_interval: float = 0.3,
        change_timeout: float = 5.0,
        stability_timeout: float = 10.0,
    ) -> Image.Image:
        """Wait for screen to change from baseline, then stabilize."""
        change_start = time.time()
        current = baseline

        while time.time() - change_start < change_timeout:
            time.sleep(poll_interval)
            current = self.capture_screenshot()
            similarity = compute_image_similarity(baseline, current)

            if similarity < change_threshold:
                break

        stability_start = time.time()
        stable_frames = 0
        previous = current

        while time.time() - stability_start < stability_timeout:
            time.sleep(poll_interval)
            current = self.capture_screenshot()
            similarity = compute_image_similarity(previous, current)

            if similarity >= stability_threshold:
                stable_frames += 1
                if stable_frames >= stability_count:
                    break
            else:
                stable_frames = 0

            previous = current

        return current

    def navigate(self, url: str, max_retries: int = 3) -> Image.Image:
        """Navigate the browser to a URL and wait for the page to settle."""
        if not url.startswith(("http://", "https://")):
            url = f"https://{url}"

        baseline = self.capture_screenshot()
        code = f'await page.goto("{url}", {{waitUntil: "domcontentloaded"}})'

        last_error = None
        for attempt in range(max_retries):
            result = self.kernel.browsers.playwright.execute(id=self.session_id, code=code)

            if result.success:
                return self.wait_for_screen_settle(baseline=baseline)

            last_error = result.error
            if attempt < max_retries - 1:
                backoff = 0.5 * (2**attempt)
                logger.warning(
                    f"Navigation to {url} failed (attempt {attempt + 1}/{max_retries}): "
                    f"{result.error}. Retrying in {backoff}s..."
                )
                time.sleep(backoff)

        raise RuntimeError(f"Navigation to {url} failed after {max_retries} attempts: {last_error}")

    def get_current_url(self) -> str:
        """Get the current page URL."""
        result = self.kernel.browsers.playwright.execute(
            id=self.session_id, code="return { url: page.url() };"
        )
        data = cast(dict[str, str], result.result)
        url = data.get("url")
        return url if url else ""

    def execute_action(self, action: Action) -> bool:
        """Execute an action via Kernel's computer control API."""
        action_type = getattr(action, "action_type", None)
        if action_type and action_type in self._custom_handlers:
            return self._custom_handlers[action_type](self, action)

        if isinstance(action, LeftClickAction):
            px, py = self.normalized_to_pixel(action.x, action.y)
            self.kernel.browsers.computer.click_mouse(id=self.session_id, x=px, y=py, button="left")

        elif isinstance(action, RightClickAction):
            px, py = self.normalized_to_pixel(action.x, action.y)
            self.kernel.browsers.computer.click_mouse(id=self.session_id, x=px, y=py, button="right")

        elif isinstance(action, DoubleClickAction):
            px, py = self.normalized_to_pixel(action.x, action.y)
            self.kernel.browsers.computer.click_mouse(id=self.session_id, x=px, y=py, num_clicks=2)

        elif isinstance(action, TripleClickAction):
            px, py = self.normalized_to_pixel(action.x, action.y)
            self.kernel.browsers.computer.click_mouse(id=self.session_id, x=px, y=py, num_clicks=3)

        elif isinstance(action, MiddleClickAction):
            px, py = self.normalized_to_pixel(action.x, action.y)
            self.kernel.browsers.computer.click_mouse(id=self.session_id, x=px, y=py, button="middle")

        elif isinstance(action, MouseMoveAction):
            px, py = self.normalized_to_pixel(action.x, action.y)
            self.kernel.browsers.computer.move_mouse(id=self.session_id, x=px, y=py)

        elif isinstance(action, LeftClickDragAction):
            start_px, start_py = self.normalized_to_pixel(action.start_x, action.start_y)
            end_px, end_py = self.normalized_to_pixel(action.end_x, action.end_y)
            self.kernel.browsers.computer.drag_mouse(
                id=self.session_id,
                path=[[start_px, start_py], [end_px, end_py]],
                button="left",
            )

        elif isinstance(action, TypeTextAction):
            self.kernel.browsers.computer.type_text(id=self.session_id, text=action.text)

        elif isinstance(action, KeyAction):
            def normalize_key(k: str) -> str:
                if k.lower() == "enter":
                    return "Return"
                return k.capitalize()

            if len(action.keys) == 1:
                key_str = normalize_key(action.keys[0])
            else:
                key_str = "+".join(normalize_key(k) for k in action.keys)
            self.kernel.browsers.computer.press_key(id=self.session_id, keys=[key_str])

        elif isinstance(action, ScrollAction):
            px, py = self.normalized_to_pixel(action.x, action.y)
            self.kernel.browsers.computer.scroll(
                id=self.session_id,
                x=px,
                y=py,
                delta_x=action.delta_x,
                delta_y=action.delta_y,
            )

        elif isinstance(action, WaitAction):
            time.sleep(action.seconds)

        elif isinstance(action, TerminateAction):
            pass

        elif getattr(action, "is_terminal", False):
            pass

        else:
            logger.warning(f"Unknown action type: {type(action).__name__}")

        return not getattr(action, "is_terminal", False)

    def start_heartbeat_sync(self, task_label: str | None = None) -> None:
        """Start heartbeat in a dedicated background thread."""
        if self.heartbeat_interval <= 0:
            return
        if not self.cdp_ws_url:
            logger.debug(f"Cannot start heartbeat for {self.session_id}: no cdp_ws_url")
            return
        if self._heartbeat is not None:
            return

        self._heartbeat = BrowserHeartbeat(
            self.session_id, self.cdp_ws_url, self.heartbeat_interval, task_label
        )
        self._heartbeat.start_sync()

    def stop_heartbeat_sync(self) -> None:
        """Stop the heartbeat thread synchronously."""
        if self._heartbeat:
            self._heartbeat.stop_sync()
            self._heartbeat = None


class BrowserHeartbeat:
    """Keeps a browser session alive via periodic CDP WebSocket commands."""

    def __init__(
        self,
        session_id: str,
        cdp_ws_url: str,
        interval: int = 10,
        task_label: str | None = None,
    ):
        self.session_id = session_id
        self.cdp_ws_url = cdp_ws_url
        self.interval = interval
        self.task_label = task_label
        self._thread: threading.Thread | None = None
        self._stopped = threading.Event()
        self._cmd_id = 0
        self._ssl_ctx = ssl.create_default_context()
        self._ssl_ctx.check_hostname = False
        self._ssl_ctx.verify_mode = ssl.CERT_NONE

    def start_sync(self) -> None:
        """Start heartbeat in a dedicated background thread."""
        if self._thread is not None and self._thread.is_alive():
            return

        self._stopped.clear()
        self._thread = threading.Thread(
            target=self._run_heartbeat_thread,
            name=f"heartbeat-{self.session_id[:8]}",
            daemon=True,
        )
        self._thread.start()

    def stop_sync(self) -> None:
        """Stop the heartbeat thread."""
        self._stopped.set()
        if self._thread is not None:
            self._thread.join(timeout=2.0)
            self._thread = None

    def _run_heartbeat_thread(self) -> None:
        """Main heartbeat thread function."""
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)

        try:
            loop.run_until_complete(self._heartbeat_thread_main())
        except Exception as e:
            logger.debug(f"Heartbeat thread error for {self.session_id}: {e}")
        finally:
            loop.close()

    async def _heartbeat_thread_main(self) -> None:
        """Async main for heartbeat thread."""
        ws = None

        stagger = random.uniform(0, 3)
        await asyncio.sleep(stagger)

        max_retries = 5
        for attempt in range(max_retries):
            if self._stopped.is_set():
                return

            try:
                ws = await asyncio.wait_for(
                    websockets.connect(
                        self.cdp_ws_url,
                        ssl=self._ssl_ctx,
                        ping_interval=None,
                        ping_timeout=None,
                    ),
                    timeout=10,
                )

                if await self._send_heartbeat(ws):
                    break
                else:
                    await ws.close()
                    ws = None

            except asyncio.TimeoutError:
                pass
            except Exception:
                pass

            if attempt < max_retries - 1:
                backoff = min(2**attempt, 8)
                await asyncio.sleep(backoff)

        if ws is None:
            return

        try:
            while not self._stopped.is_set():
                await asyncio.sleep(self.interval)
                if self._stopped.is_set():
                    break

                result = await self._send_heartbeat(ws)

                if not result:
                    break
        finally:
            try:
                await ws.close()
            except Exception:
                pass

    async def _send_heartbeat(self, ws) -> bool:
        """Send a single heartbeat command and wait for response."""
        try:
            self._cmd_id += 1
            await ws.send(json.dumps({"id": self._cmd_id, "method": "Browser.getVersion"}))
            await asyncio.wait_for(ws.recv(), timeout=5)
            return True
        except Exception:
            return False


@contextmanager
def acquired_browser(
    kernel: "Kernel",
    pool_name: str,
    acquire_timeout_seconds: int = 60,
    viewport_width: int = 1920,
    viewport_height: int = 1080,
    heartbeat_interval: int = 10,
    reset_on_init: bool = True,
) -> Iterator[KernelBrowserAdapter]:
    """
    Context manager for acquiring a browser from a pool.

    Usage:
        kernel = Kernel()
        with acquired_browser(kernel, "my-pool") as adapter:
            adapter.navigate("https://example.com")
            screenshot = adapter.capture_screenshot()
    """
    browser = kernel.browser_pools.acquire(
        pool_name,
        acquire_timeout_seconds=acquire_timeout_seconds,
    )

    adapter = KernelBrowserAdapter(
        kernel,
        browser,
        viewport_width=viewport_width,
        viewport_height=viewport_height,
        heartbeat_interval=heartbeat_interval,
        reset_on_init=reset_on_init,
    )

    try:
        yield adapter
    except Exception:
        adapter.stop_heartbeat_sync()
        kernel.browser_pools.release(pool_name, session_id=browser.session_id, reuse=False)
        raise
    else:
        adapter.stop_heartbeat_sync()
        reuse = not adapter._should_not_reuse
        kernel.browser_pools.release(pool_name, session_id=browser.session_id, reuse=reuse)


class MockBrowserAdapter:
    """Mock adapter for testing without Kernel."""

    def __init__(
        self,
        screenshot: Image.Image,
        viewport_width: int = 1920,
        viewport_height: int = 1080,
    ):
        self.screenshot = screenshot
        self.viewport_width = viewport_width
        self.viewport_height = viewport_height
        self.action_history: list[Action] = []
        self._custom_handlers: dict[str, ActionHandler] = {}

    def register_handler(self, action_type: str, handler: ActionHandler) -> None:
        self._custom_handlers[action_type] = handler

    def normalized_to_pixel(self, norm_x: int, norm_y: int) -> tuple[int, int]:
        pixel_x = int(norm_x * self.viewport_width / 999)
        pixel_y = int(norm_y * self.viewport_height / 999)
        return pixel_x, pixel_y

    def capture_screenshot(self) -> Image.Image:
        return self.screenshot

    def navigate(self, url: str) -> Image.Image:
        return self.screenshot

    def get_current_url(self) -> str:
        return "https://mock.example.com"

    def execute_action(self, action: Action) -> bool:
        self.action_history.append(action)

        action_type = getattr(action, "action_type", None)
        if action_type and action_type in self._custom_handlers:
            return self._custom_handlers[action_type](self, action)  # type: ignore[arg-type]

        return not getattr(action, "is_terminal", False)
