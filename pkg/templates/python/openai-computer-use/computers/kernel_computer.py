import base64
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
MODIFIER_KEYSYMS = {
    "Control_L",
    "Control_R",
    "Alt_L",
    "Alt_R",
    "Shift_L",
    "Shift_R",
    "Super_L",
    "Super_R",
    "Meta_L",
    "Meta_R",
}
GOTO_CHORD_DELAY_MS = 200


def _translate_keys(keys: List[str]) -> List[str]:
    return [KEYSYM_MAP.get(k, k) for k in keys]


def _expand_combo_keys(keys: List[str]) -> List[str]:
    out: List[str] = []
    for raw in keys:
        if not isinstance(raw, str):
            continue
        parts = raw.split("+") if "+" in raw else [raw]
        for part in parts:
            token = part.strip()
            if token:
                out.append(token)
    return out


def _normalize_keypress_payload(
    keys: List[str] | None = None, hold_keys: List[str] | None = None
) -> Dict[str, List[str]]:
    translated_hold = _translate_keys(_expand_combo_keys(hold_keys or []))
    translated_keys = _translate_keys(_expand_combo_keys(keys or []))

    hold_from_keys: List[str] = []
    primary_keys: List[str] = []
    for key in translated_keys:
        if key in MODIFIER_KEYSYMS:
            hold_from_keys.append(key)
        else:
            primary_keys.append(key)

    if not primary_keys:
        return {"keys": translated_keys, "hold_keys": translated_hold}

    merged_hold = translated_hold + hold_from_keys
    deduped_hold: List[str] = []
    for key in merged_hold:
        if key not in deduped_hold:
            deduped_hold.append(key)
    return {"keys": primary_keys, "hold_keys": deduped_hold}


def _normalize_button(button) -> str:
    if button is None:
        return "left"
    if isinstance(button, int):
        return {1: "left", 2: "middle", 3: "right"}.get(button, "left")
    return str(button)


def _normalize_drag_path(path: Any) -> List[List[int]]:
    points: List[List[int]] = []
    if isinstance(path, list):
        for point in path:
            if not isinstance(point, dict):
                continue
            x = point.get("x")
            y = point.get("y")
            if (
                isinstance(x, (int, float))
                and not isinstance(x, bool)
                and isinstance(y, (int, float))
                and not isinstance(y, bool)
            ):
                points.append([int(x), int(y)])
    if not points:
        return []
    if len(points) == 1:
        x, y = points[0]
        return [[x, y], [x + 1, y]]
    return points


def _drag_noop_action() -> Dict[str, Any]:
    return {"type": "sleep", "sleep": {"duration_ms": 1}}


