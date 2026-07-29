"""
Groq-driven Moondream + Kernel agent loop.
"""

from __future__ import annotations

import asyncio
import json
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Tuple

from groq import Groq
from json_repair import repair_json

from moondream import MoondreamClient
from tools import (
    COORDINATE_SCALE,
    ComputerAction,
    ComputerTool,
    DEFAULT_SCREEN_SIZE,
    ScreenSize,
)


MODEL_NAME = "openai/gpt-oss-120b"


SYSTEM_PROMPT = """You are a browser-automation controller. You do NOT see images.
You must decide actions and call Moondream for any visual understanding.
Return ONLY a single JSON object that matches the schema below.
Parsing note: the client will extract the substring between the first '{' and last '}' and run jsonrepair on it.
Therefore, do NOT include any extra text before or after the JSON object.

Browser context:
- The browser is already open. Do NOT request an open_browser action.

Action policy:
- Bundle multiple actions when you can (e.g., navigate -> moondream_query).
- Use moondream_* actions for all visual understanding; keep queries short and specific.
- Never emit moondream_query without a clear question.
- Use click_at/type_text_at/scroll_at with coordinates in 0-1000 normalized scale.
- If you need coordinates, call moondream_point first.
- Prefer type_text_at with press_enter=true to submit searches; use key_combination mainly for shortcuts.
- You may include post_wait_ms in args to wait after an action (agent handles it).
- If the task requires a URL or page identity, call page_info after the relevant navigation/click.
- If your actions did not change state, reassess with a new Moondream question rather than repeating.
- If you need a specific item URL/details, open a specific item page (not a results list) and confirm it.
- If a click does not change the page, try a different target or use hover_at to reveal link text/URL.
- When opening an item, prefer clicking the title or image; verify you reached a detail page before returning its URL.
- If list items offer separate “comments/discussion” links and “title/article” links, click the title/article link unless the task explicitly asks for comments.
- On list pages with metadata/source links, click the title line (main link), not the source/domain/metadata line.
- If the task includes constraints, use on-screen evidence to select a qualifying item before answering.
- On list pages, identify a candidate item that matches constraints, then point to its title/image and click to open.
- Do not answer until you can confirm you are on the target page type (e.g., a single-item detail page).
- For “first/top result” tasks, click the topmost result item (not navigation, ads, or comments).
- When returning a URL, use the most recent page_info URL from the current page.
- Before final response for item-specific tasks, confirm the page type with moondream_query.
- If a click doesn't open the item, try a different target or a double-click by setting clicks: 2. If you suspect a new tab opened, use key_combination with ctrl+tab and re-check page_info.
- Use action result field state_changed to decide if a click/scroll had an effect; if false, adjust target or strategy.
- If the user specifies a site to search (e.g., Wikipedia), use that site's search first; only switch to another search engine if the site search fails.
- Never output placeholders like {{x}}, {{url}}, or <url_placeholder> in actions or final_response.
- Do not ask Moondream to infer the URL or page title; use page_info for those.
- If the task specifies a domain/URL, avoid leaving that domain unless the task explicitly requires it; if page_info shows an unexpected domain, go_back or navigate to the intended domain.
- If the task specifies a domain, your final_response URL must include that domain.
- After typing a search query, submit it (press_enter or search button). Avoid clicking unrelated suggestions or ads.
- For tasks like “first/top result,” ask Moondream to point at the first item or top result and click it.
- When moondream_point returns coordinates (x_norm/y_norm), use those exact numbers in click_at (x,y). Never use placeholders.
- Do not navigate to URLs derived from Moondream answers. Only navigate to URLs provided by the user or confirmed via page_info.
- If search results are not found after a couple of attempts, fallback to direct navigation to the most likely official page.
- Moondream query quality matters. Ask short, concrete, visual questions. Avoid vague or multi-part questions.
- When the task requires price or currency, verify the price on the detail page with a targeted Moondream query and return the exact text.
- For dense result grids, you may use moondream_detect with objects like "product image" or "item card" and click the topmost box.
- Never ask Moondream for a URL or link; only use page_info for URLs.

Moondream query examples (good vs bad):
GOOD: "Is there a search box on this page?"
BAD: "What should I do next?"
GOOD: "What is the exact price shown for the highlighted item?"
BAD: "Tell me everything about this page."
GOOD: "Is this a single-item detail page?"
BAD: "Is this page good?"
GOOD: "Which button says 'Sign in'?"
BAD: "Find the right thing."
BAD: "What is the URL for this page?"

Moondream query templates:
- Presence: "Is there a <thing> on the page?"
- Identification: "What is the exact text of the <thing>?"
- Page type: "Is this a <list/detail/login> page?"
- Verification: "Does the page show the item I just clicked?"
- Result matching: "Which result shows the domain <domain>?"
- If asked to use a search box, attempt a search interaction before using direct navigation; only fall back if stuck, and mention fallback in final_response.
- If the user requests JSON output, ensure final_response is valid JSON that matches the requested fields.
- When setting done=true, always include a non-empty final_response with concrete values (no placeholders like {{...}}).
- Stop when the task is complete by setting done=true and final_response.

JSON Schema:
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "actions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "action": {
            "type": "string",
            "enum": [
              "navigate",
              "click_at",
              "hover_at",
              "type_text_at",
              "scroll_document",
              "scroll_at",
              "go_back",
              "go_forward",
              "key_combination",
              "drag_and_drop",
              "wait",
              "moondream_query",
              "moondream_caption",
              "moondream_point",
              "moondream_detect",
              "page_info",
              "done",
              "fail"
            ]
          },
          "args": { "type": "object" }
        },
        "required": ["action", "args"],
        "additionalProperties": false
      }
    },
    "done": { "type": "boolean" },
    "final_response": { "type": "string" },
    "error": { "type": "string" }
  },
  "required": ["actions"],
  "additionalProperties": false
}

Examples (valid JSON):
{"actions":[{"action":"navigate","args":{"url":"https://example.com"}},{"action":"moondream_caption","args":{"length":"short"}}]}
{"actions":[{"action":"moondream_point","args":{"object":"login button"}},{"action":"click_at","args":{"x":512,"y":412}}]}
{"actions":[],"done":true,"final_response":"Logged in and reached the dashboard."}
{"actions":[],"done":true,"final_response":"{\"title\":\"Example Domain\",\"url\":\"https://example.com\"}"}
"""


