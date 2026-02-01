"""
Qwen3-VL Computer Use Agent.

A VLM-based agent for computer use tasks, inspired by OSWorld's agent architecture.
Uses OpenRouter for model access and Qwen3-VL's native tool call format.

References:
    https://github.com/xlang-ai/OSWorld
    https://arxiv.org/abs/2404.07972 (OSWorld paper)
"""

from __future__ import annotations

import base64
import io
import json
import os
from dataclasses import dataclass, field
from typing import cast

from openai import OpenAI
from openai.types.chat import ChatCompletionMessageParam, ChatCompletionMessageToolCall
from PIL import Image

from .actions import Action, parse_action_from_args, parse_action_from_response
from .prompts import get_system_prompt
from .utils import resize_image

# Available Qwen VLM models on OpenRouter
AVAILABLE_MODELS: list[str] = [
    "qwen/qwen3-vl-8b-instruct",
    "qwen/qwen3-vl-30b-a3b-instruct",
    "qwen/qwen3-vl-235b-a22b-instruct",
]

DEFAULT_MODEL: str = AVAILABLE_MODELS[0]

# Tinker OpenAI-compatible inference endpoint
TINKER_OPENAI_BASE_URL: str = "https://tinker.thinkingmachines.dev/services/tinker-prod/oai/api/v1"


def is_tinker_checkpoint(model: str) -> bool:
    """Check if the model path is a Tinker checkpoint (sampler_path)."""
    return model.startswith("tinker://")


def encode_image(image: Image.Image) -> str:
    """Convert a PIL image to base64 JPEG string."""
    if image.mode == "RGBA":
        image = image.convert("RGB")
    buffered = io.BytesIO()
    image.save(buffered, format="JPEG", quality=85)
    return base64.b64encode(buffered.getvalue()).decode("utf-8")


@dataclass
class AgentConfig:
    """Configuration for the QwenAgent."""

    model: str = DEFAULT_MODEL
    api_key: str | None = None
    base_url: str = "https://openrouter.ai/api/v1"
    history_n: int = 4  # Number of history steps to include in context
    max_tokens: int = 2048
    temperature: float = 0.0
    system_prompt: str | None = None  # Custom system prompt (uses default if None)
    extra_actions: list[type[Action]] = field(default_factory=list)


@dataclass
class AgentState:
    """Mutable state for the agent during a session."""

    screenshots: list[str] = field(default_factory=list)  # base64 encoded
    actions: list[str] = field(default_factory=list)  # action descriptions
    responses: list[str] = field(default_factory=list)  # raw LLM responses
    messages: list[dict] = field(default_factory=list)  # full message history for eval
    step_count: int = 0


