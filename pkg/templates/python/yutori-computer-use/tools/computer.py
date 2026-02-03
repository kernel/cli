"""
Yutori n1 Computer Tool

Maps n1 action format to Kernel's Computer Controls API.
"""

import asyncio
import base64
import json
from typing import Literal, TypedDict, Optional

from kernel import Kernel

from .base import ToolError, ToolResult

TYPING_DELAY_MS = 12  # Typing delay in milliseconds (used by Kernel API)
# Delays in seconds for asyncio.sleep (matches TypeScript 300ms = 0.3s)
SCREENSHOT_DELAY_S = 0.3
ACTION_DELAY_S = 0.3


# n1 action types
N1ActionType = Literal[
    "click",
    "scroll",
    "type",
    "key_press",
    "hover",
    "drag",
    "wait",
    "refresh",
    "go_back",
    "goto_url",
    "read_texts_and_links",
    "stop",
]


class N1Action(TypedDict, total=False):
    action_type: N1ActionType
    center_coordinates: tuple[int, int] | list[int]
    start_coordinates: tuple[int, int] | list[int]
    direction: Literal["up", "down", "left", "right"]
    amount: int
    text: str
    press_enter_after: bool
    clear_before_typing: bool
    key_comb: str
    url: str
    answer: str


# Key mappings from Playwright format (n1 output) to xdotool format (Kernel)
KEY_MAP = {
    "Enter": "Return",
    "Escape": "Escape",
    "Backspace": "BackSpace",
    "Tab": "Tab",
    "Delete": "Delete",
    "ArrowUp": "Up",
    "ArrowDown": "Down",
    "ArrowLeft": "Left",
    "ArrowRight": "Right",
    "Home": "Home",
    "End": "End",
    "PageUp": "Page_Up",
    "PageDown": "Page_Down",
    "F1": "F1",
    "F2": "F2",
    "F3": "F3",
    "F4": "F4",
    "F5": "F5",
    "F6": "F6",
    "F7": "F7",
    "F8": "F8",
    "F9": "F9",
    "F10": "F10",
    "F11": "F11",
    "F12": "F12",
}

MODIFIER_MAP = {
    "control": "ctrl",
    "ctrl": "ctrl",
    "alt": "alt",
    "shift": "shift",
    "meta": "super",
    "command": "super",
    "cmd": "super",
}