@dataclass
class LlmOptions:
    max_iterations: int = 40
    temperature: float = 1.0
    max_completion_tokens: int = 65536
    top_p: float = 1
    post_action_wait_ms: int = 500
    reasoning_effort: str = "medium"


@dataclass
class StepLog:
    step: int
    action: str
    status: str
    detail: str
    output: Optional[str] = None


async def run_llm_agent(
    *,
    query: str,
    moondream: MoondreamClient,
    kernel_tool: ComputerTool,
    groq_api_key: str,
    options: LlmOptions,
) -> Dict[str, Any]:
    groq = Groq(api_key=groq_api_key)

    messages: List[Dict[str, str]] = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {
            "role": "user",
            "content": (
                "Task: "
                + query
                + "\nReturn a JSON object with an actions array. "
                + "Bundle multiple actions when sensible."
            ),
        },
    ]

    logs: List[StepLog] = []
    answers: List[str] = []
    last_screenshot: Optional[str] = None
    last_page_url: Optional[str] = None
    last_point_norm: Optional[Tuple[int, int]] = None
    error: Optional[str] = None

    for iteration in range(1, options.max_iterations + 1):
        try:
            raw = await asyncio.to_thread(
                _groq_completion,
                groq,
                messages,
                options,
            )
        except Exception as exc:
            messages.append(
                {
                    "role": "user",
                    "content": "Your last output was invalid. Return ONLY a JSON object that matches the schema.",
                }
            )
            try:
                raw = await asyncio.to_thread(
                    _groq_completion,
                    groq,
                    messages,
                    options,
                )
            except Exception as exc2:
                error = str(exc2)
                raw = '{"actions":[]}'

        batch_payload = _parse_json_action(raw)
        messages.append({"role": "assistant", "content": json.dumps(batch_payload)})

        actions = _normalize_actions(batch_payload)
        results: List[Dict[str, Any]] = []
        done_flag = bool(batch_payload.get("done"))
        final_response = str(batch_payload.get("final_response", "")) if done_flag else ""

        try:
            for action_item in actions:
                action = str(action_item.get("action", "")).strip()
                args = action_item.get("args") or {}
                if not action:
                    results.append({"action": "", "status": "failed", "detail": "missing action"})
                    continue

                if action == "navigate":
                    url = str(args.get("url", "")).strip()
                    if not url:
                        raise ValueError("navigate requires url")
                    if "{{" in url or "}}" in url or "placeholder" in url.lower():
                        logs.append(StepLog(iteration, action, "failed", "navigate url is placeholder"))
                        results.append(
                            {"action": action, "status": "failed", "detail": "navigate url is placeholder"}
                        )
                        continue
                    result = await kernel_tool.execute_action(ComputerAction.NAVIGATE, {"url": url})
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), f"Navigated to {url}"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": f"navigated to {url}",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "click_at":
                    try:
                        x, y = _coerce_coords(args, kernel_tool.screen_size)
                    except Exception as exc:
                        if last_point_norm:
                            x, y = last_point_norm
                            results.append(
                                {
                                    "action": action,
                                    "status": "success",
                                    "detail": "used last moondream_point",
                                    "used_last_point": True,
                                }
                            )
                        else:
                            logs.append(StepLog(iteration, action, "failed", str(exc)))
                            results.append({"action": action, "status": "failed", "detail": str(exc)})
                            continue
                    result = await kernel_tool.execute_action(
                        ComputerAction.CLICK_AT,
                        {"x": x, "y": y},
                    )
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), "Clicked at coordinates"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": "clicked",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "hover_at":
                    try:
                        x, y = _coerce_coords(args, kernel_tool.screen_size)
                    except Exception as exc:
                        logs.append(StepLog(iteration, action, "failed", str(exc)))
                        results.append({"action": action, "status": "failed", "detail": str(exc)})
                        continue
                    result = await kernel_tool.execute_action(
                        ComputerAction.HOVER_AT,
                        {"x": x, "y": y},
                    )
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), "Hovered at coordinates"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": "hovered",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "type_text_at":
                    try:
                        x, y = _coerce_coords(args, kernel_tool.screen_size)
                    except Exception as exc:
                        if last_point_norm:
                            x, y = last_point_norm
                            results.append(
                                {
                                    "action": action,
                                    "status": "success",
                                    "detail": "used last moondream_point",
                                    "used_last_point": True,
                                }
                            )
                        else:
                            logs.append(StepLog(iteration, action, "failed", str(exc)))
                            results.append({"action": action, "status": "failed", "detail": str(exc)})
                            continue
                    text = args.get("text")
                    if text is None:
                        raise ValueError("type_text_at requires text")
                    payload = {
                        "x": x,
                        "y": y,
                        "text": str(text),
                        "press_enter": bool(args.get("press_enter", False)),
                        "clear_before_typing": bool(args.get("clear_before_typing", True)),
                    }
                    result = await kernel_tool.execute_action(ComputerAction.TYPE_TEXT_AT, payload)
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), "Typed text"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": "typed",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "scroll_document":
                    direction = str(args.get("direction", "down"))
                    payload: Dict[str, Any] = {"direction": direction}
                    if args.get("magnitude") is not None:
                        payload["magnitude"] = int(args["magnitude"])
                    result = await kernel_tool.execute_action(ComputerAction.SCROLL_DOCUMENT, payload)
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), f"Scrolled {direction}"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": f"scrolled {direction}",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "scroll_at":
                    try:
                        x, y = _coerce_coords(args, kernel_tool.screen_size)
                    except Exception as exc:
                        logs.append(StepLog(iteration, action, "failed", str(exc)))
                        results.append({"action": action, "status": "failed", "detail": str(exc)})
                        continue
                    direction = str(args.get("direction", "down"))
                    payload = {"x": x, "y": y, "direction": direction}
                    if args.get("magnitude") is not None:
                        payload["magnitude"] = int(args["magnitude"])
                    result = await kernel_tool.execute_action(ComputerAction.SCROLL_AT, payload)
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), f"Scrolled {direction}"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": f"scrolled {direction}",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "go_back":
                    result = await kernel_tool.execute_action(ComputerAction.GO_BACK, {})
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), "Went back"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": "went back",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "go_forward":
                    result = await kernel_tool.execute_action(ComputerAction.GO_FORWARD, {})
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), "Went forward"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": "went forward",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "key_combination":
                    keys = str(args.get("keys", "")).strip()
                    if not keys:
                        raise ValueError("key_combination requires keys")
                    result = await kernel_tool.execute_action(ComputerAction.KEY_COMBINATION, {"keys": keys})
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), f"Pressed {keys}"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": f"pressed {keys}",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "wait":
                    seconds = float(args.get("seconds", 1))
                    await asyncio.sleep(seconds)
                    logs.append(StepLog(iteration, action, "success", f"Waited {seconds:.2f}s"))
                    results.append({"action": action, "status": "success", "detail": f"waited {seconds:.2f}s"})

                elif action == "moondream_query":
                    question = str(args.get("question", "")).strip()
                    if not question:
                        logs.append(StepLog(iteration, action, "failed", "Missing question"))
                        results.append(
                            {"action": action, "status": "failed", "detail": "missing question"}
                        )
                        continue
                    screenshot = await _ensure_screenshot(kernel_tool, last_screenshot)
                    last_screenshot = screenshot
                    answer = await moondream.query(screenshot, question)
                    answers.append(answer)
                    logs.append(StepLog(iteration, action, "success", "Answered question", output=answer))
                    results.append({"action": action, "status": "success", "answer": answer})

                elif action == "moondream_caption":
                    length = str(args.get("length", "normal"))
                    screenshot = await _ensure_screenshot(kernel_tool, last_screenshot)
                    last_screenshot = screenshot
                    caption = await moondream.caption(screenshot, length)
                    answers.append(caption)
                    logs.append(StepLog(iteration, action, "success", "Captioned image", output=caption))
                    results.append({"action": action, "status": "success", "caption": caption})

                elif action == "drag_and_drop":
                    if "x" not in args or "y" not in args:
                        raise ValueError("drag_and_drop requires x and y")
                    if "destination_x" not in args or "destination_y" not in args:
                        raise ValueError("drag_and_drop requires destination_x and destination_y")
                    try:
                        start_x, start_y = _coerce_coords(
                            {"x": args.get("x"), "y": args.get("y")},
                            kernel_tool.screen_size,
                        )
                        end_x, end_y = _coerce_coords(
                            {"x": args.get("destination_x"), "y": args.get("destination_y")},
                            kernel_tool.screen_size,
                        )
                    except Exception as exc:
                        logs.append(StepLog(iteration, action, "failed", str(exc)))
                        results.append({"action": action, "status": "failed", "detail": str(exc)})
                        continue
                    result = await kernel_tool.execute_action(
                        ComputerAction.DRAG_AND_DROP,
                        {
                            "x": start_x,
                            "y": start_y,
                            "destination_x": end_x,
                            "destination_y": end_y,
                        },
                    )
                    last_screenshot, state_changed = _update_screenshot_with_state(
                        result, last_screenshot
                    )
                    logs.append(StepLog(iteration, action, _status(result), "Dragged element"))
                    results.append(
                        {
                            "action": action,
                            "status": _status(result),
                            "detail": "dragged",
                            "state_changed": state_changed,
                        }
                    )
                    await _post_wait(action, args, options)

                elif action == "moondream_point":
                    obj = str(args.get("object", "")).strip()
                    if not obj:
                        raise ValueError("moondream_point requires object")
                    screenshot = await _ensure_screenshot(kernel_tool, last_screenshot)
                    last_screenshot = screenshot
                    point = await moondream.point(screenshot, obj)
                    if not point:
                        logs.append(StepLog(iteration, action, "failed", "No point found"))
                        results.append({"action": action, "status": "failed", "detail": "no point found"})
                    else:
                        screen = kernel_tool.screen_size
                        payload = _point_payload(point.x, point.y, screen)
                        last_point_norm = (payload["x_norm"], payload["y_norm"])
                        logs.append(StepLog(iteration, action, "success", "Point found", output=str(payload)))
                        results.append({"action": action, "status": "success", **payload})

                elif action == "moondream_detect":
                    obj = str(args.get("object", "")).strip()
                    if not obj:
                        raise ValueError("moondream_detect requires object")
                    screenshot = await _ensure_screenshot(kernel_tool, last_screenshot)
                    last_screenshot = screenshot
                    detections = await moondream.detect(screenshot, obj)
                    payload = _detect_payload(detections, kernel_tool.screen_size)
                    logs.append(StepLog(iteration, action, "success", "Detection results", output=str(payload)))
                    results.append({"action": action, "status": "success", **payload})

                elif action == "page_info":
                    payload = await _page_info(kernel_tool)
                    url_value = payload.get("url") if isinstance(payload, dict) else None
                    state_changed = bool(url_value and url_value != last_page_url)
                    if url_value:
                        last_page_url = str(url_value)
                    payload["state_changed"] = state_changed
                    status_value = "failed" if payload.get("error") else "success"
                    logs.append(
                        StepLog(iteration, action, status_value, "Page info", output=str(payload))
                    )
                    results.append({"action": action, "status": status_value, **payload})

                elif action == "done":
                    done_flag = True
                    final_response = str(args.get("final_response", ""))
                    break

                elif action == "fail":
                    error = str(args.get("error", "unknown error"))
                    logs.append(StepLog(iteration, action, "failed", error))
                    results.append({"action": action, "status": "failed", "detail": error})
                    done_flag = True
                    break

                else:
                    raise ValueError(f"Unknown action: {action}")

        except Exception as exc:
            message = str(exc)
            logs.append(StepLog(iteration, "batch", "failed", message))
            error = message
            results.append({"action": "batch", "status": "failed", "detail": message})

        _append_result(messages, "batch", {"results": results})

        if done_flag and (
            not final_response
            or "{{" in final_response
            or "}}" in final_response
            or "placeholder" in final_response.lower()
        ):
            messages.append(
                {
                    "role": "user",
                    "content": (
                        "final_response must be non-empty and use concrete values (no placeholders). "
                        "Return a corrected JSON object."
                    ),
                }
            )
            done_flag = False
            final_response = ""

        if done_flag:
            stripped = final_response.strip()
            if stripped.startswith("{"):
                try:
                    repaired = repair_json(stripped)
                    parsed = json.loads(repaired)
                    if not isinstance(parsed, dict):
                        raise ValueError("final_response JSON is not an object")
                except Exception:
                    messages.append(
                        {
                            "role": "user",
                            "content": (
                                "final_response looks like JSON but is invalid. "
                                "Return a valid JSON object string."
                            ),
                        }
                    )
                    done_flag = False
                    final_response = ""

        if done_flag:
            urls = _extract_urls(final_response)
            if urls and not last_page_url:
                messages.append(
                    {
                        "role": "user",
                        "content": (
                            "You returned a URL but did not call page_info. "
                            "Call page_info on the current page before final_response."
                        ),
                    }
                )
                done_flag = False
                final_response = ""
            elif urls and last_page_url and any(url != last_page_url for url in urls):
                messages.append(
                    {
                        "role": "user",
                        "content": (
                            "The returned URL does not match the current page_info URL. "
                            "Navigate to the correct page and then return that URL."
                        ),
                    }
                )
                done_flag = False
                final_response = ""

        if done_flag:
            summary = f"Completed {sum(1 for log in logs if log.status == 'success')}/{len(logs)} steps"
            result_payload = {
                "summary": summary,
                "final_response": final_response,
                "steps": [log.__dict__ for log in logs],
                "answers": answers,
            }
            return {"final_response": json.dumps(result_payload, indent=2), "error": error}

    summary = f"Completed {sum(1 for log in logs if log.status == 'success')}/{len(logs)} steps"
    result_payload = {
        "summary": summary,
        "final_response": "",
        "steps": [log.__dict__ for log in logs],
        "answers": answers,
    }

    return {"final_response": json.dumps(result_payload, indent=2), "error": error}


