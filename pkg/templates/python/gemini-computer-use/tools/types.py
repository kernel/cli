"""
Type definitions for Gemini Computer Use actions.
Based on Google's computer-use-preview reference implementation.
"""

from dataclasses import dataclass
from enum import StrEnum
from typing import Literal, Optional, TypedDict


class GeminiAction(StrEnum):
    """Gemini predefined computer use actions."""
    
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


# All predefined Gemini computer use function names
PREDEFINED_COMPUTER_USE_FUNCTIONS = [
    GeminiAction.OPEN_WEB_BROWSER,
    GeminiAction.CLICK_AT,
    GeminiAction.HOVER_AT,
    GeminiAction.TYPE_TEXT_AT,
    GeminiAction.SCROLL_DOCUMENT,
    GeminiAction.SCROLL_AT,
    GeminiAction.WAIT_5_SECONDS,
    GeminiAction.GO_BACK,
    GeminiAction.GO_FORWARD,
    GeminiAction.SEARCH,
    GeminiAction.NAVIGATE,
    GeminiAction.KEY_COMBINATION,
    GeminiAction.DRAG_AND_DROP,
]


# Scroll direction type
ScrollDirection = Literal["up", "down", "left", "right"]


class SafetyDecision(TypedDict, total=False):
    """Safety decision from Gemini."""
    
    decision: str
    explanation: str


class GeminiFunctionArgs(TypedDict, total=False):
    """Arguments for Gemini function calls."""
    
    # click_at, hover_at, scroll_at
    x: int
    y: int
    
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
    """Result from executing a computer action."""
    
    # Base64-encoded screenshot image
    base64_image: Optional[str] = None
    # Screenshot as raw bytes
    screenshot: Optional[bytes] = None
    # Current URL of the browser
    url: Optional[str] = None
    # Error message if the action failed
    error: Optional[str] = None


@dataclass
class EnvState:
    """Environment state returned from computer actions."""
    
    # Current URL of the browser
    url: str
    # Screenshot as bytes
    screenshot: bytes


@dataclass
class ScreenSize:
    """Screen dimensions for coordinate denormalization."""
    
    width: int
    height: int


# Default screen size (matching Kernel browser viewport)
DEFAULT_SCREEN_SIZE = ScreenSize(width=1024, height=768)

# Gemini uses normalized coordinates (0-1000)
COORDINATE_SCALE = 1000
