"""
Configuration for Agent Auth (login discovery).

Provides system prompts and settings for login discovery agents.
"""

from __future__ import annotations

from core.prompts import build_system_prompt

from .actions import AGENT_AUTH_ACTIONS

# System prompt for login discovery agents
LOGIN_DISCOVERY_PROMPT = """Your task is to find the login page for a website and identify what input fields are required.

Instructions:
1. Navigate the website to find the login/sign-in page
2. Once you find a login form, identify all input fields (username, email, password, etc.)
3. Use the found_inputs action to report the required fields (this completes the task)

Do not click on or interact with cookie consent buttons unless they are blocking your task.
Do not attempt to fill in any credentials - just identify what fields exist."""


def get_agent_auth_system_prompt() -> str:
    """Get the system prompt for Agent Auth (login discovery)."""
    return build_system_prompt(
        base_prompt=LOGIN_DISCOVERY_PROMPT,
        extra_actions=AGENT_AUTH_ACTIONS,
        exclude_actions={"terminate"},  # Use found_inputs instead
    )


def make_agent_auth_task(domain: str) -> str:
    """
    Create an agent-auth task string for a domain.

    Args:
        domain: The domain name (e.g., "github.com")

    Returns:
        Standardized task instruction
    """
    return (
        f"Navigate to {domain} and find the login or registration page. "
        f"Identify the first input field(s) required to begin the login or registration process."
    )
