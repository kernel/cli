"""
Yutori n1 Computer Tool

Maps n1-latest action format to Kernel's Computer Controls API.
Screenshots are converted to WebP for better compression across multi-step trajectories.
"""

import asyncio
import base64
from io import BytesIO
from typing import Literal, TypedDict

from kernel import Kernel
from PIL import Image

from .base import ToolError, ToolResult

TYPING_DELAY_MS = 12
SCREENSHOT_DELAY_S = 0.3
ACTION_DELAY_S = 0.3

N1ActionType = Literal[
    "left_click",
    "double_click",
    "triple_click",
    "right_click",
    "scroll",
    "type",
    "key_press",
    "hover",
    "drag",
    "wait",
    "refresh",
    "go_back",
    "goto_url",
]


class N1Action(TypedDict, total=False):
    action_type: N1ActionType
    coordinates: tuple[int, int] | list[int]
    start_coordinates: tuple[int, int] | list[int]
    direction: Literal["up", "down", "left", "right"]
    amount: int
    text: str
    press_enter_after: bool
    clear_before_typing: bool
    key_comb: str
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
    def __init__(self, kernel: Kernel, session_id: str, width: int = 1280, height: int = 800):
        self.kernel = kernel
        self.session_id = session_id
        self.width = width
        self.height = height

    async def execute(self, action: N1Action) -> ToolResult:
        action_type = action.get("action_type")

        handlers = {
            "left_click": lambda a: self._handle_click(a, "left", 1),
            "double_click": lambda a: self._handle_click(a, "left", 2),
            "triple_click": lambda a: self._handle_click(a, "left", 3),
            "right_click": lambda a: self._handle_click(a, "right", 1),
            "scroll": self._handle_scroll,
            "type": self._handle_type,
            "key_press": self._handle_key_press,
            "hover": self._handle_hover,
            "drag": self._handle_drag,
            "wait": self._handle_wait,
            "refresh": self._handle_refresh,
            "go_back": self._handle_go_back,
            "goto_url": self._handle_goto_url,
        }

        handler = handlers.get(action_type)
        if not handler:
            raise ToolError(f"Unknown action type: {action_type}")

        return await handler(action)

    async def _handle_click(self, action: N1Action, button: str, num_clicks: int) -> ToolResult:
        coords = self._get_coordinates(action.get("coordinates"))

        self.kernel.browsers.computer.click_mouse(
            self.session_id,
            x=coords["x"],
            y=coords["y"],
            button=button,
            click_type="click",
            num_clicks=num_clicks,
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_scroll(self, action: N1Action) -> ToolResult:
        coords = self._get_coordinates(action.get("coordinates"))
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
        coords = self._get_coordinates(action.get("coordinates"))

        self.kernel.browsers.computer.move_mouse(
            self.session_id,
            x=coords["x"],
            y=coords["y"],
        )

        await asyncio.sleep(SCREENSHOT_DELAY_S)
        return await self.screenshot()

    async def _handle_drag(self, action: N1Action) -> ToolResult:
        start_coords = self._get_coordinates(action.get("start_coordinates"))
        end_coords = self._get_coordinates(action.get("coordinates"))

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
