"""
WebJudge - LLM-as-Judge for Web Navigation Trajectories.

Ported from Online-Mind2Web. Uses a 3-phase evaluation approach:
1. Key Point Identification - Extract critical elements from task description
2. Key Screenshot Selection - Score each screenshot for relevance (1-5)
3. Outcome Judgment - Final success/failure determination

References:
    https://github.com/OSU-NLP-Group/Online-Mind2Web
    https://arxiv.org/abs/2504.01382 (Online-Mind2Web paper)

Recommended model: gpt-5-mini (previously o4-mini with 85.7% human agreement per the paper).
"""

from __future__ import annotations

import asyncio
import base64
import io
import logging
import os
import re
from dataclasses import dataclass, field

from openai import AsyncOpenAI, APITimeoutError, RateLimitError, APIConnectionError
from PIL import Image

from .base import RewardModel, Trajectory

logger = logging.getLogger(__name__)

# Configuration
SCORE_THRESHOLD = 3  # Screenshots scoring more than this value are "key screenshots"
MAX_IMAGES = 50  # Maximum images to include in final judgment

# Default evaluation criteria (from Online-Mind2Web, designed for e-commerce/filtering tasks)
DEFAULT_EVALUATION_CRITERIA = """1: The filtered results must be displayed correctly. If filters were not properly applied (i.e., missing selection, missing confirmation, or no visible effect in results), the task is not considered successful.
2: You must carefully check whether these snapshots and action history meet these key points. Ensure that specific filter conditions, such as "best," "highest," "cheapest," "latest," "most recent," "lowest," "closest," "highest-rated," "largest," and "newest" are correctly applied using the filter function(e.g., sort function).
3: Certain key points or requirements should be applied by the filter. Otherwise, a search with all requirements as input will be deemed a failure since it cannot guarantee that all results meet the requirements!
4: If the task requires filtering by a specific range of money, years, or the number of beds and bathrooms, the applied filter must exactly match the given requirement. Any deviation results in failure. To ensure the task is successful, the applied filter must precisely match the specified range without being too broad or too narrow.
Examples of Failure Cases:
- If the requirement is less than $50, but the applied filter is less than $25, it is a failure.
- If the requirement is $1500-$2500, but the applied filter is $2000-$2500, it is a failure.
- If the requirement is $25-$200, but the applied filter is $0-$200, it is a failure.
- If the required years are 2004-2012, but the filter applied is 2001-2012, it is a failure.
- If the required years are before 2015, but the applied filter is 2000-2014, it is a failure.
- If the task requires exactly 2 beds, but the filter applied is 2+ beds, it is a failure.
5: Some tasks require a submission action or a display of results to be considered successful.
6: If the retrieved information is invalid or empty(e.g., No match was found), but the agent has correctly performed the required action, it should still be considered successful.
7: If the current page already displays all available items, then applying a filter is not necessary. As long as the agent selects items that meet the requirements (e.g., the cheapest or lowest price), the task is still considered successful."""


def encode_image(image: Image.Image) -> str:
    """Convert a PIL image to base64 JPEG string."""
    if image.mode == "RGBA":
        image = image.convert("RGB")
    buffered = io.BytesIO()
    image.save(buffered, format="JPEG", quality=85)
    return base64.b64encode(buffered.getvalue()).decode("utf-8")


@dataclass
class WebJudgeResult:
    """
    Result from WebJudge evaluation.

    Includes the standard evaluation fields plus WebJudge-specific details.
    """

    # Standard evaluation fields
    success: bool
    score: float  # Typically 0.0 or 1.0 for RL
    reasoning: str = ""
    metadata: dict = field(default_factory=dict)

    # WebJudge-specific fields
    key_points: str = ""
    screenshot_scores: list[dict] = field(default_factory=list)


