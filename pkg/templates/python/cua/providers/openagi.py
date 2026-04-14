"""OpenAGI CUA provider adapter using the Lux computer-use models.

Uses the oagi SDK's AsyncDefaultAgent with Kernel's Computer Controls API.
Only available in Python (no TypeScript SDK exists).

Note: pyautogui and mouseinfo must be mocked before importing oagi to prevent
X11 connection errors in headless environments.

@see https://agiopen.org
"""

from __future__ import annotations

import asyncio
import io
import os
import sys
import time
from types import ModuleType
from importlib.machinery import ModuleSpec

# Mock pyautogui and mouseinfo before importing oagi — they require X11
# but we use Kernel's Computer Controls API instead.
if "mouseinfo" not in sys.modules:
    _mock_mouseinfo = ModuleType("mouseinfo")
    _mock_mouseinfo.__spec__ = ModuleSpec("mouseinfo", None)
    sys.modules["mouseinfo"] = _mock_mouseinfo

if "pyautogui" not in sys.modules:
    _mock_pyautogui = ModuleType("pyautogui")
    _mock_pyautogui.__spec__ = ModuleSpec("pyautogui", None)
    sys.modules["pyautogui"] = _mock_pyautogui

from oagi import AsyncDefaultAgent
from oagi.types.models.action import (
    Action,
    ActionType,
    parse_coords,
    parse_drag_coords,
    parse_scroll,
)
from PIL import Image as PILImage

from . import CuaProvider, TaskOptions, TaskResult

DEFAULT_MODEL = "lux-actor-1"

# Key mappings from pyautogui/Lux format to xdotool format
XDOTOOL_KEY_MAP = {
    "enter": "Return", "return": "Return",
    "escape": "Escape", "esc": "Escape",
    "backspace": "BackSpace", "tab": "Tab", "space": "space",
    "up": "Up", "down": "Down", "left": "Left", "right": "Right",
    "pageup": "Page_Up", "page_up": "Page_Up", "pgup": "Page_Up",
    "pagedown": "Page_Down", "page_down": "Page_Down", "pgdn": "Page_Down",
    "home": "Home", "end": "End",
    "insert": "Insert", "delete": "Delete", "del": "Delete",
    "ctrl": "ctrl", "control": "ctrl",
    "alt": "alt", "shift": "shift",
    "super": "super", "win": "super", "command": "super", "cmd": "super", "meta": "super",
    **{f"f{i}": f"F{i}" for i in range(1, 13)},
}


def _translate_key(key: str) -> str:
    k = key.strip().lower()
    if k in XDOTOOL_KEY_MAP:
        return XDOTOOL_KEY_MAP[k]
    if len(key) == 1:
        return key
    return key.capitalize()


def _parse_hotkey(args_str: str) -> list[str]:
    args_str = args_str.strip("()")
    keys = [_translate_key(k.strip()) for k in args_str.split("+")]
    if len(keys) > 1:
        return ["+".join(keys)]
    return keys


class _KernelImage:
    """Lightweight Image wrapper implementing the oagi Image protocol."""

    def __init__(self, data: bytes, width: int | None = None, height: int | None = None):
        self._data = data
        self._width = width
        self._height = height

    def read(self) -> bytes:
        return self._data

    def resize(self, width: int, height: int) -> "_KernelImage":
        img = PILImage.open(io.BytesIO(self._data))
        resized = img.resize((width, height), PILImage.Resampling.LANCZOS)
        buf = io.BytesIO()
        if resized.mode == "RGBA":
            rgb = PILImage.new("RGB", resized.size, (255, 255, 255))
            rgb.paste(resized, mask=resized.split()[3])
            resized = rgb
        resized.save(buf, format="JPEG", quality=85)
        return _KernelImage(buf.getvalue(), width, height)


