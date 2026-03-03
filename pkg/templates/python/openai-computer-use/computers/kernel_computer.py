import base64
import json
import time
from typing import List, Dict, Any, Callable

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
        return {
            "type": "click_mouse",
            "click_mouse": {
                "x": action.get("x", 0),
                "y": action.get("y", 0),
                "button": _normalize_button(action.get("button")),
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


def _truncate(text: str, max_len: int = 60) -> str:
    if len(text) <= max_len:
        return text
    return f"{text[: max_len - 3]}..."


def _describe_action(action_type: str, action_args: Dict[str, Any]) -> str:
    if action_type == "click":
        x = int(action_args.get("x", 0))
        y = int(action_args.get("y", 0))
        button = str(action_args.get("button", "left"))
        if button in ("", "left"):
            return f"click({x}, {y})"
        return f"click({x}, {y}, {button})"
    if action_type == "double_click":
        return f"double_click({int(action_args.get('x', 0))}, {int(action_args.get('y', 0))})"
    if action_type == "type":
        text = _truncate(str(action_args.get("text", "")))
        return f"type({text!r})"
    if action_type == "keypress":
        return f"keypress({action_args.get('keys', [])})"
    if action_type == "scroll":
        return (
            f"scroll({int(action_args.get('x', 0))}, {int(action_args.get('y', 0))}, "
            f"dx={int(action_args.get('scroll_x', 0))}, dy={int(action_args.get('scroll_y', 0))})"
        )
    if action_type == "move":
        return f"move({int(action_args.get('x', 0))}, {int(action_args.get('y', 0))})"
    if action_type == "drag":
        return "drag(...)"
    if action_type == "wait":
        return f"wait({int(action_args.get('ms', 1000))}ms)"
    return action_type


def _describe_batch_actions(actions: List[Dict[str, Any]]) -> str:
    pieces = []
    for action in actions:
        action_type = str(action.get("type", "unknown"))
        action_args = {k: v for k, v in action.items() if k != "type"}
        pieces.append(_describe_action(action_type, action_args))
    return "batch[" + " -> ".join(pieces) + "]"


class KernelComputer:
    """Wraps Kernel's native computer control API for browser automation."""

    def __init__(
        self,
        client: Kernel,
        session_id: str,
        on_event: Callable[[dict], None] | None = None,
    ):
        self.client = client
        self.session_id = session_id
        self.on_event = on_event

    def get_environment(self):
        return "browser"

    def get_dimensions(self):
        return (1024, 768)

    def _emit_backend(
        self, op: str, detail: str | None = None, elapsed_ms: int | None = None
    ) -> None:
        if not self.on_event:
            return
        data: Dict[str, Any] = {"op": op}
        if detail:
            data["detail"] = detail
        if elapsed_ms is not None:
            data["elapsed_ms"] = elapsed_ms
        self.on_event({"event": "backend", "data": data})

    def _trace_backend(
        self,
        op: str,
        fn: Callable[[], Any],
        detail: str | Callable[[Any], str | None] | None = None,
    ) -> Any:
        self._emit_backend(op)
        started_at = time.time()
        completed = False
        result = None
        try:
            result = fn()
            completed = True
            return result
        finally:
            resolved_detail = None
            if completed:
                if callable(detail):
                    try:
                        resolved_detail = detail(result)
                    except Exception:
                        resolved_detail = None
                elif isinstance(detail, str):
                    resolved_detail = detail
            elapsed_ms = int((time.time() - started_at) * 1000)
            self._emit_backend(f"{op}.done", resolved_detail, elapsed_ms)

    def screenshot(self) -> str:
        def _do() -> str:
            resp = self.client.browsers.computer.capture_screenshot(self.session_id)
            return base64.b64encode(resp.read()).decode("utf-8")

        return self._trace_backend("screenshot", _do)

    def click(self, x: int, y: int, button="left") -> None:
        normalized_button = _normalize_button(button)
        op = _describe_action("click", {"x": x, "y": y, "button": normalized_button})
        self._trace_backend(
            op,
            lambda: self.client.browsers.computer.click_mouse(
                self.session_id, x=x, y=y, button=normalized_button
            ),
        )

    def double_click(self, x: int, y: int) -> None:
        op = _describe_action("double_click", {"x": x, "y": y})
        self._trace_backend(
            op,
            lambda: self.client.browsers.computer.click_mouse(
                self.session_id, x=x, y=y, num_clicks=2
            ),
        )

    def type(self, text: str) -> None:
        op = _describe_action("type", {"text": text})
        self._trace_backend(
            op, lambda: self.client.browsers.computer.type_text(self.session_id, text=text)
        )

    def keypress(self, keys: List[str]) -> None:
        translated_keys = _translate_keys(keys)
        op = _describe_action("keypress", {"keys": translated_keys})
        self._trace_backend(
            op,
            lambda: self.client.browsers.computer.press_key(
                self.session_id, keys=translated_keys
            ),
        )

    def scroll(self, x: int, y: int, scroll_x: int, scroll_y: int) -> None:
        op = _describe_action(
            "scroll", {"x": x, "y": y, "scroll_x": scroll_x, "scroll_y": scroll_y}
        )
        self._trace_backend(
            op,
            lambda: self.client.browsers.computer.scroll(
                self.session_id, x=x, y=y, delta_x=scroll_x, delta_y=scroll_y
            ),
        )

    def move(self, x: int, y: int) -> None:
        op = _describe_action("move", {"x": x, "y": y})
        self._trace_backend(
            op, lambda: self.client.browsers.computer.move_mouse(self.session_id, x=x, y=y)
        )

    def drag(self, path: List[Dict[str, int]]) -> None:
        op = _describe_action("drag", {"path": path})

        def _do() -> None:
            p = [[pt["x"], pt["y"]] for pt in path]
            self.client.browsers.computer.drag_mouse(self.session_id, path=p)

        self._trace_backend(op, _do)

    def wait(self, ms: int = 1000) -> None:
        time.sleep(ms / 1000)

    def batch_actions(self, actions: List[Dict[str, Any]]) -> None:
        op = _describe_batch_actions(actions)

        def _do() -> None:
            translated = [_translate_cua_action(a) for a in actions]
            self.client.browsers.computer.batch(self.session_id, actions=translated)

        self._trace_backend(op, _do)

    def goto(self, url: str) -> None:
        op = f"goto({json.dumps(url)})"
        self._trace_backend(
            op,
            lambda: self.client.browsers.playwright.execute(
                self.session_id, code=f"await page.goto({json.dumps(url)})"
            ),
        )

    def back(self) -> None:
        self._trace_backend(
            "back()",
            lambda: self.client.browsers.playwright.execute(
                self.session_id, code="await page.goBack()"
            ),
        )

    def forward(self) -> None:
        self._trace_backend(
            "forward()",
            lambda: self.client.browsers.playwright.execute(
                self.session_id, code="await page.goForward()"
            ),
        )

    def get_current_url(self) -> str:
        def _do() -> str:
            result = self.client.browsers.playwright.execute(
                self.session_id, code="return page.url()"
            )
            return result.result if result.result else ""

        return self._trace_backend("get_current_url()", _do)