def _groq_completion(groq: Groq, messages: List[Dict[str, str]], options: LlmOptions) -> str:
    completion = groq.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        temperature=options.temperature,
        max_completion_tokens=options.max_completion_tokens,
        top_p=options.top_p,
        reasoning_effort=options.reasoning_effort,
        stream=False,
        response_format={"type": "json_object"},
    )
    return completion.choices[0].message.content or ""


def _parse_json_action(raw: str) -> Dict[str, Any]:
    start = raw.find("{")
    end = raw.rfind("}")
    if start == -1 or end == -1 or end <= start:
        raise ValueError("No JSON object found in LLM response")
    snippet = raw[start : end + 1]
    repaired = repair_json(snippet)
    data = json.loads(repaired)
    if not isinstance(data, dict):
        raise ValueError("LLM JSON did not produce an object")
    return data


def _extract_urls(final_response: str) -> List[str]:
    text = final_response.strip()
    if not (text.startswith("{") and text.endswith("}")):
        return []
    try:
        repaired = repair_json(text)
        data = json.loads(repaired)
    except Exception:
        return []
    if not isinstance(data, dict):
        return []
    urls: List[str] = []
    for key, value in data.items():
        if "url" in str(key).lower() and isinstance(value, str):
            urls.append(value)
    return urls


