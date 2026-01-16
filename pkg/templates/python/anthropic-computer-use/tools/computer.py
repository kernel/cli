"""
Computer tool using Kernel's Computer Controls API.
Modified from https://github.com/anthropics/anthropic-quickstarts/blob/main/computer-use-demo/computer_use_demo/tools/computer.py
Replaces Playwright with Kernel Computer Controls API.
"""

import asyncio
import base64
from typing import Literal, TypedDict, cast, get_args

from kernel import Kernel
from anthropic.types.beta import BetaToolComputerUse20241022Param, BetaToolUnionParam

from .base import BaseAnthropicTool, ToolError, ToolResult

TYPING_DELAY_MS = 12

# Key mappings for Kernel Computer Controls API
# Map common key names to xdotool-compatible format that Kernel uses
KEY_MAP = {
    'return': 'Return',
    'enter': 'Return',
    'space': 'space',
    'left': 'Left',
    'right': 'Right',
    'up': 'Up',
    'down': 'Down',
    'home': 'Home',
    'end': 'End',
    'pageup': 'Page_Up',
    'page_up': 'Page_Up',
    'pagedown': 'Page_Down',
    'page_down': 'Page_Down',
    'delete': 'Delete',
    'backspace': 'BackSpace',
    'tab': 'Tab',
    'esc': 'Escape',
    'escape': 'Escape',
    'insert': 'Insert',
    'f1': 'F1',
    'f2': 'F2',
    'f3': 'F3',
    'f4': 'F4',
    'f5': 'F5',
    'f6': 'F6',
    'f7': 'F7',
    'f8': 'F8',
    'f9': 'F9',
    'f10': 'F10',
    'f11': 'F11',
    'f12': 'F12',
    'minus': 'minus',
    'equal': 'equal',
    'plus': 'plus',
}

# Modifier key mappings
MODIFIER_KEY_MAP = {
    'ctrl': 'ctrl',
    'control': 'ctrl',
    'alt': 'alt',
    'cmd': 'super',
    'command': 'super',
    'win': 'super',
    'meta': 'super',
    'shift': 'shift',
}

Action_20241022 = Literal[
    "key",
    "type",
    "mouse_move",
    "left_click",
    "left_click_drag",
    "right_click",
    "middle_click",
    "double_click",
    "screenshot",
    "cursor_position",
]

Action_20250124 = (
    Action_20241022
    | Literal[
        "left_mouse_down",
        "left_mouse_up",
        "scroll",
        "hold_key",
        "wait",
        "triple_click",
    ]
)

ScrollDirection = Literal["up", "down", "left", "right"]


class ComputerToolOptions(TypedDict):
    display_height_px: int
    display_width_px: int
    display_number: int | None


