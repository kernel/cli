"""
Yutori n2 Computer Tool

Maps n2's `computer_batch` action list onto Kernel's Computer Controls API.
n2 plans every action in a batch against the screenshot taken *before* the batch
ran, so the batch executes sequentially, stops at the first error, and answers
with a single screenshot taken after the last action that ran.

@see https://docs.yutori.com/reference/n2
"""

from __future__ import annotations

import asyncio
import base64
from dataclasses import dataclass
from io import BytesIO
from typing import Any, Literal, Optional, TypedDict

from kernel import Kernel
from PIL import Image

from .base import ToolError

TYPING_DELAY_MS = 12
# Let the UI settle between actions in a batch, and again before the screenshot
# that the model will plan its next batch against.
INTER_ACTION_DELAY_S = 0.08
SETTLE_DELAY_S = 0.4

NAVIGATOR_COORDINATE_SCALE = 1000

# n2's scroll `amount` is a wheel notch count (1-50), which is exactly what
# Kernel's delta_x/delta_y take ("xdotool wheel units"), so it passes through.
DEFAULT_SCROLL_AMOUNT = 3

# WebP quality for screenshots. Kernel returns PNGs, which are crisp and
# tolerate aggressive WebP compression with no visible degradation — matches
# Yutori SDK's DEFAULT_WEBP_QUALITY_FOR_PNG=30 (yutori-sdk-python/yutori/
# navigator/images.py). Requests are capped at 10 MB and a trajectory carries
# several full-screen captures, so the compression matters.
WEBP_QUALITY = 30

N2ActionName = Literal[
    "left_click",
    "double_click",
    "triple_click",
    "middle_click",
    "right_click",
    "scroll",
    "type",
    "key_press",
    "drag",
    "mouse_move",
    "mouse_down",
    "mouse_up",
    "hold_key",
    "wait",
    "screenshot",
]


class N2Action(TypedDict, total=False):
    """One `{name, arguments}` member of a `computer_batch` call."""

    name: N2ActionName
    arguments: dict[str, Any]


@dataclass
class BatchOutcome:
    executed: int
    total: int
    # Set when an action raised; the remaining actions were skipped.
    failed_index: Optional[int] = None
    failed_name: Optional[str] = None
    failed_message: Optional[str] = None

    def describe(self) -> str:
        if self.failed_index is None:
            return f"Executed {self.executed} of {self.total} actions."
        return (
            f"Executed {self.executed} of {self.total} actions. "
            f"Action {self.failed_index + 1} ({self.failed_name}) failed: {self.failed_message}. "
            f"The remaining actions were skipped."
        )


# n2 emits lowercase key names (e.g. `enter`, `ctrl+c`, `down down down enter`).
# Kernel's press_key expects XKeysym names (e.g. `Return`, `Ctrl`, `Page_Up`).
# This map covers every key Yutori documents at
# https://docs.yutori.com/reference/n2#key-space — keys not in the map pass
# through unchanged (printable characters like `a` and `1` are already XKeysym).
#
# Sister implementation (Playwright target instead of XKeysym):
# https://github.com/yutori-ai/yutori-sdk-python/blob/main/yutori/navigator/keys.py
KEY_MAP: dict[str, str] = {
    # Modifiers
    "ctrl": "Ctrl",
    "control": "Ctrl",
    "shift": "Shift",
    "alt": "Alt",
    "meta": "Super_L",
    "command": "Super_L",
    "cmd": "Super_L",
    "super": "Super_L",
    "option": "Alt",
    # Enter
    "enter": "Return",
    "return": "Return",
    # Navigation
    "tab": "Tab",
    "backspace": "BackSpace",
    "delete": "Delete",
    "escape": "Escape",
    "esc": "Escape",
    "space": "space",
    # Arrows
    "up": "Up",
    "down": "Down",
    "left": "Left",
    "right": "Right",
    "arrowup": "Up",
    "arrowdown": "Down",
    "arrowleft": "Left",
    "arrowright": "Right",
    # Page nav
    "home": "Home",
    "end": "End",
    "pageup": "Page_Up",
    "pagedown": "Page_Down",
    # Function keys
    **{f"f{i}": f"F{i}" for i in range(1, 13)},
    # Punctuation — n2 sends word forms, most of which are already XKeysym names
    "minus": "minus",
    "plus": "plus",
    "equal": "equal",
    "comma": "comma",
    "period": "period",
    "slash": "slash",
    "backslash": "backslash",
    "semicolon": "semicolon",
    "quote": "apostrophe",
    "backquote": "grave",
    "bracketleft": "bracketleft",
    "bracketright": "bracketright",
    # Locks / special
    "capslock": "Caps_Lock",
    "numlock": "Num_Lock",
    "scrolllock": "Scroll_Lock",
    "insert": "Insert",
    "pause": "Pause",
    "printscreen": "Print",
}