def _normalize_actions(payload: Dict[str, Any]) -> List[Dict[str, Any]]:
    if "actions" in payload and isinstance(payload["actions"], list):
        return [item for item in payload["actions"] if isinstance(item, dict)]
    action = payload.get("action")
    args = payload.get("args") if isinstance(payload.get("args"), dict) else {}
    if action:
        return [{"action": action, "args": args}]
    return []


def _append_result(messages: List[Dict[str, str]], action: str, payload: Any) -> None:
    messages.append(
        {
            "role": "user",
            "content": json.dumps({"type": "action_result", "action": action, "output": payload}),
        }
    )


def _status(result: Any) -> str:
    return "failed" if result and getattr(result, "error", None) else "success"


def _update_screenshot(result: Any, last_screenshot: Optional[str]) -> Optional[str]:
    if result and getattr(result, "base64_image", None):
        return result.base64_image
    return last_screenshot


def _update_screenshot_with_state(
    result: Any, last_screenshot: Optional[str]
) -> Tuple[Optional[str], bool]:
    new_screenshot = _update_screenshot(result, last_screenshot)
    if new_screenshot is None:
        return last_screenshot, False
    if last_screenshot is None:
        return new_screenshot, True
    return new_screenshot, new_screenshot != last_screenshot


def _coerce_coords(args: Dict[str, Any], screen_size: ScreenSize) -> Tuple[int, int]:
    if "x" not in args or "y" not in args:
        raise ValueError("x and y are required")
    if isinstance(args.get("x"), str) and ("{" in args["x"] or "}" in args["x"]):
        raise ValueError("x must be a number, not a placeholder")
    if isinstance(args.get("y"), str) and ("{" in args["y"] or "}" in args["y"]):
        raise ValueError("y must be a number, not a placeholder")
    x = float(args["x"])
    y = float(args["y"])
    if 0 <= x <= 1 and 0 <= y <= 1:
        return int(x * COORDINATE_SCALE), int(y * COORDINATE_SCALE)
    if 0 <= x <= COORDINATE_SCALE and 0 <= y <= COORDINATE_SCALE:
        return int(x), int(y)
    width = screen_size.width if screen_size else DEFAULT_SCREEN_SIZE.width
    height = screen_size.height if screen_size else DEFAULT_SCREEN_SIZE.height
    return int((x / width) * COORDINATE_SCALE), int((y / height) * COORDINATE_SCALE)


