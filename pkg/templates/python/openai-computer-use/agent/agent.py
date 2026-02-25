import json
from typing import Callable
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
        self.acknowledge_safety_check_callback = acknowledge_safety_check_callback

        if computer:
            dimensions = computer.get_dimensions()
            self.tools += [
                {
                    "type": "computer-preview",
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
        if item["type"] == "message":
            if self.print_steps:
                print(item["content"][0]["text"])

        if item["type"] == "function_call":
            name, args = item["name"], json.loads(item["arguments"])
            if self.print_steps:
                print(f"{name}({args})")

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
            if self.print_steps:
                print(f"{action_type}({action_args})")

            self._execute_computer_action(action_type, action_args)

            screenshot_base64 = self.computer.screenshot()
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
        self, input_items, print_steps=True, debug=False, show_images=False
    ):
        self.print_steps = print_steps
        self.debug = debug
        self.show_images = show_images
        new_items = []

        while new_items[-1].get("role") != "assistant" if new_items else True:
            self.debug_print([sanitize_message(msg) for msg in input_items + new_items])

            response = create_response(
                model=self.model,
                input=input_items + new_items,
                tools=self.tools,
                truncation="auto",
                instructions=BATCH_INSTRUCTIONS,
            )
            self.debug_print(response)

            if "output" not in response and self.debug:
                print(response)
                raise ValueError("No output from model")
            else:
                new_items += response["output"]
                for item in response["output"]:
                    new_items += self.handle_item(item)

        return new_items