class ComputerTool:
    def __init__(self, kernel: Kernel, session_id: str, width: int = 1280, height: int = 800):
        self.kernel = kernel
        self.session_id = session_id
        self.width = width
        self.height = height

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
        coords = self._get_coordinates(action.get("center_coordinates"))

        self.kernel.browsers.computer.click_mouse(
            self.session_id,
            x=coords["x"],
            y=coords["y"],
            button="left",
            click_type="click",
            num_clicks=1,
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_scroll(self, action: N1Action) -> ToolResult:
        coords = self._get_coordinates(action.get("center_coordinates"))
        direction = action.get("direction")
        amount = action.get("amount", 3)

        if direction not in ("up", "down", "left", "right"):
            raise ToolError(f"Invalid scroll direction: {direction}")

        scroll_delta = amount * 100

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

        self.kernel.browsers.computer.scroll(
            self.session_id,
            x=coords["x"],
            y=coords["y"],
            delta_x=delta_x,
            delta_y=delta_y,
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_type(self, action: N1Action) -> ToolResult:
        text = action.get("text")
        if not text:
            raise ToolError("text is required for type action")

        if action.get("clear_before_typing"):
            self.kernel.browsers.computer.press_key(
                self.session_id,
                keys=["ctrl+a"],
            )
            await asyncio.sleep(0.1)
            self.kernel.browsers.computer.press_key(
                self.session_id,
                keys=["BackSpace"],
            )
            await asyncio.sleep(0.1)

        self.kernel.browsers.computer.type_text(
            self.session_id,
            text=text,
            delay=TYPING_DELAY_MS,
        )

        if action.get("press_enter_after"):
            await asyncio.sleep(0.1)
            self.kernel.browsers.computer.press_key(
                self.session_id,
                keys=["Return"],
            )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_key_press(self, action: N1Action) -> ToolResult:
        key_comb = action.get("key_comb")
        if not key_comb:
            raise ToolError("key_comb is required for key_press action")

        mapped_key = self._map_key(key_comb)

        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=[mapped_key],
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_hover(self, action: N1Action) -> ToolResult:
        coords = self._get_coordinates(action.get("center_coordinates"))

        self.kernel.browsers.computer.move_mouse(
            self.session_id,
            x=coords["x"],
            y=coords["y"],
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_drag(self, action: N1Action) -> ToolResult:
        start_coords = self._get_coordinates(action.get("start_coordinates"))
        end_coords = self._get_coordinates(action.get("center_coordinates"))

        self.kernel.browsers.computer.drag_mouse(
            self.session_id,
            path=[[start_coords["x"], start_coords["y"]], [end_coords["x"], end_coords["y"]]],
            button="left",
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_wait(self, action: N1Action) -> ToolResult:
        await asyncio.sleep(2)
        return await self.screenshot()

    async def _handle_refresh(self, action: N1Action) -> ToolResult:
        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=["F5"],
        )
        await asyncio.sleep(2)
        return await self.screenshot()

    async def _handle_go_back(self, action: N1Action) -> ToolResult:
        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=["alt+Left"],
        )
        await asyncio.sleep(1.5)
        return await self.screenshot()

    async def _handle_goto_url(self, action: N1Action) -> ToolResult:
        url = action.get("url")
        if not url:
            raise ToolError("url is required for goto_url action")

        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=["ctrl+l"],
        )
        await asyncio.sleep(ACTION_DELAY_S)

        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=["ctrl+a"],
        )
        await asyncio.sleep(0.1)

        self.kernel.browsers.computer.type_text(
            self.session_id,
            text=url,
            delay=TYPING_DELAY_MS,
        )
        await asyncio.sleep(ACTION_DELAY_S)

        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=["Return"],
        )
        await asyncio.sleep(2)
        return await self.screenshot()

    async def _handle_read_texts_and_links(self, action: N1Action) -> ToolResult:
        try:
            result = self.kernel.browsers.playwright.execute(
                self.session_id,
                code="""
                    const snapshot = await page._snapshotForAI();
                    const url = page.url();
                    const title = await page.title();
                    return { url, title, snapshot };
                """,
                timeout_sec=30
            )

            screenshot_result = await self.screenshot()

            if result.success and result.result:
                data = result.result
                return {
                    "base64_image": screenshot_result.get("base64_image", ""),
                    "output": json.dumps({
                        "url": data.get("url"),
                        "title": data.get("title"),
                        "snapshot": data.get("snapshot")
                    }, indent=2)
                }

            print("Playwright execution failed, falling back to screenshot only")
            return screenshot_result
        except Exception as e:
            print(f"read_texts_and_links failed: {e}")
            return await self.screenshot()

    async def _handle_stop(self, action: N1Action) -> ToolResult:
        return {"output": action.get("answer", "Task completed")}

    async def screenshot(self) -> ToolResult:
        try:
            response = self.kernel.browsers.computer.capture_screenshot(
                self.session_id
            )
            image_bytes = response.read()
            base64_image = base64.b64encode(image_bytes).decode("utf-8")
            return {"base64_image": base64_image}
        except Exception as e:
            raise ToolError(f"Failed to take screenshot: {e}")

    def _get_coordinates(
        self, coords: tuple[int, int] | list[int] | None
    ) -> dict[str, int]:
        if coords is None or len(coords) != 2:
            # Default to center of screen
            return {"x": self.width // 2, "y": self.height // 2}

        x, y = coords
        if not isinstance(x, (int, float)) or not isinstance(y, (int, float)) or x < 0 or y < 0:
            raise ToolError(f"Invalid coordinates: {coords}")

        return {"x": int(x), "y": int(y)}

    def _map_key(self, key: str) -> str:
        # Handle modifier combinations (e.g., "Control+a" -> "ctrl+a")
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
