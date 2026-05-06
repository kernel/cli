"""
Yutori n1.5 Computer Tool

Maps n1.5-latest action format to Kernel's Computer Controls API.
Screenshots are converted to WebP for better compression across multi-step trajectories.

@see https://docs.yutori.com/reference/n1-5
"""

import asyncio
import base64
import json
from io import BytesIO
from typing import Any, Literal, TypedDict

from kernel import Kernel
from PIL import Image

from .base import ToolError, ToolResult

TYPING_DELAY_MS = 12
SCREENSHOT_DELAY_S = 0.3
ACTION_DELAY_S = 0.3

N15ActionType = Literal[
    "left_click",
    "double_click",
    "triple_click",
    "middle_click",
    "right_click",
    "mouse_move",
    "mouse_down",
    "mouse_up",
    "scroll",
    "type",
    "key_press",
    "hold_key",
    "drag",
    "wait",
    "refresh",
    "go_back",
    "go_forward",
    "goto_url",
]


class N15Action(TypedDict, total=False):
    action_type: N15ActionType
    coordinates: tuple[int, int] | list[int]
    start_coordinates: tuple[int, int] | list[int]
    direction: Literal["up", "down", "left", "right"]
    amount: int
    text: str
    key: str
    modifier: str
    duration: int
    url: str


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
    def __init__(self, kernel: Kernel, session_id: str, width: int = 1280, height: int = 800, kiosk_mode: bool = False):
        self.kernel = kernel
        self.session_id = session_id
        self.width = width
        self.height = height
        self.kiosk_mode = kiosk_mode

    async def execute(self, action: N15Action) -> ToolResult:
        action_type = action.get("action_type")

        handlers = {
            "left_click": lambda a: self._handle_click(a, "left", 1),
            "double_click": lambda a: self._handle_click(a, "left", 2),
            "triple_click": lambda a: self._handle_click(a, "left", 3),
            "middle_click": lambda a: self._handle_click(a, "middle", 1),
            "right_click": lambda a: self._handle_click(a, "right", 1),
            "mouse_move": self._handle_mouse_move,
            "mouse_down": lambda a: self._handle_mouse_button(a, "down"),
            "mouse_up": lambda a: self._handle_mouse_button(a, "up"),
            "scroll": self._handle_scroll,
            "type": self._handle_type,
            "key_press": self._handle_key_press,
            "hold_key": self._handle_hold_key,
            "drag": self._handle_drag,
            "wait": self._handle_wait,
            "refresh": self._handle_refresh,
            "go_back": self._handle_go_back,
            "go_forward": self._handle_go_forward,
            "goto_url": self._handle_goto_url,
        }

        handler = handlers.get(action_type)
        if not handler:
            raise ToolError(f"Unknown action type: {action_type}")

        return await handler(action)

    async def _handle_click(self, action: N15Action, button: str, num_clicks: int) -> ToolResult:
        coords = self._get_coordinates(action.get("coordinates"))
        modifier = action.get("modifier")
        kwargs: dict[str, Any] = {
            "x": coords["x"],
            "y": coords["y"],
            "button": button,
            "click_type": "click",
            "num_clicks": num_clicks,
        }
        if modifier:
            kwargs["hold_keys"] = [self._map_key(modifier)]

        self.kernel.browsers.computer.click_mouse(self.session_id, **kwargs)

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_mouse_move(self, action: N15Action) -> ToolResult:
        coords = self._get_coordinates(action.get("coordinates"))

        self.kernel.browsers.computer.move_mouse(
            self.session_id,
            x=coords["x"],
            y=coords["y"],
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_mouse_button(self, action: N15Action, click_type: str) -> ToolResult:
        coords = self._get_coordinates(action.get("coordinates"))

        self.kernel.browsers.computer.click_mouse(
            self.session_id,
            x=coords["x"],
            y=coords["y"],
            button="left",
            click_type=click_type,
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_scroll(self, action: N15Action) -> ToolResult:
        coords = self._get_coordinates(action.get("coordinates"))
        direction = action.get("direction")
        notches = max(action.get("amount", 3), 1)

        if direction not in ("up", "down", "left", "right"):
            raise ToolError(f"Invalid scroll direction: {direction}")

        # Backend (kernel-images) uses delta_x/delta_y as wheel-event repeat count (notches), not pixels.
        delta_x = 0
        delta_y = 0

        if direction == "up":
            delta_y = -notches
        elif direction == "down":
            delta_y = notches
        elif direction == "left":
            delta_x = -notches
        elif direction == "right":
            delta_x = notches

        self.kernel.browsers.computer.scroll(
            self.session_id,
            x=coords["x"],
            y=coords["y"],
            delta_x=delta_x,
            delta_y=delta_y,
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        screenshot_result = await self.screenshot()
        screenshot_result["output"] = f"Scrolled {notches} wheel unit(s) {direction}."
        return screenshot_result

    async def _handle_type(self, action: N15Action) -> ToolResult:
        text = action.get("text")
        if not text:
            raise ToolError("text is required for type action")

        self.kernel.browsers.computer.type_text(
            self.session_id,
            text=text,
            delay=TYPING_DELAY_MS,
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_key_press(self, action: N15Action) -> ToolResult:
        key = action.get("key")
        if not key:
            raise ToolError("key is required for key_press action")

        mapped_key = self._map_key(key)

        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=[mapped_key],
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_hold_key(self, action: N15Action) -> ToolResult:
        key = action.get("key")
        if not key:
            raise ToolError("key is required for hold_key action")

        mapped_key = self._map_key(key)
        duration = action.get("duration") or 1000

        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=[mapped_key],
            duration=duration,
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_drag(self, action: N15Action) -> ToolResult:
        start_coords = self._get_coordinates(action.get("start_coordinates"))
        end_coords = self._get_coordinates(action.get("coordinates"))

        self.kernel.browsers.computer.drag_mouse(
            self.session_id,
            path=[[start_coords["x"], start_coords["y"]], [end_coords["x"], end_coords["y"]]],
            button="left",
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_wait(self, action: N15Action) -> ToolResult:
        duration = action.get("duration")
        seconds = (duration / 1000) if duration and duration > 0 else 2
        await asyncio.sleep(seconds)
        return await self.screenshot()

    async def _handle_refresh(self, action: N15Action) -> ToolResult:
        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=["F5"],
        )
        await asyncio.sleep(2)
        return await self.screenshot()

    async def _handle_go_back(self, action: N15Action) -> ToolResult:
        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=["alt+Left"],
        )
        await asyncio.sleep(1.5)
        return await self.screenshot()

    async def _handle_go_forward(self, action: N15Action) -> ToolResult:
        self.kernel.browsers.computer.press_key(
            self.session_id,
            keys=["alt+Right"],
        )
        await asyncio.sleep(1.5)
        return await self.screenshot()

    async def _handle_goto_url(self, action: N15Action) -> ToolResult:
        url = action.get("url")
        if not url:
            raise ToolError("url is required for goto_url action")

        if self.kiosk_mode:
            response = self.kernel.browsers.playwright.execute(
                self.session_id,
                code=f"await page.goto({json.dumps(url)});",
                timeout_sec=60,
            )
            if not response.success:
                raise ToolError(response.error or "Playwright goto failed")
            await asyncio.sleep(ACTION_DELAY_S)
            return await self.screenshot()

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

    async def screenshot(self) -> ToolResult:
        try:
            response = self.kernel.browsers.computer.capture_screenshot(
                self.session_id
            )
            png_bytes = response.read()
            img = Image.open(BytesIO(png_bytes))
            webp_buf = BytesIO()
            img.save(webp_buf, "WEBP", quality=80)
            base64_image = base64.b64encode(webp_buf.getvalue()).decode("utf-8")
            return {"base64_image": base64_image}
        except Exception as e:
            raise ToolError(f"Failed to take screenshot: {e}")

    def _get_coordinates(
        self, coords: tuple[int, int] | list[int] | None
    ) -> dict[str, int]:
        if coords is None or len(coords) != 2:
            return {"x": self.width // 2, "y": self.height // 2}

        x, y = coords
        if not isinstance(x, (int, float)) or not isinstance(y, (int, float)) or x < 0 or y < 0:
            raise ToolError(f"Invalid coordinates: {coords}")

        return {"x": int(x), "y": int(y)}

    def _map_key(self, key: str) -> str:
        if "+" in key:
            parts = key.split("+")
            mapped_parts = []
            for part in parts:
                trimmed = part.strip()
                lower = trimmed.lower()
                
                if lower in MODIFIER_MAP:
                    mapped_parts.append(MODIFIER_MAP[lower])
                else:
                    mapped_parts.append(KEY_MAP.get(trimmed, trimmed))
            
            return "+".join(mapped_parts)

        return KEY_MAP.get(key, key)
