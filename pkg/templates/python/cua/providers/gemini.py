"""
Gemini CUA provider.

Uses Google's Gemini computer use API with native browser environment.
Coordinates are normalized (0-1000 scale) by the Gemini API.
"""

import os
from datetime import datetime
from typing import Any

from google import genai
from google.genai import types
from google.genai.types import (
    Content,
    FunctionResponse,
    GenerateContentConfig,
    Part,
)

from tools import CommonAction, KernelExecutor
from providers import ProviderConfig, ProviderResult

COORDINATE_SCALE = 1000
PX_PER_NOTCH = 60
MAX_NOTCHES = 17
MAX_RECENT_SCREENSHOTS = 3

PREDEFINED_FUNCTIONS = {
    "open_web_browser", "click_at", "hover_at", "type_text_at",
    "scroll_document", "scroll_at", "wait_5_seconds", "go_back", "go_forward",
    "search", "navigate", "key_combination", "drag_and_drop",
}


def _get_system_prompt() -> str:
    current_date = datetime.now().strftime("%A, %B %d, %Y")
    return f"""You are a helpful assistant that can use a web browser.
You are operating a Chrome browser through computer use tools.
The browser is already open and ready for use.

When you need to navigate to a page, use the navigate action with a full URL.
When you need to interact with elements, use click_at, type_text_at, etc.
After each action, carefully evaluate the screenshot to determine your next step.

Current date: {current_date}."""


def _to_common_action(
    name: str, args: dict[str, Any], vw: int, vh: int,
) -> CommonAction:
    def dx(x: float) -> int:
        return round((x / COORDINATE_SCALE) * vw)
    def dy(y: float) -> int:
        return round((y / COORDINATE_SCALE) * vh)

    if name == "open_web_browser":
        return CommonAction(type="screenshot")
    if name == "click_at":
        return CommonAction(type="click", x=dx(args.get("x", 0)), y=dy(args.get("y", 0)))
    if name == "hover_at":
        return CommonAction(type="mouse_move", x=dx(args.get("x", 0)), y=dy(args.get("y", 0)))
    if name in ("scroll_document", "scroll_at"):
        direction = args.get("direction", "down")
        mag = args.get("magnitude", 400)
        notches = min(MAX_NOTCHES, max(1, round(mag / PX_PER_NOTCH)))
        sx, sy = 0, 0
        if direction == "down": sy = notches
        elif direction == "up": sy = -notches
        elif direction == "right": sx = notches
        elif direction == "left": sx = -notches
        x = dx(args["x"]) if "x" in args else vw // 2
        y = dy(args["y"]) if "y" in args else vh // 2
        return CommonAction(type="scroll", x=x, y=y, scroll_x=sx, scroll_y=sy)
    if name == "wait_5_seconds":
        return CommonAction(type="wait", duration=5000)
    if name == "go_back":
        return CommonAction(type="back")
    if name == "go_forward":
        return CommonAction(type="key", keys="alt+Right")
    if name == "search":
        return CommonAction(type="key", keys="ctrl+l")
    if name == "navigate":
        return CommonAction(type="goto", url=args.get("url", ""))
    if name == "key_combination":
        return CommonAction(type="key", keys=args.get("keys", ""))
    if name == "drag_and_drop":
        return CommonAction(
            type="drag",
            start_x=dx(args.get("x", 0)), start_y=dy(args.get("y", 0)),
            end_x=dx(args.get("destination_x", 0)), end_y=dy(args.get("destination_y", 0)),
        )
    # type_text_at is handled specially in the loop
    return CommonAction(type="screenshot")