class QwenAgent:
    """
    Qwen3-VL based computer use agent.

    Implements an observation-action loop for web navigation tasks.
    Uses normalized coordinates (0-999) for coordinate-independent actions.

    Usage:
        agent = QwenAgent(AgentConfig(model="qwen/qwen3-vl-8b-instruct"))

        while True:
            action = agent.predict(task, screenshot)
            if action is None or action.is_terminal:
                break
            execute_action(action)
            screenshot = capture_screenshot()
    """

    def __init__(self, config: AgentConfig | None = None):
        """
        Initialize the agent.

        Args:
            config: Agent configuration. Uses defaults if not provided.
        """
        self.config = config or AgentConfig()
        self._system_prompt = self.config.system_prompt or get_system_prompt()
        self._extra_actions = self.config.extra_actions or None

        # Initialize OpenAI client
        if is_tinker_checkpoint(self.config.model):
            base_url = TINKER_OPENAI_BASE_URL
            api_key = self.config.api_key or os.getenv("TINKER_API_KEY")
        else:
            base_url = self.config.base_url
            api_key = self.config.api_key or os.getenv("OPENROUTER_API_KEY")

        self.client = OpenAI(
            api_key=api_key or "dummy",
            base_url=base_url,
        )

        # Agent state
        self.state = AgentState()

    @property
    def system_prompt(self) -> str:
        """Get the current system prompt."""
        return self._system_prompt

    def reset(self) -> None:
        """Reset agent state for a new task."""
        self.state = AgentState()

    def record_prior_action(self, action_description: str) -> None:
        """
        Record an action that was taken externally (e.g., initial navigation).

        This allows the agent to be aware of actions taken before the predict loop,
        such as navigating to the starting URL.

        Args:
            action_description: Description of the action (e.g., "navigate(https://...)")
        """
        self.state.actions.append(action_description)

    def predict(self, instruction: str, screenshot: Image.Image) -> Action | None:
        """
        Generate the next action given a task instruction and screenshot.

        Screenshots are captured BEFORE each action is taken, following
        the Online-Mind2Web convention where screenshot[i] shows the state
        before action[i] was executed.

        Args:
            instruction: The task instruction (e.g., "Find the login page")
            screenshot: Current screenshot of the browser (state before action)

        Returns:
            The predicted Action, or None if parsing fails
        """
        # Process and store screenshot
        processed_screenshot = resize_image(screenshot, max_size=1024)
        screenshot_b64 = encode_image(processed_screenshot)
        self.state.screenshots.append(screenshot_b64)

        # Build messages with history
        messages = self._build_messages(instruction, screenshot_b64)

        # Store system message on first call
        if self.state.step_count == 0:
            self.state.messages.append({
                "role": "system",
                "content": self.system_prompt,
            })

        # Store user message (the last message in the built messages list)
        user_msg = messages[-1]  # The user message with screenshot + instruction
        self.state.messages.append(dict(user_msg))

        # Call LLM
        response = self.client.chat.completions.create(
            model=self.config.model,
            messages=messages,
            max_tokens=self.config.max_tokens,
            temperature=self.config.temperature,
        )

        msg = response.choices[0].message
        response_text = msg.content or ""
        self.state.responses.append(response_text)

        # Parse response to Action
        action = parse_action_from_response(response_text, self._extra_actions)

        # If no action found in text, check for native OpenAI-style tool_calls
        tool_calls_data = None
        if action is None and msg.tool_calls:
            tool_calls_data = []
            for tc in msg.tool_calls:
                if not isinstance(tc, ChatCompletionMessageToolCall):
                    continue
                tool_calls_data.append({
                    "id": tc.id,
                    "type": "function",
                    "function": {
                        "name": tc.function.name,
                        "arguments": tc.function.arguments,
                    }
                })
                if tc.function.name == "computer_use":
                    try:
                        args = json.loads(tc.function.arguments)
                        action = parse_action_from_args(args, self._extra_actions)
                        if action:
                            break
                    except json.JSONDecodeError:
                        pass

        # Store assistant response (with tool_calls if present)
        assistant_msg: dict = {"role": "assistant", "content": response_text}
        if tool_calls_data:
            assistant_msg["tool_calls"] = tool_calls_data
        self.state.messages.append(assistant_msg)

        if action:
            self.state.actions.append(action.to_description())

        self.state.step_count += 1

        return action

    def _build_messages(self, instruction: str, current_screenshot_b64: str) -> list[ChatCompletionMessageParam]:
        """
        Build the message list for the LLM, including history.

        Args:
            instruction: The task instruction
            current_screenshot_b64: Base64 encoded current screenshot

        Returns:
            List of message dicts for the OpenAI API
        """
        messages: list[dict] = [
            {
                "role": "system",
                "content": [{"type": "text", "text": self.system_prompt}],
            }
        ]

        # Build previous actions summary (keep only most recent history_n actions)
        num_actions = len(self.state.actions)
        history_start = max(0, num_actions - self.config.history_n)
        previous_actions = []
        for i in range(history_start, num_actions):
            previous_actions.append(f"Step {i + 1}: {self.state.actions[i]}")
        previous_actions_str = "\n".join(previous_actions) if previous_actions else "None"

        # Build instruction prompt
        instruction_prompt = f"""Please generate the next action according to the screenshot, instruction, and previous actions.

Instruction: {instruction}

Previous actions:
{previous_actions_str}"""

        # Single user message with both image and text
        messages.append(
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {
                            "url": f"data:image/jpeg;base64,{current_screenshot_b64}"
                        },
                    },
                    {
                        "type": "text",
                        "text": instruction_prompt,
                    },
                ],
            }
        )

        return cast(list[ChatCompletionMessageParam], messages)

    def get_action_history(self) -> list[str]:
        """Get the list of action descriptions taken so far."""
        return list(self.state.actions)

    def get_last_response(self) -> str | None:
        """Get the most recent LLM response."""
        if self.state.responses:
            return self.state.responses[-1]
        return None

    def get_messages(self) -> list[dict]:
        """Get the full message history including tool calls."""
        return list(self.state.messages)
