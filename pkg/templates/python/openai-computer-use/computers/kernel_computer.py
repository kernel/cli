import base64
import json
import time
from typing import List, Dict, Any

from kernel import Kernel

# CUA model key names -> X11 keysym names for the Kernel computer API
KEYSYM_MAP = {
    "ENTER": "Return",
    "Enter": "Return",
    "RETURN": "Return",
    "BACKSPACE": "BackSpace",
    "Backspace": "BackSpace",
    "DELETE": "Delete",
    "TAB": "Tab",
    "ESCAPE": "Escape",
    "Escape": "Escape",
    "ESC": "Escape",
    "SPACE": "space",
    "Space": "space",
    "UP": "Up",
    "DOWN": "Down",
    "LEFT": "Left",
    "RIGHT": "Right",
    "HOME": "Home",
    "END": "End",
    "PAGEUP": "Prior",
    "PAGE_UP": "Prior",
    "PageUp": "Prior",
    "PAGEDOWN": "Next",
    "PAGE_DOWN": "Next",
    "PageDown": "Next",
    "CAPS_LOCK": "Caps_Lock",
    "CapsLock": "Caps_Lock",
    "CTRL": "Control_L",
    "Ctrl": "Control_L",
    "CONTROL": "Control_L",
    "Control": "Control_L",
    "ALT": "Alt_L",
    "Alt": "Alt_L",
    "SHIFT": "Shift_L",
    "Shift": "Shift_L",
    "META": "Super_L",
    "Meta": "Super_L",
    "SUPER": "Super_L",
    "Super": "Super_L",
    "CMD": "Super_L",
    "COMMAND": "Super_L",
    "F1": "F1", "F2": "F2", "F3": "F3", "F4": "F4",
    "F5": "F5", "F6": "F6", "F7": "F7", "F8": "F8",
    "F9": "F9", "F10": "F10", "F11": "F11", "F12": "F12",
    "INSERT": "Insert",
    "Insert": "Insert",
    "PRINT": "Print",
    "SCROLLLOCK": "Scroll_Lock",
    "PAUSE": "Pause",
    "NUMLOCK": "Num_Lock",
}


def _translate_keys(keys: List[str]) -> List[str]:
    return [KEYSYM_MAP.get(k, k) for k in keys]


def _normalize_button(button) -> str:
    if button is None:
        return "left"
    if isinstance(button, int):
        return {1: "left", 2: "middle", 3: "right"}.get(button, "left")
    return str(button)


def _translate_cua_action(action: Dict[str, Any]) -> Dict[str, Any]:
    action_type = action.get("type", "")
    if action_type == "click":
        button = action.get("button")
        if button == "back":
            return {"type": "press_key", "press_key": {"keys": ["Alt_L", "Left"]}}
        if button == "forward":
            return {"type": "press_key", "press_key": {"keys": ["Alt_L", "Right"]}}
        if button == "wheel":
            return {
                "type": "scroll",
                "scroll": {"x": action.get("x", 0), "y": action.get("y", 0), "delta_x": 0, "delta_y": 0},
            }
        return {
            "type": "click_mouse",
            "click_mouse": {
                "x": action.get("x", 0),
                "y": action.get("y", 0),
                "button": _normalize_button(button),
            },
        }
    elif action_type == "double_click":
        return {
            "type": "click_mouse",
            "click_mouse": {
                "x": action.get("x", 0),
                "y": action.get("y", 0),
                "num_clicks": 2,
            },
        }
    elif action_type == "type":
        return {"type": "type_text", "type_text": {"text": action.get("text", "")}}
    elif action_type == "keypress":
        return {"type": "press_key", "press_key": {"keys": _translate_keys(action.get("keys", []))}}
    elif action_type == "scroll":
        return {
            "type": "scroll",
            "scroll": {
                "x": action.get("x", 0),
                "y": action.get("y", 0),
                "delta_x": action.get("scroll_x", 0),
                "delta_y": action.get("scroll_y", 0),
            },
        }
    elif action_type == "move":
        return {"type": "move_mouse", "move_mouse": {"x": action.get("x", 0), "y": action.get("y", 0)}}
    elif action_type == "drag":
        path = [[p["x"], p["y"]] for p in action.get("path", [])]
        return {"type": "drag_mouse", "drag_mouse": {"path": path}}
    elif action_type == "wait":
        return {"type": "sleep", "sleep": {"duration_ms": action.get("ms", 1000)}}
    else:
        raise ValueError(f"Unknown CUA action type: {action_type}")


class KernelComputer:
    """Wraps Kernel's native computer control API for browser automation."""

    def __init__(self, client: Kernel, session_id: str):
        self.client = client
        self.session_id = session_id

    def get_environment(self):
        return "browser"

    def get_dimensions(self):
        return (1024, 768)

    def screenshot(self) -> str:
        resp = self.client.browsers.computer.capture_screenshot(self.session_id)
        return base64.b64encode(resp.read()).decode("utf-8")

    def click(self, x: int, y: int, button="left") -> None:
        if button == "back":
            self.back()
            return
        if button == "forward":
            self.forward()
            return
        if button == "wheel":
            self.scroll(x, y, 0, 0)
            return
        self.client.browsers.computer.click_mouse(self.session_id, x=x, y=y, button=_normalize_button(button))

    def double_click(self, x: int, y: int) -> None:
        self.client.browsers.computer.click_mouse(self.session_id, x=x, y=y, num_clicks=2)

    def type(self, text: str) -> None:
        self.client.browsers.computer.type_text(self.session_id, text=text)

    def keypress(self, keys: List[str]) -> None:
        self.client.browsers.computer.press_key(self.session_id, keys=_translate_keys(keys))

    def scroll(self, x: int, y: int, scroll_x: int, scroll_y: int) -> None:
        self.client.browsers.computer.scroll(self.session_id, x=x, y=y, delta_x=scroll_x, delta_y=scroll_y)

    def move(self, x: int, y: int) -> None:
        self.client.browsers.computer.move_mouse(self.session_id, x=x, y=y)

    def drag(self, path: List[Dict[str, int]]) -> None:
        p = [[pt["x"], pt["y"]] for pt in path]
        self.client.browsers.computer.drag_mouse(self.session_id, path=p)

    def wait(self, ms: int = 1000) -> None:
        time.sleep(ms / 1000)

    def batch_actions(self, actions: List[Dict[str, Any]]) -> None:
        translated = [_translate_cua_action(a) for a in actions]
        self.client.browsers.computer.batch(self.session_id, actions=translated)

    def goto(self, url: str) -> None:
        self.client.browsers.playwright.execute(
            self.session_id, code=f"await page.goto({json.dumps(url)})"
        )

    def back(self) -> None:
        self.client.browsers.playwright.execute(self.session_id, code="await page.goBack()")

    def forward(self) -> None:
        self.client.browsers.playwright.execute(self.session_id, code="await page.goForward()")

    def get_current_url(self) -> str:
        result = self.client.browsers.playwright.execute(self.session_id, code="return page.url()")
        return result.result if result.result else ""
