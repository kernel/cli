import json
import time
from typing import Any, Callable
from computers.kernel_computer import KernelComputer
from utils import (
    create_response,
    show_image,
    pp,
    sanitize_message,
    check_blocklisted_url,
)

BATCH_FUNC_NAME = "batch_computer_actions"

BATCH_INSTRUCTIONS = """You have two ways to perform actions:
1. The standard computer tool — use for single actions when you need screenshot feedback after each step.
2. batch_computer_actions — use to execute multiple actions at once when you can predict the outcome.

ALWAYS prefer batch_computer_actions when performing predictable sequences like:
- Clicking a text field, typing text, and pressing Enter
- Typing a URL and pressing Enter
- Any sequence where you don't need to see intermediate results"""

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
        "- Any sequence where intermediate screenshots are not needed"
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
                            "enum": ["click", "double_click", "type", "keypress", "scroll", "move", "drag", "wait"],
                        },
                        "x": {"type": "number"},
                        "y": {"type": "number"},
                        "text": {"type": "string"},
                        "keys": {"type": "array", "items": {"type": "string"}},
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


class Agent:
    """An agent that uses OpenAI CUA with Kernel's native computer control API."""

    def __init__(
        self,
        model="computer-use-preview",
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
            dimensions = computer.get_dimensions()
            self.tools += [
                {
                    "type": "computer_use_preview",
                    "display_width": dimensions[0],
                    "display_height": dimensions[1],
                    "environment": computer.get_environment(),
                },
                BATCH_TOOL,
                {
                    "type": "function",
                    "name": "back",
                    "description": "Go back to the previous page.",
                    "parameters": {},
                },
                {
                    "type": "function",
                    "name": "goto",
                    "description": "Go to a specific URL.",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "url": {
                                "type": "string",
                                "description": "Fully qualified URL to navigate to.",
                            },
                        },
                        "additionalProperties": False,
                        "required": ["url"],
                    },
                },
                {
                    "type": "function",
                    "name": "forward",
                    "description": "Go forward to the next page.",
                    "parameters": {},
                },
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

    def _describe_action(self, action_type: str, action_args: dict[str, Any]) -> str:
        if action_type == "click":
            x = int(action_args.get("x", 0))
            y = int(action_args.get("y", 0))
            button = action_args.get("button", "left")
            if button in ("", "left"):
                return f"click({x}, {y})"
            return f"click({x}, {y}, {button})"
        if action_type == "double_click":
            return f"double_click({int(action_args.get('x', 0))}, {int(action_args.get('y', 0))})"
        if action_type == "type":
            text = str(action_args.get("text", ""))
            if len(text) > 60:
                text = f"{text[:57]}..."
            return f"type({text!r})"
        if action_type == "keypress":
            keys = action_args.get("keys", [])
            return f"keypress({keys})"
        if action_type == "scroll":
            return (
                f"scroll({int(action_args.get('x', 0))}, {int(action_args.get('y', 0))}, "
                f"dx={int(action_args.get('scroll_x', 0))}, dy={int(action_args.get('scroll_y', 0))})"
            )
        if action_type == "move":
            return f"move({int(action_args.get('x', 0))}, {int(action_args.get('y', 0))})"
        if action_type == "drag":
            return "drag(...)"
        if action_type == "wait":
            return f"wait({int(action_args.get('ms', 1000))}ms)"
        if action_type == "screenshot":
            return "screenshot()"
        return action_type

    def _describe_batch_actions(self, actions: list[dict[str, Any]]) -> str:
        pieces: list[str] = []
        for action in actions:
            action_type = str(action.get("type", "unknown"))
            action_args = {k: v for k, v in action.items() if k != "type"}
            pieces.append(self._describe_action(action_type, action_args))
        return "batch[" + " -> ".join(pieces) + "]"

    def _execute_computer_action(self, action_type, action_args):
        if action_type == "click":
            self.computer.click(**action_args)
        elif action_type == "double_click":
            self.computer.double_click(**action_args)
        elif action_type == "type":
            self.computer.type(**action_args)
        elif action_type == "keypress":
            self.computer.keypress(**action_args)
        elif action_type == "scroll":
            self.computer.scroll(**action_args)
        elif action_type == "move":
            self.computer.move(**action_args)
        elif action_type == "drag":
            self.computer.drag(**action_args)
        elif action_type == "wait":
            self.computer.wait(**action_args)
        elif action_type == "screenshot":
            pass
        else:
            print(f"Warning: unknown action type: {action_type}")

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
                        "description": self._describe_batch_actions(typed_actions),
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

            if hasattr(self.computer, name):
                method = getattr(self.computer, name)
                method(**args)
            return [
                {
                    "type": "function_call_output",
                    "call_id": item["call_id"],
                    "output": "success",
                }
            ]

        if item["type"] == "computer_call":
            action = item["action"]
            action_type = action["type"]
            action_args = {k: v for k, v in action.items() if k != "type"}
            elapsed_ms = self._current_model_elapsed_ms()
            payload = {
                "action_type": action_type,
                "description": self._describe_action(action_type, action_args),
                "action": action,
            }
            if elapsed_ms is not None:
                payload["elapsed_ms"] = elapsed_ms
            self._emit_event(
                "action",
                payload,
            )

            self._execute_computer_action(action_type, action_args)

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
                    "type": "input_image",
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
        self.computer.batch_actions(actions)
        screenshot_base64 = self.computer.screenshot()
        return [
            {
                "type": "function_call_output",
                "call_id": call_id,
                "output": json.dumps([
                    {"type": "text", "text": "Actions executed successfully."},
                    {"type": "image_url", "image_url": f"data:image/png;base64,{screenshot_base64}"},
                ]),
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
