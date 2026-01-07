"""
Kernel Action Handler.

Implements the AsyncActionHandler protocol using Kernel's Computer Controls API.
"""

import asyncio
import time
from typing import TYPE_CHECKING

from oagi.types.models.action import (
    Action,
    ActionType,
    parse_coords,
    parse_drag_coords,
    parse_scroll,
)

if TYPE_CHECKING:
    from kernel_session import KernelBrowserSession


class KernelActionHandler:
    """
    Action handler using Kernel's Computer Controls API.

    Implements the AsyncActionHandler protocol:
    - __call__(actions: list[Action]) -> None: Execute a list of actions

    Maps Lux action types to Kernel Computer Controls:
    - CLICK -> click_mouse(x, y)
    - LEFT_DOUBLE -> click_mouse(x, y, num_clicks=2)
    - LEFT_TRIPLE -> click_mouse(x, y, num_clicks=3)
    - RIGHT_SINGLE -> click_mouse(x, y, button="right")
    - DRAG -> drag_mouse(path=[[x1,y1], [x2,y2]])
    - HOTKEY -> press_key(keys=[...])
    - TYPE -> type_text(text=...)
    - SCROLL -> scroll(x, y, delta_y=...)
    """

    def __init__(
        self,
        session: "KernelBrowserSession",
        action_pause: float = 0.1,
        scroll_amount: int = 100,
        wait_duration: float = 1.0,
        type_delay: int = 50,
    ):
        """
        Initialize the action handler.

        Args:
            session: The Kernel browser session to control
            action_pause: Pause between actions in seconds
            scroll_amount: Amount to scroll (pixels)
            wait_duration: Duration for wait actions in seconds
            type_delay: Delay between keystrokes in milliseconds
        """
        self.session = session
        self.action_pause = action_pause
        self.scroll_amount = scroll_amount
        self.wait_duration = wait_duration
        self.type_delay = type_delay

    def _denormalize_coords(self, x: int, y: int) -> tuple[int, int]:
        """
        Convert coordinates from 0-1000 range to actual screen coordinates.

        Lux model uses normalized 0-1000 coordinate system.
        """
        screen_x = int(x * self.session.viewport_width / 1000)
        screen_y = int(y * self.session.viewport_height / 1000)

        # Clamp to valid range
        screen_x = max(1, min(screen_x, self.session.viewport_width - 1))
        screen_y = max(1, min(screen_y, self.session.viewport_height - 1))

        return screen_x, screen_y

    def _parse_coords(self, args_str: str) -> tuple[int, int]:
        """Extract and denormalize x, y coordinates from argument string."""
        coords = parse_coords(args_str)
        if not coords:
            raise ValueError(f"Invalid coordinates format: {args_str}")
        return self._denormalize_coords(coords[0], coords[1])

    def _parse_drag_coords(self, args_str: str) -> tuple[int, int, int, int]:
        """Extract and denormalize drag coordinates from argument string."""
        coords = parse_drag_coords(args_str)
        if not coords:
            raise ValueError(f"Invalid drag coordinates format: {args_str}")
        x1, y1 = self._denormalize_coords(coords[0], coords[1])
        x2, y2 = self._denormalize_coords(coords[2], coords[3])
        return x1, y1, x2, y2

    def _parse_scroll(self, args_str: str) -> tuple[int, int, str]:
        """Extract and denormalize scroll parameters from argument string."""
        result = parse_scroll(args_str)
        if not result:
            raise ValueError(f"Invalid scroll format: {args_str}")
        x, y = self._denormalize_coords(result[0], result[1])
        return x, y, result[2]

    # Mapping from pyautogui/Lux key names to xdotool key names
    # Kernel uses xdotool underneath which has specific key naming conventions
    XDOTOOL_KEY_MAP = {
        # Enter/Return
        "enter": "Return",
        "return": "Return",
        # Escape
        "escape": "Escape",
        "esc": "Escape",
        # Backspace
        "backspace": "BackSpace",
        # Tab
        "tab": "Tab",
        # Space
        "space": "space",
        # Arrow keys
        "up": "Up",
        "down": "Down",
        "left": "Left",
        "right": "Right",
        # Page navigation
        "pageup": "Page_Up",
        "page_up": "Page_Up",
        "pgup": "Page_Up",
        "pagedown": "Page_Down",
        "page_down": "Page_Down",
        "pgdn": "Page_Down",
        # Home/End
        "home": "Home",
        "end": "End",
        # Insert/Delete
        "insert": "Insert",
        "delete": "Delete",
        "del": "Delete",
        # Function keys
        "f1": "F1",
        "f2": "F2",
        "f3": "F3",
        "f4": "F4",
        "f5": "F5",
        "f6": "F6",
        "f7": "F7",
        "f8": "F8",
        "f9": "F9",
        "f10": "F10",
        "f11": "F11",
        "f12": "F12",
        # Modifier keys
        "ctrl": "ctrl",
        "control": "ctrl",
        "alt": "alt",
        "shift": "shift",
        "super": "super",
        "win": "super",
        "command": "super",
        "cmd": "super",
        "meta": "super",
        # Caps lock
        "capslock": "Caps_Lock",
        "caps_lock": "Caps_Lock",
        "caps": "Caps_Lock",
        # Print screen
        "printscreen": "Print",
        "print_screen": "Print",
        "prtsc": "Print",
        # Scroll lock
        "scrolllock": "Scroll_Lock",
        "scroll_lock": "Scroll_Lock",
        # Pause/Break
        "pause": "Pause",
        "break": "Break",
        # Numpad
        "numlock": "Num_Lock",
        "num_lock": "Num_Lock",
    }

    def _translate_key(self, key: str) -> str:
        """Translate a key name from pyautogui/Lux format to xdotool format."""
        key_lower = key.strip().lower()
        # Check if we have a mapping for this key
        if key_lower in self.XDOTOOL_KEY_MAP:
            return self.XDOTOOL_KEY_MAP[key_lower]
        # For single character keys, return as-is
        if len(key) == 1:
            return key
        # For unknown keys, capitalize first letter (xdotool convention)
        return key.capitalize()

    def _parse_hotkey(self, args_str: str) -> list[str]:
        """Parse hotkey string into list of keys and translate to xdotool format."""
        # Remove parentheses if present
        args_str = args_str.strip("()")
        # Split by '+' to get individual keys
        keys = [self._translate_key(key.strip()) for key in args_str.split("+")]
        # Format as Kernel expects: "Ctrl+t" style for combinations
        if len(keys) > 1:
            return ["+".join(keys)]
        return keys

    def _execute_click(self, x: int, y: int, num_clicks: int = 1, button: str = "left"):
        """Execute a click action."""
        self.session.kernel.browsers.computer.click_mouse(
            id=self.session.session_id,
            x=x,
            y=y,
            button=button,
            num_clicks=num_clicks,
        )

    def _execute_drag(self, x1: int, y1: int, x2: int, y2: int):
        """Execute a drag action."""
        self.session.kernel.browsers.computer.drag_mouse(
            id=self.session.session_id,
            path=[[x1, y1], [x2, y2]],
            button="left",
        )

    def _execute_type(self, text: str, press_enter: bool = False):
        """Execute a type action, optionally pressing Enter after."""
        self.session.kernel.browsers.computer.type_text(
            id=self.session.session_id,
            text=text,
            delay=self.type_delay,
        )
        # Press Enter if requested
        if press_enter:
            self.session.kernel.browsers.computer.press_key(
                id=self.session.session_id,
                keys=["Return"],
            )

    def _execute_hotkey(self, keys: list[str]):
        """Execute a hotkey action."""
        self.session.kernel.browsers.computer.press_key(
            id=self.session.session_id,
            keys=keys,
        )

    def _execute_scroll(self, x: int, y: int, direction: str):
        """Execute a scroll action."""
        # Move to position first
        self.session.kernel.browsers.computer.move_mouse(
            id=self.session.session_id,
            x=x,
            y=y,
        )
        # Scroll in the specified direction
        delta_y = self.scroll_amount if direction == "up" else -self.scroll_amount
        self.session.kernel.browsers.computer.scroll(
            id=self.session.session_id,
            x=x,
            y=y,
            delta_x=0,
            delta_y=delta_y,
        )

    def _execute_single_action(self, action: Action) -> None:
        """Execute a single action once."""
        arg = action.argument.strip("()")

        match action.type:
            case ActionType.CLICK:
                x, y = self._parse_coords(arg)
                self._execute_click(x, y)

            case ActionType.LEFT_DOUBLE:
                x, y = self._parse_coords(arg)
                self._execute_click(x, y, num_clicks=2)

            case ActionType.LEFT_TRIPLE:
                x, y = self._parse_coords(arg)
                self._execute_click(x, y, num_clicks=3)

            case ActionType.RIGHT_SINGLE:
                x, y = self._parse_coords(arg)
                self._execute_click(x, y, button="right")

            case ActionType.DRAG:
                x1, y1, x2, y2 = self._parse_drag_coords(arg)
                self._execute_drag(x1, y1, x2, y2)

            case ActionType.HOTKEY:
                keys = self._parse_hotkey(arg)
                self._execute_hotkey(keys)

            case ActionType.TYPE:
                # Remove quotes if present
                text = arg.strip("\"'")
                # Check if text ends with newline (indicates Enter should be pressed)
                press_enter = text.endswith("\n") or text.endswith("\\n")
                if press_enter:
                    # Remove trailing newline(s)
                    text = text.rstrip("\n").rstrip("\\n")
                self._execute_type(text, press_enter=press_enter)

            case ActionType.SCROLL:
                x, y, direction = self._parse_scroll(arg)
                self._execute_scroll(x, y, direction)

            case ActionType.FINISH:
                # Task completion - nothing to do
                print("Task marked as finished")

            case ActionType.WAIT:
                # Wait for specified duration
                time.sleep(self.wait_duration)

            case ActionType.CALL_USER:
                # Call user - implementation depends on requirements
                print("User intervention requested")

            case _:
                print(f"Unknown action type: {action.type}")

    def _execute_action(self, action: Action) -> None:
        """Execute an action, potentially multiple times."""
        count = action.count or 1

        for _ in range(count):
            self._execute_single_action(action)
            # Small pause between repeated actions
            if count > 1:
                time.sleep(self.action_pause)

    async def __call__(self, actions: list[Action]) -> None:
        """
        Execute a list of actions.

        Args:
            actions: List of Action objects to execute
        """
        if not self.session.session_id:
            raise RuntimeError("Browser session not initialized")

        for action in actions:
            try:
                # Run the synchronous action execution in a thread pool
                await asyncio.get_event_loop().run_in_executor(
                    None, self._execute_action, action
                )
                # Pause between actions
                await asyncio.sleep(self.action_pause)
            except Exception as e:
                print(f"Error executing action {action.type}: {e}")
                raise

    def reset(self):
        """Reset handler state. Called at automation start/end."""
        pass  # No state to reset for Kernel handler
