"""
Tzafon Northstar Computer Tool

Executes function tool calls from the Northstar model on the browser.
Coordinates arrive in a normalised 0-999 grid and are scaled to the
browser viewport before dispatch.
"""

import asyncio
import base64
from typing import Any

from kernel import Kernel

from .base import ToolError

KEY_MAP: dict[str, str] = {
    "return": "Return", "enter": "Return",
    "space": "space", "tab": "Tab",
    "backspace": "BackSpace", "delete": "Delete",
    "escape": "Escape", "esc": "Escape", "insert": "Insert",
    "up": "Up", "down": "Down", "left": "Left", "right": "Right",
    "home": "Home", "end": "End",
    "pageup": "Page_Up", "page_up": "Page_Up",
    "pagedown": "Page_Down", "page_down": "Page_Down",
    **{f"f{i}": f"F{i}" for i in range(1, 13)},
}

MODIFIER_MAP: dict[str, str] = {
    "ctrl": "ctrl", "control": "ctrl",
    "alt": "alt", "shift": "shift",
    "meta": "super", "cmd": "super", "command": "super", "win": "super",
}


def _map_key(key_combo: str) -> str:
    """Map a key combo string like 'ctrl+a' or 'Enter' to xdotool format."""
    parts = key_combo.split("+") if "+" in key_combo else [key_combo]
    mapped = []
    for part in parts:
        k = part.strip().lower()
        mapped.append(MODIFIER_MAP.get(k) or KEY_MAP.get(k, part.strip()))
    return "+".join(mapped)


class ComputerTool:
    def __init__(self, kernel: Kernel, session_id: str, width: int = 1280, height: int = 800):
        self.kernel = kernel
        self.session_id = session_id
        self.width = width
        self.height = height

    @staticmethod
    def _coord(val: Any) -> int:
        """Parse a coordinate value. Handles ints, floats, strings, and
        the model's occasional '470,77' (comma-separated pair in one field)."""
        if val is None:
            return 0
        s = str(val)
        if "," in s:
            s = s.split(",")[0].strip()
        return int(float(s))

    def _scale(self, x: Any, y: Any) -> tuple[int, int]:
        """Convert 0-999 grid coordinates to pixel coordinates."""
        x, y = self._coord(x), self._coord(y)
        px = max(0, min(x * (self.width - 1) // 999, self.width - 1))
        py = max(0, min(y * (self.height - 1) // 999, self.height - 1))
        return px, py

    async def execute_function(self, name: str, args: dict) -> None:
        if name == "click":
            px, py = self._scale(args["x"], args["y"])
            self.kernel.browsers.computer.click_mouse(
                self.session_id, x=px, y=py, button=args.get("button", "left"),
            )

        elif name == "double_click":
            px, py = self._scale(args["x"], args["y"])
            self.kernel.browsers.computer.click_mouse(
                self.session_id, x=px, y=py, num_clicks=2,
            )

        elif name == "point_and_type":
            px, py = self._scale(args["x"], args["y"])
            self.kernel.browsers.computer.click_mouse(self.session_id, x=px, y=py)
            await asyncio.sleep(0.3)
            self.kernel.browsers.computer.type_text(self.session_id, text=args["text"])
            if args.get("press_enter"):
                await asyncio.sleep(0.1)
                self.kernel.browsers.computer.press_key(self.session_id, keys=["Return"])

        elif name == "key":
            self.kernel.browsers.computer.press_key(
                self.session_id, keys=[_map_key(args["keys"])],
            )

        elif name == "scroll":
            px, py = self._scale(args.get("x", 500), args.get("y", 500))
            dy = max(-10, min(10, int(args.get("dy", 3))))
            self.kernel.browsers.computer.scroll(
                self.session_id, x=px, y=py, delta_x=0, delta_y=dy,
            )

        elif name == "drag":
            px1, py1 = self._scale(args["x1"], args["y1"])
            px2, py2 = self._scale(args["x2"], args["y2"])
            self.kernel.browsers.computer.drag_mouse(
                self.session_id, path=[[px1, py1], [px2, py2]],
            )

        else:
            raise ToolError(f"Unknown function: {name}")

    def capture_screenshot(self) -> str:
        res = self.kernel.browsers.computer.capture_screenshot(self.session_id)
        b64 = base64.b64encode(res.read()).decode()
        return f"data:image/png;base64,{b64}"
