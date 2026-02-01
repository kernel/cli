"""
Action types for computer use agents.

Defines structured action types compatible with OSWorld's action space.
Actions can be parsed from LLM tool calls and executed via browser adapters.

Reference: https://github.com/xlang-ai/OSWorld
"""

from __future__ import annotations

import json
import re
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any, ClassVar

if TYPE_CHECKING:
    from PIL import Image


class Action(ABC):
    """
    Abstract base class for computer use actions.

    Subclasses must define:
    - action_type: The string identifier for the action
    - description: Human-readable description for documentation
    - parameters: Dict of parameter schemas for tool definition
    - parse_args: Class method to parse from tool call arguments
    - to_description: Instance method to create a human-readable description
    - to_tool_args: Instance method to format as tool call arguments

    Optional:
    - is_terminal: If True, this action stops the agent loop (default: False)
    - skip_screen_settle: If True, skip waiting for screen to settle after action
    - model_description: The model's reasoning text (set during parsing)
    """

    action_type: ClassVar[str]
    description: ClassVar[str]
    parameters: ClassVar[dict[str, dict[str, Any]]] = {}
    is_terminal: ClassVar[bool] = False
    skip_screen_settle: ClassVar[bool] = False

    # Instance attribute: model's reasoning from "Action:" line
    model_description: str | None = None

    @classmethod
    @abstractmethod
    def parse_args(cls, args: dict) -> Action | None:
        """Parse an Action instance from tool call arguments."""
        ...

    @abstractmethod
    def to_description(self) -> str:
        """Convert this action to a human-readable description."""
        ...

    @abstractmethod
    def to_tool_args(self) -> dict:
        """Convert this action to tool call arguments dict."""
        ...

    def to_tool_call(self) -> str:
        """Format this action as a string."""
        payload = {"name": "computer_use", "arguments": self.to_tool_args()}
        return f"<tool_call>\n{json.dumps(payload)}\n</tool_call>"

    def to_response(self, action_description: str) -> str:
        """Format a complete assistant response with this action."""
        description = action_description.rstrip(".")
        return f"Action: {description}.\n{self.to_tool_call()}"

    def overlay_on_image(self, image: Image.Image) -> Image.Image | None:
        """
        Overlay this action on a screenshot for visualization.

        Override in subclasses that support visualization (e.g., click actions).

        Returns:
            Annotated PIL Image, or None if this action type doesn't support it
        """
        return None


# =============================================================================
# Standard OSWorld Actions
# =============================================================================


