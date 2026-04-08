"""
Unified tool mapping layer.

Normalizes provider-specific action formats into Kernel Computer Controls API calls.
"""

import asyncio
import base64
from dataclasses import dataclass
from typing import Optional

from kernel import Kernel

TYPING_DELAY_MS = 12
POST_ACTION_DELAY = 0.5
SCREENSHOT_DELAY = 2.0


@dataclass
class CommonAction:
    """Provider-agnostic action representation."""
    type: str  # click, double_click, right_click, mouse_move, type, key, scroll, drag, wait, screenshot, goto, back
    x: Optional[int] = None
    y: Optional[int] = None
    text: Optional[str] = None
    keys: Optional[str] = None
    url: Optional[str] = None
    scroll_x: int = 0
    scroll_y: int = 0
    duration: Optional[int] = None
    start_x: Optional[int] = None
    start_y: Optional[int] = None
    end_x: Optional[int] = None
    end_y: Optional[int] = None
    path: Optional[list] = None


@dataclass
class ToolResult:
    output: Optional[str] = None
    error: Optional[str] = None
    base64_image: Optional[str] = None


# Key mappings: provider key names -> X11 keysym for Kernel API
KEY_MAP = {
    "return": "Return", "enter": "Return", "Enter": "Return", "ENTER": "Return",
    "left": "Left", "right": "Right", "up": "Up", "down": "Down",
    "ArrowLeft": "Left", "ArrowRight": "Right", "ArrowUp": "Up", "ArrowDown": "Down",
    "LEFT": "Left", "RIGHT": "Right", "UP": "Up", "DOWN": "Down",
    "home": "Home", "end": "End", "Home": "Home", "End": "End",
    "pageup": "Page_Up", "page_up": "Page_Up", "PageUp": "Page_Up",
    "pagedown": "Page_Down", "page_down": "Page_Down", "PageDown": "Page_Down",
    "delete": "Delete", "Delete": "Delete",
    "backspace": "BackSpace", "Backspace": "BackSpace", "BACKSPACE": "BackSpace",
    "tab": "Tab", "Tab": "Tab",
    "esc": "Escape", "escape": "Escape", "Escape": "Escape", "ESC": "Escape",
    "space": "space", "Space": "space",
    "f1": "F1", "f2": "F2", "f3": "F3", "f4": "F4", "f5": "F5", "f6": "F6",
    "f7": "F7", "f8": "F8", "f9": "F9", "f10": "F10", "f11": "F11", "f12": "F12",
    "F1": "F1", "F2": "F2", "F3": "F3", "F4": "F4", "F5": "F5", "F6": "F6",
    "F7": "F7", "F8": "F8", "F9": "F9", "F10": "F10", "F11": "F11", "F12": "F12",
}

MODIFIER_MAP = {
    "ctrl": "ctrl", "control": "ctrl", "Control": "ctrl", "Ctrl": "ctrl",
    "alt": "alt", "Alt": "alt",
    "shift": "shift", "Shift": "shift",
    "meta": "super", "Meta": "super", "cmd": "super", "command": "super",
    "win": "super", "super": "super", "Super": "super",
}


def translate_key(key: str) -> str:
    return MODIFIER_MAP.get(key) or KEY_MAP.get(key) or key


def translate_key_combination(combo: str) -> str:
    if "+" not in combo:
        return translate_key(combo)
    return "+".join(translate_key(part.strip()) for part in combo.split("+"))


