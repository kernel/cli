"""
Gemini Computer Use sampling loop.
Based on Google's computer-use-preview reference implementation.
"""

import os
from datetime import datetime
from typing import Any, Dict, List

from google import genai
from google.genai import types
from google.genai.types import (
    Content,
    FunctionResponse,
    GenerateContentConfig,
    Part,
)
from kernel import Kernel

from tools import ComputerTool, PREDEFINED_COMPUTER_USE_FUNCTIONS


# System prompt for browser-based computer use
SYSTEM_PROMPT = f"""You are a helpful assistant that can use a web browser.
You are operating a Chrome browser through computer use tools.
The browser is already open and ready for use.

When you need to navigate to a page, use the navigate action with a full URL.
When you need to interact with elements, use click_at, type_text_at, etc.
After each action, carefully evaluate the screenshot to determine your next step.

Current date: {datetime.now().strftime("%A, %B %d, %Y")}."""

# Maximum number of recent turns to keep screenshots for (to manage context)
MAX_RECENT_TURN_WITH_SCREENSHOTS = 3


async def sampling_loop(
    *,
    model: str,
    query: str,
    api_key: str,
    kernel: Kernel,
    session_id: str,
    max_iterations: int = 50,
    system_prompt_suffix: str = "",
) -> Dict[str, Any]:
    """
    Run the Gemini computer use sampling loop.

    Args:
        model: The Gemini model to use
        query: The user's query/task
        api_key: Google API key
        kernel: Kernel client instance
        session_id: Browser session ID
        max_iterations: Maximum number of loop iterations
        system_prompt_suffix: Additional system prompt text

    Returns:
        Dict with 'final_response' and 'iterations'
    """
    # Initialize the Gemini client
    client = genai.Client(
        api_key=api_key,
        vertexai=os.environ.get("USE_VERTEXAI", "0").lower() in ["true", "1"],
        project=os.environ.get("VERTEXAI_PROJECT"),
        location=os.environ.get("VERTEXAI_LOCATION"),
    )

    computer_tool = ComputerTool(kernel, session_id)

    # Initialize conversation with user query
    contents: List[Content] = [
        Content(
            role="user",
            parts=[Part(text=query)],
        )
    ]

    system_prompt = (
        f"{SYSTEM_PROMPT}\n\n{system_prompt_suffix}"
        if system_prompt_suffix
        else SYSTEM_PROMPT
    )

    # Generate content config
    generate_content_config = GenerateContentConfig(
        temperature=1,
        top_p=0.95,
        top_k=40,
        max_output_tokens=8192,
        system_instruction=system_prompt,
        tools=[
            types.Tool(
                computer_use=types.ComputerUse(
                    environment=types.Environment.ENVIRONMENT_BROWSER,
                ),
            ),
        ],
        thinking_config=types.ThinkingConfig(include_thoughts=True),
    )

    iteration = 0
    final_response = ""

    while iteration < max_iterations:
        iteration += 1
        print(f"\n=== Iteration {iteration} ===")

        try:
            # Generate response from Gemini
            response = client.models.generate_content(
                model=model,
                contents=contents,
                config=generate_content_config,
            )

            if not response.candidates:
                print("No candidates in response")
                break

            candidate = response.candidates[0]
            if not candidate.content:
                print("No content in candidate")
                break

            # Add assistant response to conversation
            contents.append(candidate.content)

            # Extract text and function calls
            reasoning = _extract_text(candidate.content)
            function_calls = _extract_function_calls(candidate.content)

            # Log the response
            print(f"Reasoning: {reasoning or '(none)'}")
            print(f"Function calls: {len(function_calls)}")
            for fc in function_calls:
                print(f"  - {fc.name}: {fc.args}")

            # Check finish reason
            finish_reason = candidate.finish_reason
            if (
                finish_reason == types.FinishReason.MALFORMED_FUNCTION_CALL
                and not function_calls
            ):
                print("Malformed function call, retrying...")
                continue

            # If no function calls, the model is done
            if not function_calls:
                print("Agent loop complete")
                final_response = reasoning or ""
                break

            # Execute function calls and collect results
            function_responses: List[Part] = []
            for fc in function_calls:
                args = dict(fc.args) if fc.args else {}

                # Handle safety decisions if present
                if (
                    "safety_decision" in args
                    and args["safety_decision"].get("decision") == "require_confirmation"
                ):
                    print(
                        f"Safety confirmation required: {args['safety_decision'].get('explanation')}"
                    )
                    print("Auto-acknowledging safety check")

                # Execute the action
                print(f"Executing action: {fc.name}")
                result = await computer_tool.execute_action(fc.name, args)

                if result.error:
                    print(f"Action error: {result.error}")
                    function_responses.append(
                        Part(
                            function_response=FunctionResponse(
                                name=fc.name,
                                response={"error": result.error},
                            )
                        )
                    )
                else:
                    # Build response with screenshot - always include URL (required by Computer Use API)
                    response_data: Dict[str, Any] = {
                        "url": result.url or "about:blank",
                    }

                    # Include screenshot for predefined functions
                    parts = None
                    if result.screenshot and _is_predefined_function(fc.name):
                        parts = [
                            types.FunctionResponsePart(
                                inline_data=types.FunctionResponseBlob(
                                    mime_type="image/png",
                                    data=result.screenshot,
                                )
                            )
                        ]

                    function_responses.append(
                        Part(
                            function_response=FunctionResponse(
                                name=fc.name,
                                response=response_data,
                                parts=parts,
                            )
                        )
                    )

            # Add function responses to conversation
            contents.append(
                Content(
                    role="user",
                    parts=function_responses,
                )
            )

            # Manage screenshot history to avoid context overflow
            _prune_old_screenshots(contents)

        except Exception as e:
            print(f"Error in sampling loop: {e}")
            break

    if iteration >= max_iterations:
        print("Max iterations reached")

    return {
        "final_response": final_response,
        "iterations": iteration,
    }