@dataclass
class LeftClickAction(Action):
    """Click the left mouse button at normalized coordinates."""

    x: int  # 0-999 normalized
    y: int  # 0-999 normalized

    action_type: ClassVar[str] = "left_click"
    description: ClassVar[str] = "Click the left mouse button at a coordinate."
    parameters: ClassVar[dict] = {
        "coordinate": {
            "type": "array",
            "description": "The [x, y] coordinates (0-999 normalized).",
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> LeftClickAction | None:
        coord = args.get("coordinate", [0, 0])
        return cls(x=int(coord[0]), y=int(coord[1]))

    def to_description(self) -> str:
        return f"Left click at ({self.x}, {self.y})"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "coordinate": [self.x, self.y]}

    def overlay_on_image(self, image: Image.Image) -> Image.Image:
        from .utils import add_click_overlay, normalized_to_pixel

        px, py = normalized_to_pixel(self.x, self.y, image.width, image.height)
        return add_click_overlay(image, px, py)


@dataclass
class RightClickAction(Action):
    """Click the right mouse button at normalized coordinates."""

    x: int
    y: int

    action_type: ClassVar[str] = "right_click"
    description: ClassVar[str] = "Click the right mouse button at a coordinate."
    parameters: ClassVar[dict] = {
        "coordinate": {
            "type": "array",
            "description": "The [x, y] coordinates (0-999 normalized).",
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> RightClickAction | None:
        coord = args.get("coordinate", [0, 0])
        return cls(x=int(coord[0]), y=int(coord[1]))

    def to_description(self) -> str:
        return f"Right click at ({self.x}, {self.y})"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "coordinate": [self.x, self.y]}

    def overlay_on_image(self, image: Image.Image) -> Image.Image:
        from .utils import add_click_overlay, normalized_to_pixel

        px, py = normalized_to_pixel(self.x, self.y, image.width, image.height)
        return add_click_overlay(image, px, py)


@dataclass
class DoubleClickAction(Action):
    """Double-click the left mouse button at normalized coordinates."""

    x: int
    y: int

    action_type: ClassVar[str] = "double_click"
    description: ClassVar[str] = "Double-click the left mouse button at a coordinate."
    parameters: ClassVar[dict] = {
        "coordinate": {
            "type": "array",
            "description": "The [x, y] coordinates (0-999 normalized).",
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> DoubleClickAction | None:
        coord = args.get("coordinate", [0, 0])
        return cls(x=int(coord[0]), y=int(coord[1]))

    def to_description(self) -> str:
        return f"Double click at ({self.x}, {self.y})"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "coordinate": [self.x, self.y]}

    def overlay_on_image(self, image: Image.Image) -> Image.Image:
        from .utils import add_click_overlay, normalized_to_pixel

        px, py = normalized_to_pixel(self.x, self.y, image.width, image.height)
        return add_click_overlay(image, px, py)


@dataclass
class TripleClickAction(Action):
    """Triple-click the left mouse button at normalized coordinates."""

    x: int
    y: int

    action_type: ClassVar[str] = "triple_click"
    description: ClassVar[str] = "Triple-click the left mouse button at a coordinate."
    parameters: ClassVar[dict] = {
        "coordinate": {
            "type": "array",
            "description": "The [x, y] coordinates (0-999 normalized).",
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> TripleClickAction | None:
        coord = args.get("coordinate", [0, 0])
        return cls(x=int(coord[0]), y=int(coord[1]))

    def to_description(self) -> str:
        return f"Triple click at ({self.x}, {self.y})"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "coordinate": [self.x, self.y]}

    def overlay_on_image(self, image: Image.Image) -> Image.Image:
        from .utils import add_click_overlay, normalized_to_pixel

        px, py = normalized_to_pixel(self.x, self.y, image.width, image.height)
        return add_click_overlay(image, px, py)


@dataclass
class MiddleClickAction(Action):
    """Click the middle mouse button at normalized coordinates."""

    x: int
    y: int

    action_type: ClassVar[str] = "middle_click"
    description: ClassVar[str] = "Click the middle mouse button at a coordinate."
    parameters: ClassVar[dict] = {
        "coordinate": {
            "type": "array",
            "description": "The [x, y] coordinates (0-999 normalized).",
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> MiddleClickAction | None:
        coord = args.get("coordinate", [0, 0])
        return cls(x=int(coord[0]), y=int(coord[1]))

    def to_description(self) -> str:
        return f"Middle click at ({self.x}, {self.y})"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "coordinate": [self.x, self.y]}

    def overlay_on_image(self, image: Image.Image) -> Image.Image:
        from .utils import add_click_overlay, normalized_to_pixel

        px, py = normalized_to_pixel(self.x, self.y, image.width, image.height)
        return add_click_overlay(image, px, py)


@dataclass
class MouseMoveAction(Action):
    """Move the mouse to normalized coordinates."""

    x: int
    y: int

    action_type: ClassVar[str] = "mouse_move"
    description: ClassVar[str] = "Move the cursor to a coordinate."
    parameters: ClassVar[dict] = {
        "coordinate": {
            "type": "array",
            "description": "The [x, y] coordinates (0-999 normalized).",
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> MouseMoveAction | None:
        coord = args.get("coordinate", [0, 0])
        return cls(x=int(coord[0]), y=int(coord[1]))

    def to_description(self) -> str:
        return f"Move mouse to ({self.x}, {self.y})"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "coordinate": [self.x, self.y]}


@dataclass
class LeftClickDragAction(Action):
    """Click and drag to normalized coordinates."""

    start_x: int
    start_y: int
    end_x: int
    end_y: int

    action_type: ClassVar[str] = "left_click_drag"
    description: ClassVar[str] = "Click and drag the cursor from start to end coordinate."
    parameters: ClassVar[dict] = {
        "coordinate": {
            "type": "array",
            "description": "The end [x, y] coordinates (0-999 normalized).",
            "required": True,
        },
        "start_coordinate": {
            "type": "array",
            "description": "The start [x, y] coordinates. Uses current position if not provided.",
            "required": False,
        },
    }

    @classmethod
    def parse_args(cls, args: dict) -> LeftClickDragAction | None:
        coord = args.get("coordinate", [0, 0])
        start_coord = args.get("start_coordinate", coord)
        return cls(
            start_x=int(start_coord[0]),
            start_y=int(start_coord[1]),
            end_x=int(coord[0]),
            end_y=int(coord[1]),
        )

    def to_description(self) -> str:
        return f"Drag from ({self.start_x}, {self.start_y}) to ({self.end_x}, {self.end_y})"

    def to_tool_args(self) -> dict:
        return {
            "action": self.action_type,
            "coordinate": [self.end_x, self.end_y],
            "start_coordinate": [self.start_x, self.start_y],
        }


@dataclass
class TypeTextAction(Action):
    """Type text into the focused input field."""

    text: str

    action_type: ClassVar[str] = "type"
    description: ClassVar[str] = "Type text into the focused input field."
    parameters: ClassVar[dict] = {
        "text": {
            "type": "string",
            "description": "The text to type.",
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> TypeTextAction | None:
        return cls(text=args.get("text", ""))

    def to_description(self) -> str:
        text_preview = self.text[:30] + "..." if len(self.text) > 30 else self.text
        return f"Type '{text_preview}'"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "text": self.text}


@dataclass
class KeyAction(Action):
    """Press keyboard keys (single key or combination)."""

    keys: list[str]  # e.g. ["enter"], ["ctrl", "c"]

    action_type: ClassVar[str] = "key"
    description: ClassVar[str] = 'Press keyboard keys (e.g., ["enter"], ["ctrl", "c"]).'
    parameters: ClassVar[dict] = {
        "keys": {
            "type": "array",
            "description": 'The keys to press. Example: ["enter"] or ["ctrl", "c"].',
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> KeyAction | None:
        keys = args.get("keys", [])
        if isinstance(keys, str):
            keys = [keys]
        return cls(keys=keys)

    def to_description(self) -> str:
        return f"Press keys: {'+'.join(self.keys)}"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "keys": self.keys}


@dataclass
class ScrollAction(Action):
    """Scroll the page at a position."""

    x: int  # Position to scroll at (normalized)
    y: int
    delta_y: int  # Positive = down, negative = up
    delta_x: int = 0  # Horizontal scroll (optional)

    action_type: ClassVar[str] = "scroll"
    description: ClassVar[str] = "Scroll the page at a position."
    parameters: ClassVar[dict] = {
        "coordinate": {
            "type": "array",
            "description": "The [x, y] position to scroll at (0-999 normalized).",
            "required": False,
        },
        "direction": {
            "type": "string",
            "enum": ["up", "down"],
            "description": "Scroll direction.",
            "required": False,
        },
        "pixels": {
            "type": "number",
            "description": "Scroll amount in pixels.",
            "required": False,
        },
        "delta_y": {
            "type": "number",
            "description": "Vertical scroll amount. Positive = down, negative = up.",
            "required": False,
        },
    }

    @classmethod
    def parse_args(cls, args: dict) -> ScrollAction | None:
        coord = args.get("coordinate", [500, 500])
        direction = args.get("direction", "down")
        pixels = args.get("pixels", 120)
        delta_y = pixels if direction == "down" else -pixels
        if "delta_y" in args:
            delta_y = args["delta_y"]
        return cls(
            x=int(coord[0]),
            y=int(coord[1]),
            delta_y=int(delta_y),
            delta_x=int(args.get("delta_x", 0)),
        )

    def to_description(self) -> str:
        direction = "down" if self.delta_y > 0 else "up"
        return f"Scroll {direction} at ({self.x}, {self.y})"

    def to_tool_args(self) -> dict:
        return {
            "action": self.action_type,
            "coordinate": [self.x, self.y],
            "delta_y": self.delta_y,
            "delta_x": self.delta_x,
        }


@dataclass
class WaitAction(Action):
    """Wait for a specified duration."""

    seconds: float = 1.0

    action_type: ClassVar[str] = "wait"
    description: ClassVar[str] = "Wait for the page to load."
    skip_screen_settle: ClassVar[bool] = True
    parameters: ClassVar[dict] = {
        "time": {
            "type": "number",
            "description": "Seconds to wait. Default is 1.",
            "required": False,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> WaitAction | None:
        return cls(seconds=float(args.get("time", args.get("seconds", 1.0))))

    def to_description(self) -> str:
        return f"Wait {self.seconds}s"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "time": self.seconds}


@dataclass
class TerminateAction(Action):
    """Terminate the task with a status."""

    status: str  # "success" or "failure"

    action_type: ClassVar[str] = "terminate"
    description: ClassVar[str] = "Terminate the task and report completion status."
    is_terminal: ClassVar[bool] = True
    skip_screen_settle: ClassVar[bool] = True
    parameters: ClassVar[dict] = {
        "status": {
            "type": "string",
            "enum": ["success", "failure"],
            "description": "Task completion status.",
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> TerminateAction | None:
        return cls(status=args.get("status", "failure"))

    def to_description(self) -> str:
        return f"Terminate with status: {self.status}"

    def to_tool_args(self) -> dict:
        return {"action": self.action_type, "status": self.status}


# =============================================================================
# Action Registry and Parsing
# =============================================================================

STANDARD_ACTIONS: list[type[Action]] = [
    LeftClickAction,
    RightClickAction,
    DoubleClickAction,
    TripleClickAction,
    MiddleClickAction,
    MouseMoveAction,
    LeftClickDragAction,
    TypeTextAction,
    KeyAction,
    ScrollAction,
    WaitAction,
    TerminateAction,
]


def get_action_registry(
    extra_actions: list[type[Action]] | None = None,
    exclude_actions: set[str] | None = None,
) -> dict[str, type[Action]]:
    """
    Build a registry mapping action_type strings to Action classes.

    Args:
        extra_actions: Additional Action classes to include
        exclude_actions: Set of action_type strings to exclude

    Returns:
        Dict mapping action_type to Action class
    """
    exclude = exclude_actions or set()
    registry = {
        action.action_type: action
        for action in STANDARD_ACTIONS
        if action.action_type not in exclude
    }
    if extra_actions:
        for action in extra_actions:
            if action.action_type not in exclude:
                registry[action.action_type] = action
    return registry


def build_action_descriptions(
    extra_actions: list[type[Action]] | None = None,
    exclude_actions: set[str] | None = None,
) -> str:
    """Build a formatted string of action descriptions for use in prompts."""
    exclude = exclude_actions or set()
    actions = [a for a in STANDARD_ACTIONS if a.action_type not in exclude]
    if extra_actions:
        actions.extend(a for a in extra_actions if a.action_type not in exclude)

    lines = ["The action to perform:"]
    for action_cls in actions:
        lines.append(f"* {action_cls.action_type}: {action_cls.description}")
    return "\n".join(lines)


def collect_all_parameters(
    extra_actions: list[type[Action]] | None = None,
    exclude_actions: set[str] | None = None,
) -> dict[str, dict[str, Any]]:
    """Collect all unique parameters from all action types."""
    exclude = exclude_actions or set()
    actions = [a for a in STANDARD_ACTIONS if a.action_type not in exclude]
    if extra_actions:
        actions.extend(a for a in extra_actions if a.action_type not in exclude)

    all_params: dict[str, dict[str, Any]] = {}
    for action_cls in actions:
        for param_name, param_schema in action_cls.parameters.items():
            if param_name not in all_params:
                all_params[param_name] = param_schema.copy()
                all_params[param_name].pop("required", None)
    return all_params


def parse_tool_call(response: str) -> dict | None:
    """
    Extract the tool call JSON from an LLM response.

    Expects format:
    Action: <description>
    <tool_call>
    {"name": "computer_use", "arguments": {...}}
    </tool_call>

    Returns the parsed JSON dict or None if not found.
    """
    # Try with closing tag first, then without (30B model sometimes omits closing tag)
    match = re.search(r"<tool_call>\s*(.*?)\s*</tool_call>", response, re.DOTALL)
    if not match:
        # Fallback: match <tool_call> followed by JSON object without closing tag
        match = re.search(r"<tool_call>\s*(\{.*\})\s*$", response, re.DOTALL)
    if not match:
        return None

    try:
        return json.loads(match.group(1))
    except json.JSONDecodeError:
        return None


def parse_action_from_response(
    response: str,
    extra_actions: list[type[Action]] | None = None,
) -> Action | None:
    """
    Parse an Action from an LLM response containing a tool call.

    Also extracts the model's reasoning from the "Action:" line.

    Args:
        response: Raw LLM response text
        extra_actions: Additional Action classes to recognize

    Returns:
        Parsed Action or None if parsing fails
    """
    tool_call = parse_tool_call(response)
    if not tool_call:
        return None

    if tool_call.get("name") != "computer_use":
        return None

    args = tool_call.get("arguments", {})
    action = parse_action_from_args(args, extra_actions)

    # Extract model's reasoning from "Action:" line
    if action is not None:
        action_match = re.match(r"Action:\s*(.+?)(?:\n|$)", response, re.IGNORECASE)
        if action_match:
            action.model_description = action_match.group(1).strip().rstrip(".")

    return action


def parse_action_from_args(
    args: dict,
    extra_actions: list[type[Action]] | None = None,
) -> Action | None:
    """
    Parse an Action from tool call arguments.

    Args:
        args: The "arguments" dict from a computer_use tool call
        extra_actions: Additional Action classes to recognize

    Returns:
        Parsed Action or None if parsing fails
    """
    action_type = args.get("action")
    if not action_type:
        return None

    registry = get_action_registry(extra_actions)
    action_cls = registry.get(action_type)
    if action_cls:
        return action_cls.parse_args(args)

    return None