async def _ensure_screenshot(computer: ComputerTool, last_screenshot: Optional[str]) -> str:
    if last_screenshot:
        return last_screenshot
    result = await computer.screenshot()
    if result.error or not result.base64_image:
        raise RuntimeError(result.error or "Failed to capture screenshot")
    return result.base64_image


async def _post_wait(action: str, args: Dict[str, Any], options: LlmOptions) -> None:
    wait_actions = {
        "navigate",
        "click_at",
        "hover_at",
        "type_text_at",
        "scroll_document",
        "scroll_at",
        "go_back",
        "go_forward",
        "key_combination",
        "drag_and_drop",
    }
    if action not in wait_actions:
        return
    override = args.get("post_wait_ms")
    if isinstance(override, (int, float)):
        wait_ms = int(override)
    else:
        wait_ms = int(options.post_action_wait_ms)
    if wait_ms > 0:
        await asyncio.sleep(wait_ms / 1000)


async def _page_info(kernel_tool: ComputerTool) -> Dict[str, Any]:
    try:
        from playwright.async_api import async_playwright
    except Exception:
        return {"error": "playwright not installed"}

    try:
        browser_info = kernel_tool.kernel.browsers.retrieve(kernel_tool.session_id)
        cdp_url = getattr(browser_info, "cdp_ws_url", None)
    except Exception as exc:
        return {"error": f"failed to retrieve cdp url: {exc}"}

    if not cdp_url:
        return {"error": "cdp url not available"}

    try:
        async with async_playwright() as p:
            browser = await p.chromium.connect_over_cdp(cdp_url)
            pages = []
            for context in browser.contexts:
                pages.extend(context.pages)
            if not pages:
                page = await browser.new_page()
            else:
                page = pages[-1]
            title = await page.title()
            url = page.url
            await browser.close()
        return {"url": url, "title": title}
    except Exception as exc:
        return {"error": f"playwright error: {exc}"}


