import json
import time
from typing import Any, Callable
from computers.kernel_computer import (
    KernelComputer,
    _describe_action,
    _describe_batch_actions,
)
from utils import (
    create_response,
    show_image,
    pp,
    sanitize_message,
    check_blocklisted_url,
)

BATCH_FUNC_NAME = "batch_computer_actions"
EXTRA_FUNC_NAME = "computer_use_extra"

BATCH_INSTRUCTIONS = """You have three ways to perform actions:
1. The standard computer tool — use for single actions when you need screenshot feedback after each step.
2. batch_computer_actions — use to execute multiple actions at once when you can predict the outcome.
3. computer_use_extra — use high-level browser actions: goto, back, and url.

ALWAYS prefer batch_computer_actions when performing predictable sequences like:
- Clicking a text field, typing text, and pressing Enter
- Any sequence where you don't need to see intermediate results

Use computer_use_extra for:
- action="goto" only when changing the page URL
- action="back" to go back in history
- action="url" to read the exact current URL

When interacting with page content (search boxes, forms, chat inputs):
- Click the target input first, then type.
- Do not use URL-navigation actions for in-page text entry."""

BATCH_TOOL = {
    "type": "function",
    "name": BATCH_FUNC_NAME,
    "description": (
        "Execute multiple computer actions in sequence without waiting for "
        "screenshots between them. Use this when you can predict the outcome of a "
        "sequence of actions without needing intermediate visual feedback. After all "
        "actions execute, a single screenshot is taken and returned.\n\n"
        "PREFER this over individual computer actions when:\n"
        "- Typing text followed by pressing Enter\n"
        "- Clicking a field and then typing into it\n"
        "- Any sequence where intermediate screenshots aren't needed\n\n"
        "Constraint: return-value actions (url, screenshot) can appear at most once "
        "and only as the final action in the batch."
    ),
    "parameters": {
        "type": "object",
        "properties": {
            "actions": {
                "type": "array",
                "description": "Ordered list of actions to execute",
                "items": {
                    "type": "object",
                    "properties": {
                        "type": {
                            "type": "string",
                            "enum": [
                                "click",
                                "double_click",
                                "type",
                                "keypress",
                                "scroll",
                                "move",
                                "drag",
                                "wait",
                                "goto",
                                "back",
                                "url",
                                "screenshot",
                            ],
                        },
                        "x": {"type": "number"},
                        "y": {"type": "number"},
                        "text": {"type": "string"},
                        "url": {"type": "string"},
                        "keys": {"type": "array", "items": {"type": "string"}},
                        "hold_keys": {"type": "array", "items": {"type": "string"}},
                        "button": {"type": "string"},
                        "scroll_x": {"type": "number"},
                        "scroll_y": {"type": "number"},
                    },
                    "required": ["type"],
                },
            },
        },
        "required": ["actions"],
    },
    "strict": False,
}

EXTRA_TOOL = {
    "type": "function",
    "name": EXTRA_FUNC_NAME,
    "description": "High-level browser actions for navigation and URL retrieval.",
    "parameters": {
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["goto", "back", "url"],
                "description": "Action to perform: goto, back, or url.",
            },
            "url": {
                "type": "string",
                "description": "Required when action is goto. Fully qualified URL to navigate to.",
            },
        },
        "required": ["action"],
    },
    "strict": False,
}

# Keep this shape aligned with CUA and current OpenAI Responses API.
OPENAI_COMPUTER_TOOL = {"type": "computer"}