class BaseComputerTool:
    """
    A tool that allows the agent to interact with the screen, keyboard, and mouse using Kernel's Computer Controls API.
    The tool parameters are defined by Anthropic and are not editable.
    """

    name: Literal["computer"] = "computer"
    width: int = 1024
    height: int = 768
    display_num: int | None = None
    
    # Kernel client and session
    kernel: Kernel | None = None
    session_id: str | None = None
    
    # Track last mouse position for drag operations
    _last_mouse_position: tuple[int, int] = (0, 0)
    _screenshot_delay = 2.0

    @property
    def options(self) -> ComputerToolOptions:
        return {
            "display_width_px": self.width,
            "display_height_px": self.height,
            "display_number": self.display_num,
        }

    def __init__(self, kernel: Kernel | None = None, session_id: str | None = None):
        super().__init__()
        self.kernel = kernel
        self.session_id = session_id

    def validate_coordinates(self, coordinate: tuple[int, int] | list[int] | None = None) -> tuple[int, int] | None:
        """Validate that coordinates are non-negative integers and convert lists to tuples if needed."""
        if coordinate is None:
            return None
            
        # Convert list to tuple if needed
        if isinstance(coordinate, list):
            coordinate = tuple(coordinate)
            
        if not isinstance(coordinate, tuple) or len(coordinate) != 2:
            raise ToolError(f"{coordinate} must be a tuple or list of length 2")
            
        x, y = coordinate
        if not isinstance(x, int) or not isinstance(y, int) or x < 0 or y < 0:
            raise ToolError(f"{coordinate} must be a tuple or list of non-negative ints")
            
        return coordinate

    def map_key(self, key: str) -> str:
        """Map a key to its Kernel/xdotool equivalent."""
        key_lower = key.lower().strip()
        
        # Handle modifier keys
        if key_lower in MODIFIER_KEY_MAP:
            return MODIFIER_KEY_MAP[key_lower]
        
        # Handle special keys
        if key_lower in KEY_MAP:
            return KEY_MAP[key_lower]
        
        # Handle key combinations (e.g. "ctrl+a")
        if '+' in key:
            parts = key.split('+')
            mapped_parts = []
            for part in parts:
                part = part.strip().lower()
                if part in MODIFIER_KEY_MAP:
                    mapped_parts.append(MODIFIER_KEY_MAP[part])
                elif part in KEY_MAP:
                    mapped_parts.append(KEY_MAP[part])
                else:
                    mapped_parts.append(part)
            return '+'.join(mapped_parts)
        
        # Return the key as is if no mapping exists
        return key

    async def __call__(
        self,
        *,
        action: Action_20241022,
        text: str | None = None,
        coordinate: tuple[int, int] | list[int] | None = None,
        **kwargs,
    ):
        if not self.kernel or not self.session_id:
            raise ToolError("Kernel client or session not initialized")

        if action in ("mouse_move", "left_click_drag"):
            if coordinate is None:
                raise ToolError(f"coordinate is required for {action}")
            if text is not None:
                raise ToolError(f"text is not accepted for {action}")

            coordinate = self.validate_coordinates(coordinate)
            x, y = coordinate

            if action == "mouse_move":
                self.kernel.browsers.computer.move_mouse(
                    id=self.session_id,
                    x=x,
                    y=y,
                )
                self._last_mouse_position = (x, y)
                return await self.screenshot()
            elif action == "left_click_drag":
                start_coord = kwargs.get("start_coordinate")
                start_x, start_y = self.validate_coordinates(start_coord) if start_coord else self._last_mouse_position
                
                print(f"Dragging from ({start_x}, {start_y}) to ({x}, {y})")
                
                self.kernel.browsers.computer.drag_mouse(
                    id=self.session_id,
                    path=[[start_x, start_y], [x, y]],
                    button="left",
                )
                self._last_mouse_position = (x, y)
                return await self.screenshot()

        if action in ("key", "type"):
            if text is None:
                raise ToolError(f"text is required for {action}")
            if coordinate is not None:
                raise ToolError(f"coordinate is not accepted for {action}")
            if not isinstance(text, str):
                raise ToolError(f"{text} must be a string")

            if action == "key":
                mapped_key = self.map_key(text)
                self.kernel.browsers.computer.press_key(
                    id=self.session_id,
                    keys=[mapped_key],
                )
                return await self.screenshot()
            elif action == "type":
                self.kernel.browsers.computer.type_text(
                    id=self.session_id,
                    text=text,
                    delay=TYPING_DELAY_MS,
                )
                return await self.screenshot()

        if action in (
            "left_click",
            "right_click",
            "double_click",
            "middle_click",
            "screenshot",
            "cursor_position",
        ):
            if text is not None:
                raise ToolError(f"text is not accepted for {action}")

            if action == "screenshot":
                return await self.screenshot()
            elif action == "cursor_position":
                # Kernel Computer Controls API doesn't track cursor position
                raise ToolError("Cursor position is not available with Kernel Computer Controls API")
            else:
                if coordinate is not None:
                    coordinate = self.validate_coordinates(coordinate)
                    x, y = coordinate
                else:
                    x, y = self._last_mouse_position
                
                button = "left"
                if action == "right_click":
                    button = "right"
                elif action == "middle_click":
                    button = "middle"
                
                num_clicks = 1
                if action == "double_click":
                    num_clicks = 2
                
                self.kernel.browsers.computer.click_mouse(
                    id=self.session_id,
                    x=x,
                    y=y,
                    button=button,
                    num_clicks=num_clicks,
                )
                self._last_mouse_position = (x, y)
                return await self.screenshot()

        raise ToolError(f"Invalid action: {action}")

    async def screenshot(self):
        """Take a screenshot using Kernel Computer Controls API and return the base64 encoded image."""
        if not self.kernel or not self.session_id:
            raise ToolError("Kernel client or session not initialized")

        print("Starting screenshot...")
        await asyncio.sleep(self._screenshot_delay)
        
        response = self.kernel.browsers.computer.capture_screenshot(id=self.session_id)
        screenshot_bytes = response.read()
        
        print(f"Screenshot taken, size: {len(screenshot_bytes)} bytes")
        
        return ToolResult(
            base64_image=base64.b64encode(screenshot_bytes).decode()
        )


class ComputerTool20241022(BaseComputerTool, BaseAnthropicTool):
    api_type: Literal["computer_20241022"] = "computer_20241022"

    def to_params(self) -> BetaToolComputerUse20241022Param:
        return {"name": self.name, "type": self.api_type, **self.options}