class KernelExecutor:
    """Executes CommonActions against the Kernel Computer Controls API."""

    def __init__(self, kernel: Kernel, session_id: str):
        self._kernel = kernel
        self._session_id = session_id

    async def execute(self, action: CommonAction) -> ToolResult:
        try:
            return await self._do_execute(action)
        except Exception as e:
            return ToolResult(error=str(e))

    async def _do_execute(self, action: CommonAction) -> ToolResult:
        computer = self._kernel.browsers.computer

        if action.type == "screenshot":
            return await self.screenshot()

        if action.type == "click":
            computer.click_mouse(self._session_id, x=action.x or 0, y=action.y or 0, button="left", click_type="click", num_clicks=1)
            return await self._delay_and_screenshot()

        if action.type == "double_click":
            computer.click_mouse(self._session_id, x=action.x or 0, y=action.y or 0, button="left", click_type="click", num_clicks=2)
            return await self._delay_and_screenshot()

        if action.type == "triple_click":
            computer.click_mouse(self._session_id, x=action.x or 0, y=action.y or 0, button="left", click_type="click", num_clicks=3)
            return await self._delay_and_screenshot()

        if action.type == "right_click":
            computer.click_mouse(self._session_id, x=action.x or 0, y=action.y or 0, button="right", click_type="click")
            return await self._delay_and_screenshot()

        if action.type == "middle_click":
            computer.click_mouse(self._session_id, x=action.x or 0, y=action.y or 0, button="middle", click_type="click")
            return await self._delay_and_screenshot()

        if action.type == "mouse_move":
            computer.move_mouse(self._session_id, x=action.x or 0, y=action.y or 0)
            return await self._delay_and_screenshot()

        if action.type == "mouse_down":
            computer.click_mouse(self._session_id, x=action.x or 0, y=action.y or 0, button="left", click_type="down")
            return await self._delay_and_screenshot()

        if action.type == "mouse_up":
            computer.click_mouse(self._session_id, x=action.x or 0, y=action.y or 0, button="left", click_type="up")
            return await self._delay_and_screenshot()

        if action.type == "type":
            computer.type_text(self._session_id, text=action.text or "", delay=TYPING_DELAY_MS)
            return await self._delay_and_screenshot()

        if action.type == "key":
            key = translate_key_combination(action.keys or action.text or "")
            computer.press_key(self._session_id, keys=[key])
            return await self._delay_and_screenshot()

        if action.type == "scroll":
            computer.scroll(self._session_id, x=action.x or 0, y=action.y or 0, delta_x=action.scroll_x, delta_y=action.scroll_y)
            result = await self._delay_and_screenshot()
            result.output = f"Scrolled (dx={action.scroll_x}, dy={action.scroll_y})."
            return result

        if action.type == "drag":
            path = action.path or []
            if not path and action.start_x is not None and action.end_x is not None:
                path = [[action.start_x, action.start_y or 0], [action.end_x, action.end_y or 0]]
            if not path:
                path = [[0, 0], [action.x or 0, action.y or 0]]
            computer.drag_mouse(self._session_id, path=path, button="left")
            return await self._delay_and_screenshot()

        if action.type == "wait":
            await asyncio.sleep((action.duration or 1000) / 1000)
            return await self.screenshot()

        if action.type == "goto":
            computer.press_key(self._session_id, keys=["ctrl+l"])
            await asyncio.sleep(0.2)
            computer.press_key(self._session_id, keys=["ctrl+a"])
            computer.type_text(self._session_id, text=action.url or "")
            computer.press_key(self._session_id, keys=["Return"])
            await asyncio.sleep(1.5)
            return await self.screenshot()

        if action.type == "back":
            computer.press_key(self._session_id, keys=["alt+Left"])
            await asyncio.sleep(1.0)
            return await self.screenshot()

        return ToolResult(error=f"Unknown action type: {action.type}")

    async def screenshot(self) -> ToolResult:
        await asyncio.sleep(SCREENSHOT_DELAY)
        response = self._kernel.browsers.computer.capture_screenshot(self._session_id)
        image_bytes = response.read()
        b64 = base64.b64encode(image_bytes).decode("utf-8")
        return ToolResult(base64_image=b64)

    async def _delay_and_screenshot(self) -> ToolResult:
        await asyncio.sleep(POST_ACTION_DELAY)
        return await self.screenshot()