def _prune_old_screenshots(contents: list[Content]) -> None:
    count = 0
    for content in reversed(contents):
        if content.role != "user" or not content.parts:
            continue
        has_screenshot = any(
            hasattr(p, "function_response") and p.function_response
            and (p.function_response.name or "") in PREDEFINED_FUNCTIONS
            and hasattr(p.function_response, "parts") and p.function_response.parts
            for p in content.parts
        )
        if has_screenshot:
            count += 1
            if count > MAX_RECENT_SCREENSHOTS:
                for p in content.parts:
                    if (hasattr(p, "function_response") and p.function_response
                            and (p.function_response.name or "") in PREDEFINED_FUNCTIONS):
                        p.function_response.parts = None


class GeminiProvider:
    name = "gemini"

    async def run(self, config: ProviderConfig, executor: KernelExecutor) -> ProviderResult:
        api_key = config.api_key or os.getenv("GOOGLE_API_KEY")
        if not api_key:
            raise ValueError("GOOGLE_API_KEY is required for Gemini provider")

        model = config.model or "gemini-2.5-computer-use-preview-10-2025"
        client = genai.Client(api_key=api_key)
        vw = config.viewport_width
        vh = config.viewport_height

        contents: list[Content] = [
            Content(role="user", parts=[Part(text=config.query)])
        ]

        generate_config = GenerateContentConfig(
            temperature=1, top_p=0.95, top_k=40, max_output_tokens=8192,
            system_instruction=_get_system_prompt(),
            tools=[types.Tool(computer_use=types.ComputerUse(environment=types.Environment.ENVIRONMENT_BROWSER))],
            thinking_config=types.ThinkingConfig(include_thoughts=True),
        )

        max_iterations = 50
        for i in range(max_iterations):
            print(f"[gemini] iteration {i + 1}")

            response = client.models.generate_content(
                model=model, contents=contents, config=generate_config,
            )

            if not response.candidates or not response.candidates[0].content:
                break

            candidate = response.candidates[0]
            contents.append(candidate.content)

            # Extract function calls
            function_calls = [
                p.function_call for p in (candidate.content.parts or [])
                if hasattr(p, "function_call") and p.function_call
            ]

            if not function_calls:
                texts = [p.text for p in (candidate.content.parts or []) if hasattr(p, "text") and p.text]
                return ProviderResult(result=" ".join(texts) or "Task completed.", provider="gemini")

            # Execute function calls
            function_responses: list[Part] = []
            for fc in function_calls:
                if not fc.name:
                    continue
                args = dict(fc.args) if fc.args else {}
                print(f"[gemini] action: {fc.name}")

                try:
                    if fc.name == "type_text_at":
                        x = round((args.get("x", 0) / COORDINATE_SCALE) * vw)
                        y = round((args.get("y", 0) / COORDINATE_SCALE) * vh)
                        await executor.execute(CommonAction(type="click", x=x, y=y))
                        if args.get("clear_before_typing") is not False:
                            await executor.execute(CommonAction(type="key", keys="ctrl+a"))
                        await executor.execute(CommonAction(type="type", text=args.get("text", "")))
                        if args.get("press_enter"):
                            await executor.execute(CommonAction(type="key", keys="Return"))
                        result = await executor.screenshot()
                    else:
                        common = _to_common_action(fc.name, args, vw, vh)
                        result = await executor.execute(common)

                    resp_data: dict[str, Any] = {"url": "about:blank"}
                    parts = None
                    if result.base64_image and fc.name in PREDEFINED_FUNCTIONS:
                        parts = [types.FunctionResponsePart(
                            inline_data=types.FunctionResponseBlob(
                                mime_type="image/png", data=result.base64_image,
                            )
                        )]
                    function_responses.append(Part(
                        function_response=FunctionResponse(
                            name=fc.name, response=resp_data, parts=parts,
                        )
                    ))
                except Exception as e:
                    function_responses.append(Part(
                        function_response=FunctionResponse(
                            name=fc.name, response={"error": str(e), "url": "about:blank"},
                        )
                    ))

            contents.append(Content(role="user", parts=function_responses))
            _prune_old_screenshots(contents)

        return ProviderResult(result="Max iterations reached.", provider="gemini")