class ComputerTool20250124(BaseComputerTool, BaseAnthropicTool):
    api_type: Literal["computer_20250124"] = "computer_20250124"

    def to_params(self):
        return cast(
            BetaToolUnionParam,
            {"name": self.name, "type": self.api_type, **self.options},
        )

    async def __call__(
        self,
        *,
        action: Action_20250124,
        text: str | None = None,
        coordinate: tuple[int, int] | list[int] | None = None,
        scroll_direction: ScrollDirection | None = None,
        scroll_amount: int | None = None,
        duration: int | float | None = None,
        key: str | None = None,
        **kwargs,
    ):
        if not self.kernel or not self.session_id:
            raise ToolError("Kernel client or session not initialized")

        if action in ("left_mouse_down", "left_mouse_up"):
            if coordinate is not None:
                coordinate = self.validate_coordinates(coordinate)
                x, y = coordinate
            else:
                x, y = self._last_mouse_position
                
            click_type = "down" if action == "left_mouse_down" else "up"
            self.kernel.browsers.computer.click_mouse(
                id=self.session_id,
                x=x,
                y=y,
                button="left",
                click_type=click_type,
            )
            self._last_mouse_position = (x, y)
            return await self.screenshot()

        if action == "scroll":
            if scroll_direction is None or scroll_direction not in get_args(ScrollDirection):
                raise ToolError(
                    f"{scroll_direction=} must be 'up', 'down', 'left', or 'right'"
                )
            if not isinstance(scroll_amount, int) or scroll_amount < 0:
                raise ToolError(f"{scroll_amount=} must be a non-negative int")

            if coordinate is not None:
                coordinate = self.validate_coordinates(coordinate)
                x, y = coordinate
            else:
                x, y = self._last_mouse_position

            # Each scroll_amount unit = 1 scroll wheel click ≈ 120 pixels (matches Anthropic's xdotool behavior)
            scroll_factor = scroll_amount * 120
            
            delta_x = 0
            delta_y = 0
            if scroll_direction == "up":
                delta_y = -scroll_factor
            elif scroll_direction == "down":
                delta_y = scroll_factor
            elif scroll_direction == "left":
                delta_x = -scroll_factor
            elif scroll_direction == "right":
                delta_x = scroll_factor

            print(f"Scrolling {abs(delta_x) if delta_x != 0 else abs(delta_y)} pixels {scroll_direction}")

            self.kernel.browsers.computer.scroll(
                id=self.session_id,
                x=x,
                y=y,
                delta_x=delta_x,
                delta_y=delta_y,
            )
            return await self.screenshot()

        if action in ("hold_key", "wait"):
            if duration is None or not isinstance(duration, (int, float)):
                raise ToolError(f"{duration=} must be a number")
            if duration < 0:
                raise ToolError(f"{duration=} must be non-negative")
            if duration > 100:
                raise ToolError(f"{duration=} is too long.")

            if action == "hold_key":
                if text is None:
                    raise ToolError(f"text is required for {action}")
                mapped_key = self.map_key(text)
                self.kernel.browsers.computer.press_key(
                    id=self.session_id,
                    keys=[mapped_key],
                    duration=int(duration * 1000),  # Convert to milliseconds
                )
                return await self.screenshot()

            if action == "wait":
                await asyncio.sleep(duration)
                return await self.screenshot()

        if action in (
            "left_click",
            "right_click",
            "double_click",
            "triple_click",
            "middle_click",
        ):
            if text is not None:
                raise ToolError(f"text is not accepted for {action}")

            if coordinate is not None:
                coordinate = self.validate_coordinates(coordinate)
                x, y = coordinate
            else:
                x, y = self._last_mouse_position

            button = "left"
            if action == "right_click":
                button = "right"
            elif action == "middle_click":
                button = "middle"
            
            num_clicks = 1
            if action == "double_click":
                num_clicks = 2
            elif action == "triple_click":
                num_clicks = 3

            if key:
                mapped_key = self.map_key(key)
                self.kernel.browsers.computer.press_key(
                    id=self.session_id,
                    keys=[mapped_key],
                    click_type="down",
                )

            self.kernel.browsers.computer.click_mouse(
                id=self.session_id,
                x=x,
                y=y,
                button=button,
                num_clicks=num_clicks,
            )

            if key:
                self.kernel.browsers.computer.press_key(
                    id=self.session_id,
                    keys=[mapped_key],
                    click_type="up",
                )

            self._last_mouse_position = (x, y)
            return await self.screenshot()

        return await super().__call__(
            action=action, text=text, coordinate=coordinate, key=key, **kwargs
        )
