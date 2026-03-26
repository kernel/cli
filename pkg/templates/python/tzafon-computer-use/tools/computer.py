"""
Tzafon Northstar Computer Tool

Maps Tzafon Northstar CUA action format to Kernel's Computer Controls API.
Northstar returns actions via computer_call outputs with attributes like
.type, .x, .y, .text, .keys, .scroll_x, .scroll_y, .url, etc.
"""

import asyncio
import base64
import json
from typing import Any

from kernel import Kernel

from .base import ToolError

MODIFIER_MAP = {
    "Control": "Ctrl",
    "control": "Ctrl",
    "ctrl": "Ctrl",
    "Enter": "Return",
    "enter": "Return",
    "Escape": "Escape",
    "esc": "Escape",
    "Shift": "Shift",
    "shift": "Shift",
    "Alt": "Alt",
    "alt": "Alt",
}
MODIFIER_NAMES = {"Ctrl", "Shift", "Alt", "Meta", "Super"}


def clamp(val: float, max_val: int) -> int:
    return max(0, min(round(val), max_val - 1))


def format_keys(keys: list[str]) -> list[str]:
    """Convert Tzafon key names to Kernel X11 keysym format.

    Tzafon: ["Control", "c"]  ->  Kernel: ["Ctrl+c"]
    """
    modifiers: list[str] = []
    regular: list[str] = []
    for key in keys:
        mapped = MODIFIER_MAP.get(key, key)
        if mapped in MODIFIER_NAMES:
            modifiers.append(mapped)
        else:
            regular.append(mapped)

    if not regular and modifiers:
        return ["+".join(modifiers)]
    return ["+".join([*modifiers, k]) for k in regular]


class ComputerTool:
    def __init__(self, kernel: Kernel, session_id: str, width: int = 1280, height: int = 800):
        self.kernel = kernel
        self.session_id = session_id
        self.width = width
        self.height = height

    def _x(self, action: Any) -> int:
        return clamp(action.x, self.width)

    def _y(self, action: Any) -> int:
        return clamp(action.y, self.height)

    def _center_x(self) -> int:
        return self.width // 2

    def _center_y(self) -> int:
        return self.height // 2

    async def execute(self, action: Any) -> None:
        """Map a Tzafon model action to Kernel Computer Controls."""
        t = action.type

        if t == "click" and getattr(action, "button", "left") == "right":
            self.kernel.browsers.computer.click_mouse(
                self.session_id, x=self._x(action), y=self._y(action), button="right",
            )
        elif t == "click":
            self.kernel.browsers.computer.click_mouse(
                self.session_id, x=self._x(action), y=self._y(action),
            )
        elif t == "double_click":
            self.kernel.browsers.computer.click_mouse(
                self.session_id, x=self._x(action), y=self._y(action), num_clicks=2,
            )
        elif t == "triple_click":
            self.kernel.browsers.computer.click_mouse(
                self.session_id, x=self._x(action), y=self._y(action), num_clicks=3,
            )
        elif t == "type":
            self.kernel.browsers.computer.type_text(self.session_id, text=action.text)
        elif t in ("key", "keypress"):
            self.kernel.browsers.computer.press_key(
                self.session_id, keys=format_keys(action.keys),
            )
        elif t == "key_down":
            self.kernel.browsers.computer.press_key(
                self.session_id, keys=format_keys(action.keys), duration=5000,
            )
        elif t == "key_up":
            pass
        elif t == "scroll":
            self.kernel.browsers.computer.scroll(
                self.session_id,
                x=clamp(self._center_x() if action.x is None else action.x, self.width),
                y=clamp(self._center_y() if action.y is None else action.y, self.height),
                delta_x=0 if action.scroll_x is None else action.scroll_x,
                delta_y=0 if action.scroll_y is None else action.scroll_y,
            )
        elif t == "hscroll":
            self.kernel.browsers.computer.scroll(
                self.session_id,
                x=clamp(self._center_x() if action.x is None else action.x, self.width),
                y=clamp(self._center_y() if action.y is None else action.y, self.height),
                delta_x=0 if action.scroll_x is None else action.scroll_x,
            )
        elif t == "navigate":
            self.kernel.browsers.playwright.execute(
                self.session_id, code=f"await page.goto({json.dumps(action.url)})",
            )
        elif t == "drag":
            x1 = getattr(action, "x", None)
            if x1 is None:
                x1 = getattr(action, "x1", None)
            y1 = getattr(action, "y", None)
            if y1 is None:
                y1 = getattr(action, "y1", None)
            x2 = getattr(action, "end_x", None)
            if x2 is None:
                x2 = getattr(action, "x2", None)
            y2 = getattr(action, "end_y", None)
            if y2 is None:
                y2 = getattr(action, "y2", None)
            if any(v is None for v in (x1, y1, x2, y2)):
                print(f"drag action missing coordinates, skipping: {action}")
                return
            self.kernel.browsers.computer.drag_mouse(
                self.session_id,
                path=[
                    [clamp(x1, self.width), clamp(y1, self.height)],
                    [clamp(x2, self.width), clamp(y2, self.height)],
                ],
            )
        elif t == "wait":
            await asyncio.sleep(2)
        else:
            raise ToolError(f"Unknown action type: {t}")

    def capture_screenshot(self) -> str:
        """Capture a screenshot and return it as a base64 data URL."""
        res = self.kernel.browsers.computer.capture_screenshot(self.session_id)
        b64 = base64.b64encode(res.read()).decode()
        return f"data:image/png;base64,{b64}"