class WebJudge(RewardModel):
    """
    WebJudge evaluator for web navigation trajectories.

    Three-phase evaluation:
    1. Key Point Identification - extract task requirements
    2. Key Screenshot Selection - score each screenshot for relevance
    3. Outcome Judgment - final success/failure determination

    Usage:
        webjudge = WebJudge(model="openai/gpt-5-mini")
        result = await webjudge.evaluate(trajectory)
        print(f"Success: {result.success}, Score: {result.score}")
    """

    def __init__(
        self,
        model: str = "gpt-5-mini",
        api_key: str | None = None,
        base_url: str | None = None,
        evaluation_criteria: str | None = None,
    ):
        """
        Initialize WebJudge.

        Args:
            model: Model name (e.g., "gpt-5-mini")
            api_key: API key for OpenAI. Falls back to OPENAI_API_KEY env var.
            base_url: Optional API base URL. Defaults to OpenAI.
            evaluation_criteria: Custom evaluation criteria for the judgment phase.
                If None, uses DEFAULT_EVALUATION_CRITERIA (designed for e-commerce tasks).
                Override this for different task types (e.g., authentication, navigation).
        """
        self.client = AsyncOpenAI(
            api_key=api_key or os.getenv("OPENAI_API_KEY"),
            base_url=base_url,
            timeout=600.0
        )
        self.model = model
        self.evaluation_criteria = evaluation_criteria

    async def evaluate(self, trajectory: Trajectory) -> WebJudgeResult:
        """
        Run full WebJudge evaluation on a trajectory.

        Args:
            trajectory: Trajectory to evaluate

        Returns:
            WebJudgeResult with success/failure, score, key points, and reasoning
        """
        # Phase 1: Identify key points from task description
        key_points = await self._identify_key_points(trajectory.task)

        # Phase 2: Score each screenshot for relevance
        screenshot_scores = await self._score_screenshots(
            trajectory.task, key_points, trajectory.screenshots
        )

        # Phase 3: Final judgment with key screenshots
        key_screenshots = [
            (trajectory.screenshots[i], score["reasoning"])
            for i, score in enumerate(screenshot_scores)
            if score["score"] >= SCORE_THRESHOLD
        ][:MAX_IMAGES]

        success, reasoning = await self._judge_outcome(
            task=trajectory.task,
            key_points=key_points,
            action_history=trajectory.action_history,
            key_screenshots=key_screenshots,
        )

        return WebJudgeResult(
            success=success,
            score=1.0 if success else 0.0,
            key_points=key_points,
            reasoning=reasoning,
            screenshot_scores=screenshot_scores,
        )

    async def evaluate_batch(self, trajectories: list[Trajectory]) -> list[WebJudgeResult]:
        """Evaluate multiple trajectories in parallel."""
        tasks = [self.evaluate(t) for t in trajectories]
        return list(await asyncio.gather(*tasks))

    async def _identify_key_points(self, task: str) -> str:
        """Phase 1: Extract key points from task description."""
        system_msg = """You are an expert tasked with analyzing a given task to identify the key points explicitly stated in the task description.

**Objective**: Carefully analyze the task description and extract the critical elements explicitly mentioned in the task for achieving its goal.

**Instructions**:
1. Read the task description carefully.
2. Identify and extract **key points** directly stated in the task description.
    - A **key point** is a critical element, condition, or step explicitly mentioned in the task description.
    - Do not infer or add any unstated elements.
    - Words such as "best," "highest," "cheapest," "latest," "most recent," "lowest," "closest," "highest-rated," "largest," and "newest" must go through the sort function(e.g., the key point should be "Filter by highest").

**Respond with**:
- **Key Points**: A numbered list of the explicit key points for completing this task, one per line, without explanations or additional details."""

        messages = [
            {"role": "system", "content": system_msg},
            {"role": "user", "content": f"Task: {task}"},
        ]

        try:
            response = await self.client.chat.completions.create(
                model=self.model,
                messages=messages,
            )
        except APITimeoutError as e:
            logger.error(f"WebJudge key points identification timed out: {e}")
            raise
        except RateLimitError as e:
            logger.error(f"WebJudge key points identification rate limited: {e}")
            raise
        except APIConnectionError as e:
            logger.error(f"WebJudge key points identification connection error: {e}")
            raise
        except Exception as e:
            logger.error(f"WebJudge key points identification failed: {type(e).__name__}: {e}")
            raise

        content = response.choices[0].message.content or ""

        if "**Key Points**:" in content:
            content = content.split("**Key Points**:")[1]
        elif "Key Points:" in content:
            content = content.split("Key Points:")[-1]

        return content.strip()

    async def _score_screenshots(
        self, task: str, key_points: str, screenshots: list[Image.Image]
    ) -> list[dict]:
        """Phase 2: Score each screenshot for relevance (1-5)."""
        tasks = [
            self._score_single_screenshot(task, key_points, screenshot)
            for screenshot in screenshots
        ]
        results = await asyncio.gather(*tasks)
        return list(results)

    async def _score_single_screenshot(
        self, task: str, key_points: str, screenshot: Image.Image
    ) -> dict:
        """Score a single screenshot."""
        system_msg = """You are an expert evaluator tasked with determining whether an image contains information about the necessary steps to complete a task.

**Objective**: Analyze the provided image and decide if it shows essential steps or evidence required for completing the task. Use your reasoning to explain your decision before assigning a score.

**Instructions**:
1. Provide a detailed description of the image, including its contents, visible elements, text (if any), and any notable features.

2. Carefully examine the image and evaluate whether it contains necessary steps or evidence crucial to task completion:
- Identify key points that could be relevant to task completion, such as actions, progress indicators, tool usage, applied filters, or step-by-step instructions.
- Does the image show actions, progress indicators, or critical information directly related to completing the task?
- Is this information indispensable for understanding or ensuring task success?
- If the image contains partial but relevant information, consider its usefulness rather than dismissing it outright.

3. Provide your response in the following format:
- **Reasoning**: Explain your thought process and observations. Mention specific elements in the image that indicate necessary steps, evidence, or lack thereof.
- **Score**: Assign a score based on the reasoning, using the following scale:
    - **1**: The image does not contain any necessary steps or relevant information.
    - **2**: The image contains minimal or ambiguous information, unlikely to be essential.
    - **3**: The image includes some relevant steps or hints but lacks clarity or completeness.
    - **4**: The image contains important steps or evidence that are highly relevant but not fully comprehensive.
    - **5**: The image clearly displays necessary steps or evidence crucial for completing the task.

Respond with:
1. **Reasoning**: [Your explanation]
2. **Score**: [1-5]"""

        jpg_base64_str = encode_image(screenshot)

        prompt = f"""**Task**: {task}

**Key Points for Task Completion**: {key_points}

The snapshot of the web page is shown in the image."""

        try:
            response = await self.client.chat.completions.create(
                model=self.model,
                messages=[
                    {"role": "system", "content": system_msg},
                    {
                        "role": "user",
                        "content": [
                            {"type": "text", "text": prompt},
                            {
                                "type": "image_url",
                                "image_url": {
                                    "url": f"data:image/jpeg;base64,{jpg_base64_str}",
                                    "detail": "high",
                                },
                            },
                        ],
                    },
                ],
            )

            content = response.choices[0].message.content or ""

            score_match = re.search(r"\*?\*?Score\*?\*?:?\s*(\d)", content)
            score = int(score_match.group(1)) if score_match else 1

            reasoning = ""
            if "**Reasoning**:" in content:
                reasoning = content.split("**Reasoning**:")[1].split("**Score")[0].strip()

            return {"score": score, "reasoning": reasoning}

        except APITimeoutError as e:
            logger.error(f"WebJudge screenshot scoring timed out: {e}")
            return {"score": 0, "reasoning": f"Timeout: {e}"}
        except RateLimitError as e:
            logger.error(f"WebJudge screenshot scoring rate limited: {e}")
            return {"score": 0, "reasoning": f"Rate limit: {e}"}
        except APIConnectionError as e:
            logger.error(f"WebJudge screenshot scoring connection error: {e}")
            return {"score": 0, "reasoning": f"Connection error: {e}"}
        except Exception as e:
            logger.error(f"WebJudge screenshot scoring failed: {type(e).__name__}: {e}")
            return {"score": 0, "reasoning": f"Error: {e}"}

    async def _judge_outcome(
        self,
        task: str,
        key_points: str,
        action_history: list[str],
        key_screenshots: list[tuple[Image.Image, str]],
    ) -> tuple[bool, str]:
        """Phase 3: Final outcome judgment."""
        criteria = self.evaluation_criteria or DEFAULT_EVALUATION_CRITERIA
        system_msg = f"""You are an expert in evaluating the performance of a web navigation agent. The agent is designed to help a human user navigate a website to complete a task. Given the user's task, the agent's action history, key points for task completion, some potentially important web pages in the agent's trajectory and their reasons, your goal is to determine whether the agent has completed the task and achieved all requirements.

Your response must strictly follow the following evaluation criteria!
*Important Evaluation Criteria*:
{criteria}

*IMPORTANT*
Format your response into two lines as shown below:

Thoughts: <your thoughts>
Status: "success" or "failure"
"""

        action_text = "\n".join(f"{i + 1}. {action}" for i, action in enumerate(action_history))

        thoughts_text = "\n".join(
            f"{i + 1}. {reasoning}" for i, (_, reasoning) in enumerate(key_screenshots)
        )

        if key_screenshots:
            prompt = f"""User Task: {task}

Key Points: {key_points}

Action History:
{action_text}

The potentially important snapshots of the webpage in the agent's trajectory and their reasons:
{thoughts_text}"""
        else:
            prompt = f"""User Task: {task}

Key Points: {key_points}

Action History:
{action_text}"""

        content: list[dict] = [{"type": "text", "text": prompt}]
        for screenshot, _ in key_screenshots:
            jpg_base64_str = encode_image(screenshot)
            content.append(
                {
                    "type": "image_url",
                    "image_url": {
                        "url": f"data:image/jpeg;base64,{jpg_base64_str}",
                        "detail": "high",
                    },
                }
            )

        try:
            response = await self.client.chat.completions.create(
                model=self.model,
                messages=[
                    {"role": "system", "content": system_msg},
                    {"role": "user", "content": content},
                ],
            )
        except APITimeoutError as e:
            logger.error(f"WebJudge outcome judgment timed out: {e}")
            raise
        except RateLimitError as e:
            logger.error(f"WebJudge outcome judgment rate limited: {e}")
            raise
        except APIConnectionError as e:
            logger.error(f"WebJudge outcome judgment connection error: {e}")
            raise
        except Exception as e:
            logger.error(f"WebJudge outcome judgment failed: {type(e).__name__}: {e}")
            raise

        result = response.choices[0].message.content or ""

        success = False
        if "status:" in result.lower():
            status_part = result.lower().split("status:")[1]
            success = "success" in status_part

        return success, result
