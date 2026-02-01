"""
Agent Auth benchmark for login discovery tasks.
"""

from .actions import FoundInputsAction, FoundField, AGENT_AUTH_ACTIONS
from .config import get_agent_auth_system_prompt, make_agent_auth_task

__all__ = [
    "FoundInputsAction",
    "FoundField",
    "AGENT_AUTH_ACTIONS",
    "get_agent_auth_system_prompt",
    "make_agent_auth_task",
]
