"""
Base reward model interface for RL training.

Defines abstract interfaces for trajectory evaluation that can be
implemented by different reward models (WebJudge, custom models, etc.).
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field

from PIL import Image


@dataclass
class Trajectory:
    """
    Trajectory data structure for reward model evaluation.

    Represents a complete agent trajectory through an environment.
    """

    task_id: str
    task: str  # Task description
    action_history: list[str]  # Action descriptions
    screenshots: list[Image.Image]  # Screenshot after each action
    initial_url: str | None = None
    final_url: str | None = None
    metadata: dict = field(default_factory=dict)


@dataclass
class EvaluationResult:
    """Result from reward model evaluation."""

    success: bool
    score: float  # Typically 0.0 or 1.0 for RL
    reasoning: str = ""
    metadata: dict = field(default_factory=dict)


class RewardModel(ABC):
    """
    Abstract base class for reward models.

    Reward models evaluate agent trajectories and return scores
    for use in RL training.
    """

    @abstractmethod
    async def evaluate(self, trajectory: Trajectory) -> EvaluationResult:
        """
        Evaluate a trajectory and return a reward score.

        Args:
            trajectory: The trajectory to evaluate

        Returns:
            EvaluationResult with success/failure, score, and reasoning
        """
        ...

    @abstractmethod
    async def evaluate_batch(self, trajectories: list[Trajectory]) -> list[EvaluationResult]:
        """
        Evaluate multiple trajectories in batch.

        Default implementation evaluates sequentially.
        Subclasses can override for parallel evaluation.

        Args:
            trajectories: List of trajectories to evaluate

        Returns:
            List of EvaluationResults in the same order
        """
        ...


class HeuristicRewardModel(RewardModel):
    """
    Simple heuristic reward model based on action success.

    Returns 1.0 if the trajectory ended with a successful terminal action,
    0.0 otherwise.
    """

    async def evaluate(self, trajectory: Trajectory) -> EvaluationResult:
        """Evaluate based on final action."""
        if not trajectory.action_history:
            return EvaluationResult(
                success=False,
                score=0.0,
                reasoning="No actions taken",
            )

        final_action = trajectory.action_history[-1].lower()

        # Check for success indicators
        if "success" in final_action or "terminate" in final_action:
            return EvaluationResult(
                success=True,
                score=1.0,
                reasoning=f"Task completed with: {final_action}",
            )

        return EvaluationResult(
            success=False,
            score=0.0,
            reasoning=f"Task ended with: {final_action}",
        )

    async def evaluate_batch(self, trajectories: list[Trajectory]) -> list[EvaluationResult]:
        """Evaluate multiple trajectories."""
        return [await self.evaluate(t) for t in trajectories]
