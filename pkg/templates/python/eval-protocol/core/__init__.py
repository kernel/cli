"""
Core package for VLM browser agent evaluation.

Provides:
- QwenAgent: VLM-based computer use agent
- KernelBrowserAdapter: Browser control via Kernel API
- Action types for computer use tasks
- WebJudge reward model for trajectory evaluation
"""

from .actions import (
    Action,
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
    STANDARD_ACTIONS,
    get_action_registry,
    parse_action_from_response,
    parse_action_from_args,
)
from .agent import QwenAgent, AgentConfig, AgentState
from .agent_loop import run_agent_loop, AgentLoopResult, StepResult
from .browser import KernelBrowserAdapter, acquired_browser
from .prompts import build_system_prompt, get_system_prompt
from .utils import resize_image, encode_image, compute_image_similarity

__all__ = [
    # Actions
    "Action",
    "LeftClickAction",
    "RightClickAction",
    "DoubleClickAction",
    "TripleClickAction",
    "MiddleClickAction",
    "MouseMoveAction",
    "LeftClickDragAction",
    "TypeTextAction",
    "KeyAction",
    "ScrollAction",
    "WaitAction",
    "TerminateAction",
    "STANDARD_ACTIONS",
    "get_action_registry",
    "parse_action_from_response",
    "parse_action_from_args",
    # Agent
    "QwenAgent",
    "AgentConfig",
    "AgentState",
    # Agent loop
    "run_agent_loop",
    "AgentLoopResult",
    "StepResult",
    # Browser
    "KernelBrowserAdapter",
    "acquired_browser",
    # Prompts
    "build_system_prompt",
    "get_system_prompt",
    # Utils
    "resize_image",
    "encode_image",
    "compute_image_similarity",
]