def _map_token(token: str) -> str:
    return KEY_MAP.get(token.strip().lower(), token.strip())


def _parse_key_expression(expr: str) -> list[str]:
    """Parse an n2 key expression into one Kernel combo per sequential press.

    Spaces separate sequential presses; '+' separates simultaneous tokens
    within a press. Examples:
        "enter"             -> ["Return"]
        "ctrl+c"            -> ["Ctrl+c"]
        "down down enter"   -> ["Down", "Down", "Return"]
        "ctrl+shift+t"      -> ["Ctrl+Shift+t"]
    """
    return [
        "+".join(_map_token(token) for token in combo.split("+"))
        for combo in expr.strip().split()
        if combo
    ]


def _duration_ms(duration: Any, fallback_ms: int) -> int:
    """n2 emits `duration` in seconds; Kernel takes milliseconds."""
    if isinstance(duration, (int, float)) and duration > 0:
        return int(duration * 1000)
    return fallback_ms


class ComputerTool:
    def __init__(self, kernel: Kernel, session_id: str, screen_width: int = 1280, screen_height: int = 800):
        self.kernel = kernel
        self.session_id = session_id
        self.screen_width = screen_width
        self.screen_height = screen_height

    async def run_batch(self, actions: list[N2Action]) -> BatchOutcome:
        """Run a `computer_batch` action list in order, stopping at the first failure.

        The caller reports the outcome and a single post-batch screenshot back to n2.
        """
        for index, action in enumerate(actions):
            try:
                await self._run_action(action)
            except Exception as error:
                return BatchOutcome(
                    executed=index,
                    total=len(actions),
                    failed_index=index,
                    failed_name=str(action.get("name")),
                    failed_message=str(error),
                )
            if index < len(actions) - 1:
                await asyncio.sleep(INTER_ACTION_DELAY_S)

        return BatchOutcome(executed=len(actions), total=len(actions))

    async def _run_action(self, action: N2Action) -> None:
        name = action.get("name")
        args = action.get("arguments") or {}

        if name in ("left_click", "double_click", "triple_click"):
            num_clicks = {"left_click": 1, "double_click": 2, "triple_click": 3}[name]
            return self._click(args, "left", num_clicks)
        if name == "middle_click":
            return self._click(args, "middle", 1)
        if name == "right_click":
            return self._click(args, "right", 1)
        if name == "scroll":
            return self._scroll(args)
        if name == "type":
            return self._type(args)
        if name == "key_press":
            return self._key_press(args)
        if name == "drag":
            return self._drag(args)
        if name == "mouse_move":
            return self._mouse_move(args)
        if name in ("mouse_down", "mouse_up"):
            return self._mouse_button(args, name.split("_")[1])
        if name == "hold_key":
            return self._hold_key(args)
        if name == "wait":
            return await asyncio.sleep(_duration_ms(args.get("duration"), 2000) / 1000)
        if name == "screenshot":
            # A batch already answers with a screenshot taken after its last
            # action, so an explicit `screenshot` member has nothing to do.
            return None

        raise ToolError(f"Unknown action: {name}")

    def _click(self, args: dict[str, Any], button: str, num_clicks: int) -> None:
        x, y = self._require_coordinates(args.get("coordinates"))
        kwargs: dict[str, Any] = {
            "x": x,
            "y": y,
            "button": button,
            "click_type": "click",
            "num_clicks": num_clicks,
        }
        if args.get("modifier"):
            kwargs["hold_keys"] = [_map_token(args["modifier"])]

        self.kernel.browsers.computer.click_mouse(self.session_id, **kwargs)

    def _mouse_move(self, args: dict[str, Any]) -> None:
        x, y = self._require_coordinates(args.get("coordinates"))
        self.kernel.browsers.computer.move_mouse(self.session_id, x=x, y=y)

    def _mouse_button(self, args: dict[str, Any], click_type: str) -> None:
        # Coordinates are optional here — without them the button is pressed or
        # released wherever the cursor already is, which is what a manual
        # mouse_move -> mouse_down -> mouse_move -> mouse_up drag relies on.
        if args.get("coordinates"):
            x, y = self._require_coordinates(args["coordinates"])
        else:
            position = self.kernel.browsers.computer.get_mouse_position(self.session_id)
            x, y = position.x, position.y

        self.kernel.browsers.computer.click_mouse(
            self.session_id,
            x=x,
            y=y,
            button="left",
            click_type=click_type,
        )

    def _scroll(self, args: dict[str, Any]) -> None:
        x, y = self._require_coordinates(args.get("coordinates"))
        direction = args.get("direction")

        # n2 only scrolls vertically.
        if direction not in ("up", "down"):
            raise ToolError(f"Invalid scroll direction: {direction}")

        notches = max(1, round(args.get("amount") or DEFAULT_SCROLL_AMOUNT))
        kwargs: dict[str, Any] = {
            "x": x,
            "y": y,
            "delta_x": 0,
            "delta_y": -notches if direction == "up" else notches,
        }
        if args.get("modifier"):
            kwargs["hold_keys"] = [_map_token(args["modifier"])]

        self.kernel.browsers.computer.scroll(self.session_id, **kwargs)

    def _type(self, args: dict[str, Any]) -> None:
        text = args.get("text")
        if not text:
            raise ToolError("text is required for type")

        self.kernel.browsers.computer.type_text(self.session_id, text=text, delay=TYPING_DELAY_MS)

    def _key_press(self, args: dict[str, Any]) -> None:
        key = args.get("key")
        if not key:
            raise ToolError("key is required for key_press")

        # n2 supports sequential presses ("down down down enter") — issue each
        # combo as its own press_key so they're seen as separate keystrokes.
        for combo in _parse_key_expression(key):
            self.kernel.browsers.computer.press_key(self.session_id, keys=[combo])

    def _hold_key(self, args: dict[str, Any]) -> None:
        key = args.get("key")
        if not key:
            raise ToolError("key is required for hold_key")

        for combo in _parse_key_expression(key):
            self.kernel.browsers.computer.press_key(
                self.session_id,
                keys=[combo],
                duration=_duration_ms(args.get("duration"), 1000),
            )

    def _drag(self, args: dict[str, Any]) -> None:
        start_x, start_y = self._require_coordinates(args.get("start_coordinates"))
        end_x, end_y = self._require_coordinates(args.get("coordinates"))

        self.kernel.browsers.computer.drag_mouse(
            self.session_id,
            path=[[start_x, start_y], [end_x, end_y]],
            button="left",
        )

    async def screenshot(self) -> str:
        """Capture the whole screen — browser chrome included — as base64 WebP."""
        await asyncio.sleep(SETTLE_DELAY_S)

        response = self.kernel.browsers.computer.capture_screenshot(self.session_id)
        image = Image.open(BytesIO(response.read()))
        webp_buffer = BytesIO()
        image.save(webp_buffer, "WEBP", quality=WEBP_QUALITY)

        return base64.b64encode(webp_buffer.getvalue()).decode("utf-8")

    def _require_coordinates(self, coords: Any) -> tuple[int, int]:
        """Map [0, 1000] coordinates to screen pixels, clamped to [0, dim-1].

        Clamping prevents a boundary value like 1000 from landing one pixel
        outside the screen on a 1280x800 display.
        """
        if not isinstance(coords, (list, tuple)) or len(coords) != 2:
            raise ToolError(f"coordinates are required, got {coords!r}")

        nx, ny = coords
        if not isinstance(nx, (int, float)) or not isinstance(ny, (int, float)):
            raise ToolError(f"Invalid coordinates: {coords!r}")

        x = round(nx / NAVIGATOR_COORDINATE_SCALE * self.screen_width)
        y = round(ny / NAVIGATOR_COORDINATE_SCALE * self.screen_height)

        return (
            max(0, min(self.screen_width - 1, x)),
            max(0, min(self.screen_height - 1, y)),
        )