class OpenAGIProvider:
    name = "openagi"

    def __init__(self) -> None:
        self._api_key = os.environ.get("OAGI_API_KEY", "")

    def is_configured(self) -> bool:
        return len(self._api_key) > 0

    async def run_task(self, options: TaskOptions) -> TaskResult:
        model = options.model or DEFAULT_MODEL
        computer = options.kernel.browsers.computer
        width = options.viewport_width
        height = options.viewport_height

        # Set base URL for oagi
        os.environ.setdefault("OAGI_BASE_URL", "https://api.agiopen.org")

        # Screenshot provider (implements AsyncImageProvider protocol)
        last_image: _KernelImage | None = None

        async def take_screenshot() -> _KernelImage:
            nonlocal last_image
            res = computer.capture_screenshot(options.session_id)
            raw = res.read()
            img = _KernelImage(raw)
            img = img.resize(1260, 700)
            last_image = img
            return img

        async def get_last_image() -> _KernelImage:
            if last_image is None:
                return await take_screenshot()
            return last_image

        # Action handler (implements AsyncActionHandler protocol)
        def denormalize(x: int, y: int) -> tuple[int, int]:
            sx = max(1, min(int(x * width / 1000), width - 1))
            sy = max(1, min(int(y * height / 1000), height - 1))
            return sx, sy

        def execute_single_action(action: Action) -> None:
            arg = action.argument.strip("()")

            match action.type:
                case ActionType.CLICK:
                    coords = parse_coords(arg)
                    if not coords:
                        raise ValueError(f"Invalid coordinates: {arg}")
                    x, y = denormalize(coords[0], coords[1])
                    computer.click_mouse(options.session_id, x=x, y=y)

                case ActionType.LEFT_DOUBLE:
                    coords = parse_coords(arg)
                    if not coords:
                        raise ValueError(f"Invalid coordinates: {arg}")
                    x, y = denormalize(coords[0], coords[1])
                    computer.click_mouse(options.session_id, x=x, y=y, num_clicks=2)

                case ActionType.LEFT_TRIPLE:
                    coords = parse_coords(arg)
                    if not coords:
                        raise ValueError(f"Invalid coordinates: {arg}")
                    x, y = denormalize(coords[0], coords[1])
                    computer.click_mouse(options.session_id, x=x, y=y, num_clicks=3)

                case ActionType.RIGHT_SINGLE:
                    coords = parse_coords(arg)
                    if not coords:
                        raise ValueError(f"Invalid coordinates: {arg}")
                    x, y = denormalize(coords[0], coords[1])
                    computer.click_mouse(options.session_id, x=x, y=y, button="right")

                case ActionType.DRAG:
                    coords = parse_drag_coords(arg)
                    if not coords:
                        raise ValueError(f"Invalid drag coordinates: {arg}")
                    x1, y1 = denormalize(coords[0], coords[1])
                    x2, y2 = denormalize(coords[2], coords[3])
                    computer.drag_mouse(
                        options.session_id, path=[[x1, y1], [x2, y2]], button="left",
                    )

                case ActionType.HOTKEY:
                    keys = _parse_hotkey(arg)
                    computer.press_key(options.session_id, keys=keys)

                case ActionType.TYPE:
                    text = arg.strip("\"'")
                    press_enter = text.endswith("\n") or text.endswith("\\n")
                    if press_enter:
                        text = text.rstrip("\n").rstrip("\\n")
                    computer.type_text(options.session_id, text=text, delay=50)
                    if press_enter:
                        computer.press_key(options.session_id, keys=["Return"])

                case ActionType.SCROLL:
                    result = parse_scroll(arg)
                    if not result:
                        raise ValueError(f"Invalid scroll format: {arg}")
                    x, y = denormalize(result[0], result[1])
                    direction = result[2]
                    dx = dy = 0
                    if direction == "up": dy = -1
                    elif direction == "down": dy = 1
                    elif direction == "left": dx = -1
                    elif direction == "right": dx = 1
                    computer.scroll(
                        options.session_id, x=x, y=y, delta_x=dx, delta_y=dy,
                    )

                case ActionType.FINISH:
                    pass

                case ActionType.WAIT:
                    time.sleep(1.0)

                case ActionType.CALL_USER:
                    pass

                case _:
                    print(f"Unknown action type: {action.type}")

        def execute_action(action: Action) -> None:
            count = action.count or 1
            for _ in range(count):
                execute_single_action(action)
                if count > 1:
                    time.sleep(0.1)

        async def handle_actions(actions: list[Action]) -> None:
            for action in actions:
                await asyncio.get_event_loop().run_in_executor(
                    None, execute_action, action,
                )
                await asyncio.sleep(0.1)

        # Run the agent
        agent = AsyncDefaultAgent(
            api_key=self._api_key,
            max_steps=20,
            model=model,
        )

        success = await agent.execute(
            instruction=options.query,
            action_handler=handle_actions,
            image_provider=take_screenshot,
        )

        # The oagi agent doesn't return text results directly — report success/failure
        return TaskResult(
            result=f"Task completed. Success: {success}",
            provider=self.name,
        )