def _extract_text(content: Content) -> str:
    if not content.parts:
        return ""

    texts: List[str] = []
    for part in content.parts:
        if hasattr(part, "text") and part.text:
            texts.append(part.text)
    return " ".join(texts)


def _extract_function_calls(content: Content) -> List[types.FunctionCall]:
    if not content.parts:
        return []

    calls: List[types.FunctionCall] = []
    for part in content.parts:
        if hasattr(part, "function_call") and part.function_call:
            calls.append(part.function_call)
    return calls


def _is_predefined_function(name: str) -> bool:
    return name in [a.value for a in PREDEFINED_COMPUTER_USE_FUNCTIONS]


def _prune_old_screenshots(contents: List[Content]) -> None:
    turns_with_screenshots = 0

    # Iterate in reverse to find recent turns with screenshots
    for content in reversed(contents):
        if content.role != "user" or not content.parts:
            continue

        # Check if this turn has screenshots from predefined functions
        has_screenshot = False
        for part in content.parts:
            if (
                hasattr(part, "function_response")
                and part.function_response
                and _is_predefined_function(part.function_response.name or "")
            ):
                # Check if it has parts (which contain screenshots)
                if (
                    hasattr(part.function_response, "parts")
                    and part.function_response.parts
                ):
                    has_screenshot = True
                    break

        if has_screenshot:
            turns_with_screenshots += 1

            # Remove screenshots from old turns
            if turns_with_screenshots > MAX_RECENT_TURN_WITH_SCREENSHOTS:
                for part in content.parts:
                    if (
                        hasattr(part, "function_response")
                        and part.function_response
                        and _is_predefined_function(part.function_response.name or "")
                    ):
                        # Remove the parts array (which contains the screenshot)
                        part.function_response.parts = None