def _translate_cua_action(action: Dict[str, Any]) -> Dict[str, Any]:
    action_type = action.get("type", "")
    if action_type == "click":
        button = action.get("button")
        if button == "back":
            return {
                "type": "press_key",
                "press_key": {"hold_keys": ["Alt"], "keys": ["Left"]},
            }
        if button == "forward":
            return {
                "type": "press_key",
                "press_key": {"hold_keys": ["Alt"], "keys": ["Right"]},
            }
        if button == "wheel":
            return {
                "type": "scroll",
                "scroll": {
                    "x": action.get("x", 0),
                    "y": action.get("y", 0),
                    "delta_x": int(action.get("scroll_x", 0)),
                    "delta_y": int(action.get("scroll_y", 0)),
                },
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
        normalized = _normalize_keypress_payload(
            action.get("keys", []), action.get("hold_keys", [])
        )
        payload: Dict[str, Any] = {"keys": normalized["keys"]}
        if normalized["hold_keys"]:
            payload["hold_keys"] = normalized["hold_keys"]
        return {"type": "press_key", "press_key": payload}
    elif action_type == "scroll":
        return {
            "type": "scroll",
            "scroll": {
                "x": action.get("x", 0),
                "y": action.get("y", 0),
                "delta_x": int(action.get("scroll_x", 0)),
                "delta_y": int(action.get("scroll_y", 0)),
            },
        }
    elif action_type == "move":
        return {"type": "move_mouse", "move_mouse": {"x": action.get("x", 0), "y": action.get("y", 0)}}
    elif action_type == "drag":
        path = _normalize_drag_path(action.get("path", []))
        if len(path) < 2:
            return _drag_noop_action()
        return {"type": "drag_mouse", "drag_mouse": {"path": path}}
    elif action_type == "wait":
        return {"type": "sleep", "sleep": {"duration_ms": action.get("ms", 1000)}}
    else:
        raise ValueError(f"Unknown CUA action type: {action_type}")


def _is_batch_computer_action_type(action_type: str) -> bool:
    return action_type in {
        "click",
        "double_click",
        "type",
        "keypress",
        "scroll",
        "move",
        "drag",
        "wait",
    }


def _goto_batch_actions(url: str) -> List[Dict[str, Any]]:
    return [
        {
            "type": "press_key",
            "press_key": {"hold_keys": ["Ctrl"], "keys": ["l"]},
        },
        {
            "type": "sleep",
            "sleep": {"duration_ms": GOTO_CHORD_DELAY_MS},
        },
        {
            "type": "press_key",
            "press_key": {"hold_keys": ["Ctrl"], "keys": ["a"]},
        },
        {
            "type": "type_text",
            "type_text": {"text": url},
        },
        {
            "type": "press_key",
            "press_key": {"keys": ["Return"]},
        },
    ]


def _back_batch_actions() -> List[Dict[str, Any]]:
    return [
        {
            "type": "press_key",
            "press_key": {"hold_keys": ["Alt"], "keys": ["Left"]},
        }
    ]


def _forward_batch_actions() -> List[Dict[str, Any]]:
    return [
        {
            "type": "press_key",
            "press_key": {"hold_keys": ["Alt"], "keys": ["Right"]},
        }
    ]


def _current_url_batch_actions() -> List[Dict[str, Any]]:
    return [
        {
            "type": "press_key",
            "press_key": {"hold_keys": ["Ctrl"], "keys": ["l"]},
        },
        {
            "type": "press_key",
            "press_key": {"hold_keys": ["Ctrl"], "keys": ["a"]},
        },
        {
            "type": "press_key",
            "press_key": {"hold_keys": ["Ctrl"], "keys": ["c"]},
        },
        {
            "type": "press_key",
            "press_key": {"keys": ["Escape"]},
        },
    ]


def _validate_batch_terminal_read_actions(actions: List[Dict[str, Any]]) -> None:
    read_idx = -1
    read_type = ""
    for idx, action in enumerate(actions):
        action_type = str(action.get("type", ""))
        if action_type not in ("url", "screenshot"):
            continue
        if read_idx >= 0:
            raise ValueError(
                f"batch can include at most one return-value action ({read_type} or {action_type}); "
                f"found {read_type} at index {read_idx} and {action_type} at index {idx}"
            )
        if idx != len(actions) - 1:
            raise ValueError(f'return-value action "{action_type}" must be last in batch')
        read_idx = idx
        read_type = action_type


def _build_pending_batch(actions: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    pending: List[Dict[str, Any]] = []
    for action in actions:
        action_type = str(action.get("type", ""))
        if _is_batch_computer_action_type(action_type):
            pending.append(_translate_cua_action(action))
            continue
        if action_type == "goto":
            pending.extend(_goto_batch_actions(str(action.get("url", ""))))
            continue
        if action_type == "back":
            pending.extend(_back_batch_actions())
            continue
        if action_type in ("url", "screenshot"):
            continue
        raise ValueError(f"Unknown CUA action type: {action_type}")
    return pending


def _describe_translated_batch(actions: List[Dict[str, Any]]) -> str:
    parts: List[str] = []
    for action in actions:
        action_type = str(action.get("type", ""))
        if action_type == "click_mouse":
            click = action.get("click_mouse", {})
            if not isinstance(click, dict):
                parts.append(action_type)
                continue
            if int(click.get("num_clicks", 0)) > 1:
                parts.append(f"double_click({int(click.get('x', 0))},{int(click.get('y', 0))})")
            else:
                parts.append(f"click({int(click.get('x', 0))},{int(click.get('y', 0))})")
            continue
        if action_type == "type_text":
            type_text = action.get("type_text", {})
            text = str(type_text.get("text", "")) if isinstance(type_text, dict) else ""
            parts.append(f"type({_truncate(text, 30)!r})")
            continue
        if action_type == "press_key":
            press_key = action.get("press_key", {})
            keys = press_key.get("keys", []) if isinstance(press_key, dict) else []
            hold_keys = (
                press_key.get("hold_keys", []) if isinstance(press_key, dict) else []
            )
            parts.append(f"key(hold={hold_keys}, keys={keys})")
            continue
        if action_type == "scroll":
            parts.append("scroll")
            continue
        if action_type == "move_mouse":
            parts.append("move")
            continue
        if action_type == "drag_mouse":
            parts.append("drag")
            continue
        if action_type == "sleep":
            sleep = action.get("sleep", {})
            duration = int(sleep.get("duration_ms", 0)) if isinstance(sleep, dict) else 0
            parts.append(f"sleep({duration}ms)")
            continue
        parts.append(action_type)
    return "batch[" + " -> ".join(parts) + "]"


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
        hold_keys = action_args.get("hold_keys", [])
        keys = action_args.get("keys", [])
        if hold_keys:
            return f"keypress(hold={hold_keys}, keys={keys})"
        return f"keypress({keys})"
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
    if action_type == "goto":
        return f"goto({action_args.get('url', '')!r})"
    if action_type == "back":
        return "back()"
    if action_type == "url":
        return "url()"
    if action_type == "screenshot":
        return "screenshot()"
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
        return (1920, 1080)

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
        if button == "back":
            self.back()
            return
        if button == "forward":
            self.forward()
            return
        if button == "wheel":
            self.scroll(x, y, 0, 0)
            return
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

    def keypress(self, keys: List[str], hold_keys: List[str] | None = None) -> None:
        normalized = _normalize_keypress_payload(keys, hold_keys or [])
        op = _describe_action(
            "keypress",
            {
                "keys": normalized["keys"],
                **({"hold_keys": normalized["hold_keys"]} if normalized["hold_keys"] else {}),
            },
        )
        self._trace_backend(
            op,
            lambda: self.client.browsers.computer.press_key(
                self.session_id,
                keys=normalized["keys"],
                **({"hold_keys": normalized["hold_keys"]} if normalized["hold_keys"] else {}),
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
            normalized_path = _normalize_drag_path(path)
            if len(normalized_path) < 2:
                time.sleep(0.001)
                return
            self.client.browsers.computer.drag_mouse(self.session_id, path=normalized_path)

        self._trace_backend(op, _do)

    def wait(self, ms: int = 1000) -> None:
        time.sleep(ms / 1000)

    def batch_actions(self, actions: List[Dict[str, Any]]) -> None:
        _validate_batch_terminal_read_actions(actions)
        pending = _build_pending_batch(actions)
        op = _describe_translated_batch(pending)

        def _do() -> None:
            if pending:
                self.client.browsers.computer.batch(self.session_id, actions=pending)

        self._trace_backend(op, _do)

    def goto(self, url: str) -> None:
        self.batch_actions([{"type": "goto", "url": url}])

    def back(self) -> None:
        self.batch_actions([{"type": "back"}])

    def forward(self) -> None:
        actions = _forward_batch_actions()
        op = _describe_translated_batch(actions)
        self._trace_backend(
            op,
            lambda: self.client.browsers.computer.batch(
                self.session_id, actions=actions
            ),
        )

    def get_current_url(self) -> str:
        def _do() -> str:
            copy_actions = _current_url_batch_actions()
            copy_op = _describe_translated_batch(copy_actions)
            self._trace_backend(
                copy_op,
                lambda: self.client.browsers.computer.batch(
                    self.session_id, actions=copy_actions
                ),
            )
            result = self.client.browsers.computer.read_clipboard(self.session_id)
            current_url = (result.text or "").strip()
            if not current_url:
                raise ValueError("clipboard URL was empty")
            return current_url

        return self._trace_backend("get_current_url()", _do)
