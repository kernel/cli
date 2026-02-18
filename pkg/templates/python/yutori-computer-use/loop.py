"""
Yutori n1 Sampling Loop

Implements the agent loop for Yutori's n1-latest computer use model.
n1-latest uses an OpenAI-compatible API with tool_calls:
- Actions are returned via tool_calls in the assistant message
- Tool results use role: "tool" with matching tool_call_id
- The model stops by returning content without tool_calls
- Coordinates are returned in 1000x1000 space and need scaling

@see https://docs.yutori.com/reference/n1
"""

import json
from typing import Any, Optional

from kernel import Kernel
from openai import OpenAI

from tools import ComputerTool, N1Action, ToolResult


async def sampling_loop(
    *,
    model: str = "n1-latest",
    task: str,
    api_key: str,
    kernel: Kernel,
    session_id: str,
    max_completion_tokens: int = 4096,
    max_iterations: int = 50,
    viewport_width: int = 1280,
    viewport_height: int = 800,
) -> dict[str, Any]:
    """Run the n1 sampling loop until the model stops calling tools or max iterations."""
    client = OpenAI(
        api_key=api_key,
        base_url="https://api.yutori.com/v1",
    )

    computer_tool = ComputerTool(kernel, session_id, viewport_width, viewport_height)

    initial_screenshot = await computer_tool.screenshot()

    user_content: list[dict[str, Any]] = [{"type": "text", "text": task}]
    if initial_screenshot.get("base64_image"):
        user_content.append({
            "type": "image_url",
            "image_url": {
                "url": f"data:image/webp;base64,{initial_screenshot['base64_image']}"
            },
        })

    conversation_messages: list[dict[str, Any]] = [
        {"role": "user", "content": user_content}
    ]

    iteration = 0
    final_answer: Optional[str] = None

    while iteration < max_iterations:
        iteration += 1
        print(f"\n=== Iteration {iteration} ===")

        try:
            response = client.chat.completions.create(
                model=model,
                messages=conversation_messages,
                max_completion_tokens=max_completion_tokens,
                temperature=0.3,
            )
        except Exception as api_error:
            print(f"API call failed: {api_error}")
            raise api_error

        if not response.choices or len(response.choices) == 0:
            print(f"No choices in response: {response}")
            raise ValueError("No choices in API response")

        choice = response.choices[0]
        assistant_message = choice.message
        if not assistant_message:
            raise ValueError("No response from model")

        print("Assistant content:", assistant_message.content or "(none)")

        # Preserve full assistant message (including tool_calls) in history
        assistant_dict: dict[str, Any] = {
            "role": "assistant",
            "content": assistant_message.content or "",
        }
        if assistant_message.tool_calls:
            assistant_dict["tool_calls"] = [
                {
                    "id": tc.id,
                    "type": tc.type,
                    "function": {
                        "name": tc.function.name,
                        "arguments": tc.function.arguments,
                    },
                }
                for tc in assistant_message.tool_calls
            ]
        conversation_messages.append(assistant_dict)

        tool_calls = assistant_message.tool_calls

        # No tool_calls means the model is done
        if not tool_calls:
            final_answer = assistant_message.content or None
            print(f"No tool_calls, model is done. Final answer: {final_answer}")
            break

        for tc in tool_calls:
            action_name = tc.function.name
            try:
                args = json.loads(tc.function.arguments)
            except json.JSONDecodeError:
                print(f"Failed to parse tool_call arguments: {tc.function.arguments}")
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": "Error: failed to parse arguments",
                })
                continue

            action: N1Action = {"action_type": action_name, **args}
            print(f"Executing action: {action_name}", args)

            scaled_action = _scale_coordinates(action, viewport_width, viewport_height)

            result: ToolResult
            try:
                result = await computer_tool.execute(scaled_action)
            except Exception as e:
                print(f"Action failed: {e}")
                result = {"error": str(e)}

            # Build tool response message
            if result.get("base64_image"):
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": [
                        {
                            "type": "image_url",
                            "image_url": {
                                "url": f"data:image/webp;base64,{result['base64_image']}"
                            },
                        }
                    ],
                })
            elif result.get("error"):
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": f"Action failed: {result['error']}",
                })
            else:
                conversation_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": result.get("output", "OK"),
                })

    if iteration >= max_iterations:
        print("Max iterations reached")

    return {
        "messages": conversation_messages,
        "final_answer": final_answer,
    }


def _scale_coordinates(action: N1Action, viewport_width: int, viewport_height: int) -> N1Action:
    scaled = dict(action)

    if "coordinates" in scaled and scaled["coordinates"]:
        coords = scaled["coordinates"]
        scaled["coordinates"] = [
            round((coords[0] / 1000) * viewport_width),
            round((coords[1] / 1000) * viewport_height),
        ]

    if "start_coordinates" in scaled and scaled["start_coordinates"]:
        coords = scaled["start_coordinates"]
        scaled["start_coordinates"] = [
            round((coords[0] / 1000) * viewport_width),
            round((coords[1] / 1000) * viewport_height),
        ]

    return scaled
