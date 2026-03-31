"""
Computer Tool - Maps high-level actions to Kernel Computer Controls API.
"""

import asyncio
import base64

from kernel import Kernel

from .types import (
    ComputerAction,
    ComputerFunctionArgs,
    PREDEFINED_COMPUTER_USE_FUNCTIONS,
    DEFAULT_SCREEN_SIZE,
    COORDINATE_SCALE,
    ToolResult,
    ScreenSize,
)


TYPING_DELAY_MS = 12
SCREENSHOT_DELAY_SECS = 0.5


class ComputerTool:
    def __init__(
        self,
        kernel: Kernel,
        session_id: str,
        screen_size: ScreenSize = DEFAULT_SCREEN_SIZE,
    ):
        self.kernel = kernel
        self.session_id = session_id
        self.screen_size = screen_size

    def denormalize_x(self, x: int) -> int:
        return int((x / COORDINATE_SCALE) * self.screen_size.width)

    def denormalize_y(self, y: int) -> int:
        return int((y / COORDINATE_SCALE) * self.screen_size.height)

    async def screenshot(self) -> ToolResult:
        try:
            await asyncio.sleep(SCREENSHOT_DELAY_SECS)
            response = self.kernel.browsers.computer.capture_screenshot(self.session_id)
            screenshot_bytes = response.read()
            dimensions = _parse_png_dimensions(screenshot_bytes)
            if dimensions:
                self.screen_size = dimensions

            return ToolResult(
                base64_image=base64.b64encode(screenshot_bytes).decode("utf-8"),
                url="about:blank",
                width=dimensions.width if dimensions else None,
                height=dimensions.height if dimensions else None,
            )
        except Exception as e:
            return ToolResult(error=f"Failed to take screenshot: {e}", url="about:blank")

    async def execute_action(
        self, action_name: str, args: ComputerFunctionArgs
    ) -> ToolResult:
        if action_name not in [a.value for a in PREDEFINED_COMPUTER_USE_FUNCTIONS]:
            return ToolResult(error=f"Unknown action: {action_name}")

        try:
            if action_name == ComputerAction.OPEN_WEB_BROWSER:
                # Browser is already open in Kernel, just return screenshot
                pass

            elif action_name == ComputerAction.CLICK_AT:
                if "x" not in args or "y" not in args:
                    return ToolResult(error="click_at requires x and y coordinates")
                x = self.denormalize_x(args["x"])
                y = self.denormalize_y(args["y"])
                num_clicks = int(args.get("clicks", 1)) if args.get("clicks") else 1
                self.kernel.browsers.computer.click_mouse(
                    self.session_id,
                    x=x,
                    y=y,
                    button="left",
                    click_type="click",
                    num_clicks=num_clicks,
                )

            elif action_name == ComputerAction.HOVER_AT:
                if "x" not in args or "y" not in args:
                    return ToolResult(error="hover_at requires x and y coordinates")
                x = self.denormalize_x(args["x"])
                y = self.denormalize_y(args["y"])
                self.kernel.browsers.computer.move_mouse(
                    self.session_id, x=x, y=y
                )

            elif action_name == ComputerAction.TYPE_TEXT_AT:
                if "x" not in args or "y" not in args:
                    return ToolResult(error="type_text_at requires x and y coordinates")
                if "text" not in args:
                    return ToolResult(error="type_text_at requires text")

                x = self.denormalize_x(args["x"])
                y = self.denormalize_y(args["y"])

                self.kernel.browsers.computer.click_mouse(
                    self.session_id,
                    x=x,
                    y=y,
                    button="left",
                    click_type="click",
                    num_clicks=1,
                )

                if args.get("clear_before_typing", True):
                    self.kernel.browsers.computer.press_key(
                        self.session_id, keys=["ctrl+a"]
                    )
                    await asyncio.sleep(0.05)

                self.kernel.browsers.computer.type_text(
                    self.session_id,
                    text=args["text"],
                    delay=TYPING_DELAY_MS,
                )

                if args.get("press_enter", False):
                    await asyncio.sleep(0.1)
                    self.kernel.browsers.computer.press_key(
                        self.session_id, keys=["Return"]
                    )

            elif action_name == ComputerAction.SCROLL_DOCUMENT:
                if "direction" not in args:
                    return ToolResult(error="scroll_document requires direction")
                center_x = self.screen_size.width // 2
                center_y = self.screen_size.height // 2
                scroll_delta = 500

                delta_x, delta_y = 0, 0
                direction = args["direction"]
                if direction == "down":
                    delta_y = scroll_delta
                elif direction == "up":
                    delta_y = -scroll_delta
                elif direction == "right":
                    delta_x = scroll_delta
                elif direction == "left":
                    delta_x = -scroll_delta

                self.kernel.browsers.computer.scroll(
                    self.session_id,
                    x=center_x,
                    y=center_y,
                    delta_x=delta_x,
                    delta_y=delta_y,
                )

            elif action_name == ComputerAction.SCROLL_AT:
                if "x" not in args or "y" not in args:
                    return ToolResult(error="scroll_at requires x and y coordinates")
                if "direction" not in args:
                    return ToolResult(error="scroll_at requires direction")

                x = self.denormalize_x(args["x"])
                y = self.denormalize_y(args["y"])

                magnitude = args.get("magnitude", 800)
                direction = args["direction"]
                if direction in ("up", "down"):
                    magnitude = self.denormalize_y(magnitude)
                else:
                    magnitude = self.denormalize_x(magnitude)

                delta_x, delta_y = 0, 0
                if direction == "down":
                    delta_y = magnitude
                elif direction == "up":
                    delta_y = -magnitude
                elif direction == "right":
                    delta_x = magnitude
                elif direction == "left":
                    delta_x = -magnitude

                self.kernel.browsers.computer.scroll(
                    self.session_id,
                    x=x,
                    y=y,
                    delta_x=delta_x,
                    delta_y=delta_y,
                )

            elif action_name == ComputerAction.WAIT_5_SECONDS:
                await asyncio.sleep(5)

            elif action_name == ComputerAction.GO_BACK:
                self.kernel.browsers.computer.press_key(
                    self.session_id, keys=["alt+Left"]
                )
                await asyncio.sleep(1)

            elif action_name == ComputerAction.GO_FORWARD:
                self.kernel.browsers.computer.press_key(
                    self.session_id, keys=["alt+Right"]
                )
                await asyncio.sleep(1)

            elif action_name == ComputerAction.SEARCH:
                self.kernel.browsers.computer.press_key(
                    self.session_id, keys=["ctrl+l"]
                )

            elif action_name == ComputerAction.NAVIGATE:
                if "url" not in args:
                    return ToolResult(error="navigate requires url")
                self.kernel.browsers.computer.press_key(
                    self.session_id, keys=["ctrl+l"]
                )
                await asyncio.sleep(0.1)
                self.kernel.browsers.computer.type_text(
                    self.session_id,
                    text=args["url"],
                    delay=TYPING_DELAY_MS,
                )
                await asyncio.sleep(0.1)
                self.kernel.browsers.computer.press_key(
                    self.session_id, keys=["Return"]
                )
                await asyncio.sleep(1.5)

            elif action_name == ComputerAction.KEY_COMBINATION:
                if "keys" not in args:
                    return ToolResult(error="key_combination requires keys")
                keys = str(args["keys"])
                if keys.lower() == "enter":
                    keys = "Return"
                self.kernel.browsers.computer.press_key(
                    self.session_id, keys=[keys]
                )

            elif action_name == ComputerAction.DRAG_AND_DROP:
                required = ["x", "y", "destination_x", "destination_y"]
                if not all(k in args for k in required):
                    return ToolResult(
                        error="drag_and_drop requires x, y, destination_x, and destination_y"
                    )

                start_x = self.denormalize_x(args["x"])
                start_y = self.denormalize_y(args["y"])
                end_x = self.denormalize_x(args["destination_x"])
                end_y = self.denormalize_y(args["destination_y"])

                self.kernel.browsers.computer.drag_mouse(
                    self.session_id,
                    path=[[start_x, start_y], [end_x, end_y]],
                    button="left",
                )

            else:
                return ToolResult(error=f"Unhandled action: {action_name}")

            await asyncio.sleep(SCREENSHOT_DELAY_SECS)
            return await self.screenshot()

        except Exception as e:
            return ToolResult(error=f"Action failed: {e}", url="about:blank")


def _parse_png_dimensions(data: bytes) -> ScreenSize | None:
    if len(data) < 24:
        return None
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        return None
    width = int.from_bytes(data[16:20], "big")
    height = int.from_bytes(data[20:24], "big")
    if width <= 0 or height <= 0:
        return None
    return ScreenSize(width=width, height=height)
