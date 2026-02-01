"""
System prompts for computer use agents.

Provides utilities for building system prompts with embedded tool definitions
in Qwen3-VL's native pre-training format.

Format overview:
- Tool definitions embedded in system prompt within XML tags
- Model outputs: "Action: <description>\\n<tool_call> {...} </tool_call>"
- Uses normalized 0-999 coordinate space (not pixels)

References:
    https://github.com/xlang-ai/OSWorld
    https://github.com/QwenLM/Qwen3-VL
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .actions import Action

# Default model configuration
MODEL_NAME = "Qwen/Qwen3-VL-30B-A3B-Instruct"
RENDERER_NAME = "qwen3_vl"

# Default LoRA configuration
DEFAULT_LORA_RANK = 8
DEFAULT_LEARNING_RATE = 1e-5

# Base prompt for computer use agents
CUA_BASE_PROMPT = """Do not click on or interact with cookie consent buttons ("Accept all", "Reject all", "Manage cookies", etc.) or similar system dialogs (e.g. "<site> wants to show notifications") unless they are actively blocking your ability to complete the task. Focus on the main task objective and go straight towards making progress towards it if possible.

If the task involves logging in or accessing an authenticated area, assume credentials will be provided when needed. Your job is to navigate to the appropriate login page and interact with login forms (clicking login buttons, focusing input fields, etc.) even if you don't have the actual credentials. Make progress towards the task rather than refusing because you lack credentials."""


def build_tool_definition(
    extra_actions: list[type["Action"]] | None = None,
    exclude_actions: set[str] | None = None,
) -> dict:
    """
    Build the tool definition dict for the system prompt.

    Args:
        extra_actions: Optional list of additional Action classes to include
        exclude_actions: Optional set of action_type strings to exclude

    Returns:
        Tool definition dict for embedding in system prompt
    """
    from .actions import STANDARD_ACTIONS, build_action_descriptions, collect_all_parameters

    action_descriptions = build_action_descriptions(extra_actions, exclude_actions)

    # Collect all action type strings, filtering out excluded ones
    exclude = exclude_actions or set()
    all_actions = [a for a in STANDARD_ACTIONS if a.action_type not in exclude]
    if extra_actions:
        all_actions.extend(a for a in extra_actions if a.action_type not in exclude)
    action_enums = [action.action_type for action in all_actions]

    # Collect all parameters from all action types
    all_params = collect_all_parameters(extra_actions, exclude_actions)

    # Build properties dict
    properties: dict = {
        "action": {
            "type": "string",
            "enum": action_enums,
            "description": action_descriptions,
        },
    }

    # Add all parameters from actions
    for param_name, param_schema in all_params.items():
        properties[param_name] = {k: v for k, v in param_schema.items() if k != "required"}

    return {
        "type": "function",
        "function": {
            "name_for_human": "computer_use",
            "name": "computer_use",
            "description": """Use a mouse and keyboard to interact with a computer screen.
* The screen uses normalized coordinates from 0 to 999 (not pixels).
* (0, 0) is the top-left corner, (999, 999) is the bottom-right corner.
* Make sure to click any buttons, links, icons, etc with the cursor in the center of the element.
* Whenever you intend to move the cursor to click on an element, you should consult the screenshot to determine the coordinates.
* If clicking fails, try adjusting the cursor position so the tip visually falls on the element.""",
            "parameters": {
                "type": "object",
                "required": ["action"],
                "properties": properties,
                "args_format": "Format the arguments as a JSON object.",
            },
        },
    }


def build_system_prompt(
    base_prompt: str | None = None,
    screen_width: int | None = None,
    screen_height: int | None = None,
    coordinate_type: str = "relative",
    extra_actions: list[type["Action"]] | None = None,
    exclude_actions: set[str] | None = None,
) -> str:
    """
    Build the system prompt with embedded tool definition (OSWorld style).

    This follows the native Qwen3-VL format for computer use agents.

    Args:
        base_prompt: Optional additional instructions to append.
        screen_width: Screen width in pixels (for absolute coordinates).
        screen_height: Screen height in pixels (for absolute coordinates).
        coordinate_type: "relative" (0-999) or "absolute" (pixels).
        extra_actions: Optional list of additional Action classes to include.
        exclude_actions: Optional set of action_type strings to exclude.

    Returns:
        Complete system prompt with tool definitions.
    """
    tool_definition = build_tool_definition(extra_actions, exclude_actions)
    tools_xml = f"""<tools>
{json.dumps(tool_definition, indent=2)}
</tools>"""

    if coordinate_type == "absolute" and screen_width and screen_height:
        resolution_note = f"* The screen's resolution is {screen_width}x{screen_height}."
    else:
        resolution_note = "* The screen's resolution is 1000x1000 (coordinates 0-999)."

    return f"""# Tools

You may call one or more functions to assist with the user query.

You are provided with function signatures within XML tags:
{tools_xml}

For each function call, return a JSON object with function name and arguments within XML tags:
<tool_call>
{{"name":<function_name>, "arguments":<args_dict>}}
</tool_call>

# Response format

Response format:
1) Action: a short imperative describing what to do in the UI.
2) A single <tool_call>...</tool_call> block containing only the JSON.

Rules:
- Output exactly in the order: Action, then <tool_call>.
- Be brief: one sentence for Action.
- Do not output anything else outside those parts.
{resolution_note}
- If coordinates are relative (0-999), map them to the visual element positions in the screenshot.

{base_prompt or ""}""".strip()


def get_system_prompt() -> str:
    """Get the default CUA system prompt with embedded tool definitions."""
    return build_system_prompt(CUA_BASE_PROMPT)