def _point_payload(x: float, y: float, screen: ScreenSize) -> Dict[str, Any]:
    x_norm = int(x * COORDINATE_SCALE)
    y_norm = int(y * COORDINATE_SCALE)
    x_px = int(x * screen.width)
    y_px = int(y * screen.height)
    return {
        "x": x,
        "y": y,
        "x_norm": x_norm,
        "y_norm": y_norm,
        "x_px": x_px,
        "y_px": y_px,
        "screen": {"width": screen.width, "height": screen.height},
    }


def _detect_payload(detections: List[Dict[str, float]], screen: ScreenSize) -> Dict[str, Any]:
    converted: List[Dict[str, Any]] = []
    for det in detections:
        x_min = float(det.get("x_min", 0))
        y_min = float(det.get("y_min", 0))
        x_max = float(det.get("x_max", 0))
        y_max = float(det.get("y_max", 0))
        converted.append(
            {
                "x_min": x_min,
                "y_min": y_min,
                "x_max": x_max,
                "y_max": y_max,
                "x_min_norm": int(x_min * COORDINATE_SCALE),
                "y_min_norm": int(y_min * COORDINATE_SCALE),
                "x_max_norm": int(x_max * COORDINATE_SCALE),
                "y_max_norm": int(y_max * COORDINATE_SCALE),
                "x_min_px": int(x_min * screen.width),
                "y_min_px": int(y_min * screen.height),
                "x_max_px": int(x_max * screen.width),
                "y_max_px": int(y_max * screen.height),
            }
        )
    return {"objects": converted, "screen": {"width": screen.width, "height": screen.height}}
