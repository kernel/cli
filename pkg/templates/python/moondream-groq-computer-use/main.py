import os
from typing import List, Optional, TypedDict

import kernel
from llm_loop import LlmOptions, run_llm_agent
from loop import AgentOptions, run_agent
from moondream import MoondreamClient
from session import KernelBrowserSession
from tools import ComputerTool


class StepInput(TypedDict, total=False):
    action: str
    url: str
    target: str
    text: str
    question: str
    direction: str
    magnitude: int
    x: float
    y: float
    keys: str
    seconds: float
    retries: int
    retry_delay_ms: int
    pre_wait_ms: int
    press_enter: bool
    clear_before_typing: bool
    length: str


class QueryInput(TypedDict, total=False):
    query: str
    steps: List[StepInput]
    record_replay: bool
    max_retries: int
    retry_delay_ms: int
    strict: bool
    max_iterations: int
    post_action_wait_ms: int


class QueryOutput(TypedDict):
    result: str
    replay_url: Optional[str]
    error: Optional[str]


api_key = os.getenv("MOONDREAM_API_KEY")
if not api_key:
    raise ValueError(
        "MOONDREAM_API_KEY is not set. "
        "Set it via environment variable or deploy with: kernel deploy main.py --env-file .env"
    )
groq_key = os.getenv("GROQ_API_KEY")

app = kernel.App("python-moondream-cua")


@app.action("cua-task")
async def cua_task(
    ctx: kernel.KernelContext,
    payload: QueryInput,
) -> QueryOutput:
    if not payload or not (payload.get("query") or payload.get("steps")):
        raise ValueError(
            "Query is required. Payload must include: {\"query\": \"your task description\"}"
        )

    record_replay = payload.get("record_replay", False)
    options = AgentOptions(
        max_retries=int(payload.get("max_retries", 3)),
        retry_delay_ms=int(payload.get("retry_delay_ms", 1000)),
        strict=bool(payload.get("strict", False)),
    )
    llm_options = LlmOptions(
        max_iterations=int(payload.get("max_iterations", 40)),
        post_action_wait_ms=int(payload.get("post_action_wait_ms", 500)),
    )

    async with KernelBrowserSession(
        stealth=True,
        record_replay=record_replay,
    ) as session:
        print("Kernel browser live view url:", session.live_view_url)

        async with MoondreamClient(api_key=str(api_key)) as moondream:
            if payload.get("steps"):
                result = await run_agent(
                    query=payload.get("query"),
                    steps=payload.get("steps"),
                    moondream=moondream,
                    kernel=session.kernel,
                    session_id=session.session_id,
                    options=options,
                )
            else:
                if not groq_key:
                    raise ValueError(
                        "GROQ_API_KEY is not set. "
                        "Set it via environment variable or deploy with: kernel deploy main.py --env-file .env"
                    )
                result = await run_llm_agent(
                    query=str(payload.get("query")),
                    moondream=moondream,
                    kernel_tool=ComputerTool(session.kernel, session.session_id),
                    groq_api_key=str(groq_key),
                    options=llm_options,
                )

    return {
        "result": result.get("final_response", ""),
        "replay_url": session.replay_view_url,
        "error": result.get("error"),
    }


if __name__ == "__main__":
    import asyncio

    async def main():
        test_query = "Navigate to https://example.com and describe the page"

        print(f"Running local test with query: {test_query}")

        async with KernelBrowserSession(
            stealth=True,
            record_replay=False,
        ) as session:
            print("Kernel browser live view url:", session.live_view_url)

            async with MoondreamClient(api_key=str(api_key)) as moondream:
                try:
                    if not groq_key:
                        raise ValueError("GROQ_API_KEY is required for local LLM test")

                    result = await run_llm_agent(
                        query=test_query,
                        moondream=moondream,
                        kernel_tool=ComputerTool(session.kernel, session.session_id),
                        groq_api_key=str(groq_key),
                        options=LlmOptions(),
                    )

                    print("Result:", result.get("final_response", ""))
                    if result.get("error"):
                        print("Error:", result.get("error"))
                except Exception as e:
                    print(f"Local execution failed: {e}")
                    raise

    asyncio.run(main())
