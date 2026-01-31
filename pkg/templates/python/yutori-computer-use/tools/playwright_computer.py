"""
Yutori n1 Playwright Computer Tool

Maps n1 action format to Playwright methods via CDP WebSocket connection.
Uses viewport-only screenshots optimized for Yutori n1's training data.

See: https://docs.yutori.com/reference/n1#screenshot-requirements
"""

import asyncio
import base64
import json
from typing import Optional

from playwright.async_api import async_playwright, Browser, BrowserContext, Page

from .base import ToolError, ToolResult
from .computer import N1Action

# Delay after actions before taking screenshot (in seconds for asyncio.sleep)
# Matches TypeScript SCREENSHOT_DELAY_MS = 300 (300ms = 0.3s)
SCREENSHOT_DELAY_S = 0.3

# Key mappings from n1 output format to Playwright format
KEY_MAP = {
    "Return": "Enter",
    "BackSpace": "Backspace",
    "Page_Up": "PageUp",
    "Page_Down": "PageDown",
}

MODIFIER_MAP = {
    "ctrl": "Control",
    "super": "Meta",
    "command": "Meta",
    "cmd": "Meta",
}


class PlaywrightComputerTool:
    def __init__(self, cdp_ws_url: str, width: int = 1200, height: int = 800):
        self.cdp_ws_url = cdp_ws_url
        self.width = width
        self.height = height
        self._playwright = None
        self._browser: Optional[Browser] = None
        self._context: Optional[BrowserContext] = None
        self._page: Optional[Page] = None

    async def connect(self) -> None:
        if self._browser:
            return  # Already connected

        self._playwright = await async_playwright().start()
        self._browser = await self._playwright.chromium.connect_over_cdp(self.cdp_ws_url)

        # Get existing context or create new one
        contexts = self._browser.contexts
        self._context = contexts[0] if contexts else await self._browser.new_context()

        # Handle new page events
        self._context.on("page", self._handle_new_page)

        # Get existing page or create new one
        pages = self._context.pages
        self._page = pages[0] if pages else await self._context.new_page()

        # Set viewport size to Yutori's recommended dimensions
        await self._page.set_viewport_size({"width": self.width, "height": self.height})
        self._page.on("close", self._handle_page_close)

    async def disconnect(self) -> None:
        if self._playwright:
            await self._playwright.stop()
        self._playwright = None
        self._browser = None
        self._context = None
        self._page = None

    def _handle_new_page(self, page: Page) -> None:
        print("New page created")
        self._page = page
        page.on("close", self._handle_page_close)

    def _handle_page_close(self, closed_page: Page) -> None:
        print("Page closed")
        if self._page == closed_page and self._context:
            pages = self._context.pages
            if pages:
                self._page = pages[-1]
            else:
                print("Warning: All pages have been closed.")
                self._page = None

    def _assert_page(self) -> Page:
        if not self._page:
            raise ToolError("Page not available. Did you call connect()?")
        return self._page

    async def execute(self, action: N1Action) -> ToolResult:
        action_type = action.get("action_type")

        handlers = {
            "click": self._handle_click,
            "scroll": self._handle_scroll,
            "type": self._handle_type,
            "key_press": self._handle_key_press,
            "hover": self._handle_hover,
            "drag": self._handle_drag,
            "wait": self._handle_wait,
            "refresh": self._handle_refresh,
            "go_back": self._handle_go_back,
            "goto_url": self._handle_goto_url,
            "read_texts_and_links": self._handle_read_texts_and_links,
            "stop": self._handle_stop,
        }

        handler = handlers.get(action_type)
        if not handler:
            raise ToolError(f"Unknown action type: {action_type}")

        return await handler(action)

    async def _handle_click(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        coords = self._get_coordinates(action.get("center_coordinates"))

        await page.mouse.click(coords["x"], coords["y"])
        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_scroll(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        coords = self._get_coordinates(action.get("center_coordinates"))
        direction = action.get("direction")
        amount = action.get("amount", 3)

        if direction not in ("up", "down", "left", "right"):
            raise ToolError(f"Invalid scroll direction: {direction}")

        scroll_delta = amount * 100

        await page.mouse.move(coords["x"], coords["y"])

        delta_x = 0
        delta_y = 0

        if direction == "up":
            delta_y = -scroll_delta
        elif direction == "down":
            delta_y = scroll_delta
        elif direction == "left":
            delta_x = -scroll_delta
        elif direction == "right":
            delta_x = scroll_delta

        await page.mouse.wheel(delta_x, delta_y)
        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_type(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        text = action.get("text")
        if not text:
            raise ToolError("text is required for type action")

        if action.get("clear_before_typing"):
            await page.keyboard.press("Control+a")
            await asyncio.sleep(0.1)
            await page.keyboard.press("Backspace")
            await asyncio.sleep(0.1)

        await page.keyboard.type(text)

        if action.get("press_enter_after"):
            await asyncio.sleep(0.1)
            await page.keyboard.press("Enter")

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_key_press(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        key_comb = action.get("key_comb")
        if not key_comb:
            raise ToolError("key_comb is required for key_press action")

        mapped_key = self._map_key_to_playwright(key_comb)
        await page.keyboard.press(mapped_key)

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_hover(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        coords = self._get_coordinates(action.get("center_coordinates"))

        await page.mouse.move(coords["x"], coords["y"])

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_drag(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        start_coords = self._get_coordinates(action.get("start_coordinates"))
        end_coords = self._get_coordinates(action.get("center_coordinates"))

        await page.mouse.move(start_coords["x"], start_coords["y"])
        await page.mouse.down()
        await asyncio.sleep(0.05)
        await page.mouse.move(end_coords["x"], end_coords["y"], steps=12)
        await page.mouse.up()

        await asyncio.sleep(0.3)
        return await self.screenshot()

    async def _handle_wait(self, action: N1Action) -> ToolResult:
        await asyncio.sleep(2)
        return await self.screenshot()

    async def _handle_refresh(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        await page.reload()
        await asyncio.sleep(2)
        return await self.screenshot()

    async def _handle_go_back(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        await page.go_back()
        await asyncio.sleep(1.5)
        return await self.screenshot()

    async def _handle_goto_url(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        url = action.get("url")
        if not url:
            raise ToolError("url is required for goto_url action")

        await page.goto(url)
        await asyncio.sleep(2)
        return await self.screenshot()

    async def _handle_read_texts_and_links(self, action: N1Action) -> ToolResult:
        page = self._assert_page()
        try:
            snapshot = await page.locator("body").aria_snapshot()
            url = page.url
            title = await page.title()

            screenshot_result = await self.screenshot()

            return {
                "base64_image": screenshot_result.get("base64_image", ""),
                "output": json.dumps({"url": url, "title": title, "snapshot": snapshot}, indent=2),
            }
        except Exception as e:
            print(f"read_texts_and_links failed: {e}")
            return await self.screenshot()

    async def _handle_stop(self, action: N1Action) -> ToolResult:
        return {"output": action.get("answer", "Task completed")}

    async def screenshot(self) -> ToolResult:
        page = self._assert_page()
        try:
            buffer = await page.screenshot(full_page=False)
            base64_image = base64.b64encode(buffer).decode("utf-8")
            return {"base64_image": base64_image}
        except Exception as e:
            raise ToolError(f"Failed to take screenshot: {e}")

    def get_current_url(self) -> str:
        page = self._assert_page()
        return page.url

    def _get_coordinates(
        self, coords: tuple[int, int] | list[int] | None
    ) -> dict[str, int]:
        if coords is None or len(coords) != 2:
            # Default to center of viewport
            return {"x": self.width // 2, "y": self.height // 2}

        x, y = coords
        if not isinstance(x, (int, float)) or not isinstance(y, (int, float)) or x < 0 or y < 0:
            raise ToolError(f"Invalid coordinates: {coords}")

        return {"x": int(x), "y": int(y)}

    def _map_key_to_playwright(self, key: str) -> str:
        # Handle modifier combinations (e.g., "ctrl+a" -> "Control+a")
        if "+" in key:
            parts = key.split("+")
            mapped_parts = []
            for part in parts:
                trimmed = part.strip()
                lower = trimmed.lower()

                # Map modifier names
                if lower in MODIFIER_MAP:
                    mapped_parts.append(MODIFIER_MAP[lower])
                else:
                    # Check KEY_MAP for special keys
                    mapped_parts.append(KEY_MAP.get(trimmed, trimmed))

            return "+".join(mapped_parts)

        return KEY_MAP.get(key, key)