class Agent:
    """An agent that uses OpenAI CUA with Kernel's native computer control API."""

    def __init__(
        self,
        model="gpt-5.4",
        computer: KernelComputer = None,
        tools: list[dict] = [],
        acknowledge_safety_check_callback: Callable = lambda message: False,
    ):
        self.model = model
        self.computer = computer
        self.tools = list(tools)
        self.print_steps = True
        self.debug = False
        self.show_images = False
        self.on_event: Callable[[dict], None] | None = None
        self._model_request_started_at: float | None = None
        self.acknowledge_safety_check_callback = acknowledge_safety_check_callback

        if computer:
            self.tools += [
                dict(OPENAI_COMPUTER_TOOL),
                BATCH_TOOL,
                EXTRA_TOOL,
            ]

    def debug_print(self, *args):
        if self.debug:
            pp(*args)

    def _emit_event(self, event: str, data: dict | None = None) -> None:
        if self.print_steps and self.on_event:
            self.on_event({"event": event, "data": data or {}})

    def _current_model_elapsed_ms(self) -> int | None:
        if self._model_request_started_at is None:
            return None
        return int((time.time() - self._model_request_started_at) * 1000)

    def _extract_reasoning_text(self, item: dict[str, Any]) -> str:
        summary = item.get("summary")
        if not isinstance(summary, list):
            return ""
        pieces: list[str] = []
        for part in summary:
            if not isinstance(part, dict):
                continue
            text = part.get("text")
            if isinstance(text, str) and text:
                pieces.append(text)
        return " ".join(pieces).strip()

    def _extract_prompt_text(self, item: dict[str, Any]) -> str | None:
        if item.get("role") != "user":
            return None
        content = item.get("content")
        if isinstance(content, str):
            return content
        if not isinstance(content, list):
            return None
        parts: list[str] = []
        for entry in content:
            if not isinstance(entry, dict):
                continue
            text = entry.get("text")
            if isinstance(text, str) and text:
                parts.append(text)
        return " ".join(parts) if parts else None

    def _batch_terminal_read_action(self, actions: list[dict[str, Any]]) -> str:
        if not actions:
            return ""
        action_type = str(actions[-1].get("type", ""))
        if action_type in ("url", "screenshot"):
            return action_type
        return ""

    def handle_item(self, item):
        """Handle each item; may cause a computer action + screenshot."""
        if item["type"] == "reasoning":
            text = self._extract_reasoning_text(item)
            if text:
                self._emit_event("reasoning_delta", {"text": text})

        if item["type"] == "message":
            if item.get("role") == "assistant":
                content = item.get("content", [])
                if isinstance(content, list):
                    for part in content:
                        if isinstance(part, dict) and isinstance(part.get("text"), str):
                            self._emit_event("text_delta", {"text": part["text"]})
                self._emit_event("text_done", {})

        if item["type"] == "function_call":
            name, args = item["name"], json.loads(item["arguments"])
            elapsed_ms = self._current_model_elapsed_ms()
            if name == BATCH_FUNC_NAME:
                actions = args.get("actions", [])
                if isinstance(actions, list):
                    typed_actions = [a for a in actions if isinstance(a, dict)]
                    payload = {
                        "action_type": "batch",
                        "description": _describe_batch_actions(typed_actions),
                        "action": {"type": "batch", "actions": typed_actions},
                    }
                    if elapsed_ms is not None:
                        payload["elapsed_ms"] = elapsed_ms
                    self._emit_event(
                        "action",
                        payload,
                    )
            else:
                payload = {
                    "action_type": name,
                    "description": f"{name}({json.dumps(args)})",
                    "action": args,
                }
                if elapsed_ms is not None:
                    payload["elapsed_ms"] = elapsed_ms
                self._emit_event(
                    "action",
                    payload,
                )

            if name == BATCH_FUNC_NAME:
                return self._handle_batch_call(item["call_id"], args)
            if name == EXTRA_FUNC_NAME:
                return self._handle_extra_call(item["call_id"], args)

            return [
                {
                    "type": "function_call_output",
                    "call_id": item["call_id"],
                    "output": f"Unsupported function call: {name}",
                }
            ]

        if item["type"] == "computer_call":
            elapsed_ms = self._current_model_elapsed_ms()
            actions = item.get("actions")
            if not isinstance(actions, list):
                single = item.get("action")
                actions = [single] if isinstance(single, dict) else []
            typed_actions = [a for a in actions if isinstance(a, dict)]

            if len(typed_actions) == 1:
                action_type = str(typed_actions[0].get("type", "unknown"))
                action_payload: dict[str, Any] = typed_actions[0]
                description = _describe_action(
                    action_type,
                    {k: v for k, v in typed_actions[0].items() if k != "type"},
                )
            else:
                action_type = "batch"
                action_payload = {"type": "batch", "actions": typed_actions}
                description = _describe_batch_actions(typed_actions)

            payload = {
                "action_type": action_type,
                "description": description,
                "action": action_payload,
            }
            if elapsed_ms is not None:
                payload["elapsed_ms"] = elapsed_ms
            self._emit_event("action", payload)
            self.computer.batch_actions(typed_actions)

            screenshot_base64 = self.computer.screenshot()
            self._emit_event(
                "screenshot",
                {"captured": True, "bytes_base64": len(screenshot_base64)},
            )
            if self.show_images:
                show_image(screenshot_base64)

            pending_checks = item.get("pending_safety_checks", [])
            for check in pending_checks:
                message = check["message"]
                if not self.acknowledge_safety_check_callback(message):
                    raise ValueError(
                        f"Safety check failed: {message}. Cannot continue with unacknowledged safety checks."
                    )

            call_output = {
                "type": "computer_call_output",
                "call_id": item["call_id"],
                "acknowledged_safety_checks": pending_checks,
                "output": {
                    "type": "computer_screenshot",
                    "image_url": f"data:image/png;base64,{screenshot_base64}",
                },
            }

            if self.computer.get_environment() == "browser":
                current_url = self.computer.get_current_url()
                check_blocklisted_url(current_url)
                call_output["output"]["current_url"] = current_url

            return [call_output]
        return []

    def _handle_batch_call(self, call_id, args):
        actions = args.get("actions", [])
        if not isinstance(actions, list):
            actions = []
        self.computer.batch_actions(actions)
        status_text = "Actions executed successfully."
        terminal_action = self._batch_terminal_read_action(actions if isinstance(actions, list) else [])
        if terminal_action == "url":
            try:
                current_url = self.computer.get_current_url()
                status_text = f"Actions executed successfully. Current URL: {current_url}"
            except Exception as exc:
                status_text = f"Actions executed, but url() failed: {exc}"
        output_items: list[dict[str, Any]] = [{"type": "text", "text": status_text}]
        if terminal_action != "url":
            screenshot_base64 = self.computer.screenshot()
            output_items.append(
                {
                    "type": "image_url",
                    "image_url": f"data:image/png;base64,{screenshot_base64}",
                    "detail": "original",
                }
            )
        return [
            {
                "type": "function_call_output",
                "call_id": call_id,
                "output": json.dumps(output_items),
            }
        ]

    def _handle_extra_call(self, call_id, args):
        action = args.get("action", "")
        url = args.get("url", "")
        if action == "goto":
            self.computer.batch_actions([{"type": "goto", "url": url}])
            status_text = "goto executed successfully."
        elif action == "back":
            self.computer.batch_actions([{"type": "back"}])
            status_text = "back executed successfully."
        elif action == "url":
            status_text = f"Current URL: {self.computer.get_current_url()}"
        else:
            status_text = f"unknown {EXTRA_FUNC_NAME} action: {action}"

        output_items: list[dict[str, Any]] = [{"type": "text", "text": status_text}]
        if action != "url":
            screenshot_base64 = self.computer.screenshot()
            output_items.append(
                {
                    "type": "image_url",
                    "image_url": f"data:image/png;base64,{screenshot_base64}",
                    "detail": "original",
                }
            )
        return [
            {
                "type": "function_call_output",
                "call_id": call_id,
                "output": json.dumps(output_items),
            }
        ]

    def run_full_turn(
        self,
        input_items,
        print_steps=True,
        debug=False,
        show_images=False,
        on_event: Callable[[dict], None] | None = None,
    ):
        self.print_steps = print_steps
        self.debug = debug
        self.show_images = show_images
        self.on_event = on_event
        new_items = []
        turns = 0

        for message in input_items:
            if isinstance(message, dict):
                prompt = self._extract_prompt_text(message)
                if prompt:
                    self._emit_event("prompt", {"text": prompt})

        try:
            while new_items[-1].get("role") != "assistant" if new_items else True:
                turns += 1
                self.debug_print([sanitize_message(msg) for msg in input_items + new_items])

                self._model_request_started_at = time.time()
                response = create_response(
                    model=self.model,
                    input=input_items + new_items,
                    tools=self.tools,
                    truncation="auto",
                    reasoning={
                        "effort": "low",
                        "summary": "concise",
                    },
                    instructions=BATCH_INSTRUCTIONS,
                )
                self.debug_print(response)

                if "output" not in response:
                    if self.debug:
                        print(response)
                    raise ValueError("No output from model")

                new_items += response["output"]
                for item in response["output"]:
                    new_items += self.handle_item(item)
                self._model_request_started_at = None
                self._emit_event("turn_done", {"turn": turns})
        except Exception as exc:
            self._model_request_started_at = None
            self._emit_event("error", {"message": str(exc)})
            raise

        self._emit_event("run_complete", {"turns": turns})
        return new_items
