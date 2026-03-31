"""
Type definitions for computer use actions.
"""

from dataclasses import dataclass
from enum import StrEnum
from typing import Literal, Optional, TypedDict


class ComputerAction(StrEnum):
    OPEN_WEB_BROWSER = "open_web_browser"
    CLICK_AT = "click_at"
    HOVER_AT = "hover_at"
    TYPE_TEXT_AT = "type_text_at"
    SCROLL_DOCUMENT = "scroll_document"
    SCROLL_AT = "scroll_at"
    WAIT_5_SECONDS = "wait_5_seconds"
    GO_BACK = "go_back"
    GO_FORWARD = "go_forward"
    SEARCH = "search"
    NAVIGATE = "navigate"
    KEY_COMBINATION = "key_combination"
    DRAG_AND_DROP = "drag_and_drop"


# Derive from enum to prevent drift when adding new actions
PREDEFINED_COMPUTER_USE_FUNCTIONS = list(ComputerAction)


ScrollDirection = Literal["up", "down", "left", "right"]


class SafetyDecision(TypedDict, total=False):
    decision: str
    explanation: str


class ComputerFunctionArgs(TypedDict, total=False):
    # click_at, hover_at, scroll_at
    x: int
    y: int
    clicks: int

    # type_text_at
    text: str
    press_enter: bool
    clear_before_typing: bool

    # scroll_document, scroll_at
    direction: ScrollDirection
    magnitude: int

    # navigate
    url: str

    # key_combination
    keys: str

    # drag_and_drop
    destination_x: int
    destination_y: int

    # Safety decision (may be included in any function call)
    safety_decision: SafetyDecision


@dataclass
class ToolResult:
    base64_image: Optional[str] = None
    url: Optional[str] = None
    error: Optional[str] = None
    width: Optional[int] = None
    height: Optional[int] = None


@dataclass
class ScreenSize:
    width: int
    height: int


DEFAULT_SCREEN_SIZE = ScreenSize(width=1200, height=800)

# Normalized coordinates scale (0-1000)
COORDINATE_SCALE = 1000
