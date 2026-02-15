"""
Fireworks REST API client for dataset upload and RFT job creation.

Uses the Fireworks REST API directly (no eval-protocol CLI).
See: https://docs.fireworks.ai/fine-tuning/fine-tuning-via-api
See: https://docs.fireworks.ai/api-reference/create-reinforcement-fine-tuning-job
"""

from __future__ import annotations

import logging
import time
from pathlib import Path
from typing import Any

import httpx

logger = logging.getLogger(__name__)

BASE_URL = "https://api.fireworks.ai"
DATASET_STATE_READY = "READY"
POLL_INTERVAL_SEC = 5
DATASET_POLL_TIMEOUT_SEC = 300  # 5 minutes
JOB_POLL_TIMEOUT_SEC = 600  # 10 minutes


def _headers(api_key: str) -> dict[str, str]:
    return {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }


def create_dataset(account_id: str, dataset_id: str, api_key: str) -> dict[str, Any]:
    """
    Create a dataset record on Fireworks.

    POST /v1/accounts/{account_id}/datasets
    """
    url = f"{BASE_URL}/v1/accounts/{account_id}/datasets"
    body = {
        "datasetId": dataset_id,
        "dataset": {"userUploaded": {}},
    }
    with httpx.Client(timeout=60.0) as client:
        resp = client.post(url, json=body, headers=_headers(api_key))
        resp.raise_for_status()
        return resp.json()


def upload_dataset(
    account_id: str, dataset_id: str, file_path: str | Path, api_key: str
) -> None:
    """
    Upload a JSONL file to the dataset (for files < 150MB).

    POST /v1/accounts/{account_id}/datasets/{dataset_id}:upload
    """
    url = f"{BASE_URL}/v1/accounts/{account_id}/datasets/{dataset_id}:upload"
    path = Path(file_path)
    if not path.exists():
        raise FileNotFoundError(f"Dataset file not found: {path}")
    with open(path, "rb") as f:
        files = {"file": (path.name, f, "application/jsonl")}
        with httpx.Client(timeout=120.0) as client:
            resp = client.post(
                url,
                files=files,
                headers={"Authorization": f"Bearer {api_key}"},
            )
            resp.raise_for_status()


def get_dataset(account_id: str, dataset_id: str, api_key: str) -> dict[str, Any]:
    """GET /v1/accounts/{account_id}/datasets/{dataset_id}"""
    url = f"{BASE_URL}/v1/accounts/{account_id}/datasets/{dataset_id}"
    with httpx.Client(timeout=30.0) as client:
        resp = client.get(url, headers=_headers(api_key))
        resp.raise_for_status()
        return resp.json()


def wait_for_dataset_ready(
    account_id: str, dataset_id: str, api_key: str, timeout_sec: int = DATASET_POLL_TIMEOUT_SEC
) -> None:
    """Poll until dataset state is READY."""
    deadline = time.monotonic() + timeout_sec
    while time.monotonic() < deadline:
        data = get_dataset(account_id, dataset_id, api_key)
        state = data.get("state") or data.get("dataset", {}).get("state")
        if state == DATASET_STATE_READY:
            return
        time.sleep(POLL_INTERVAL_SEC)
    raise TimeoutError(f"Dataset {dataset_id} did not become READY within {timeout_sec}s")


def create_rft_job(
    account_id: str,
    api_key: str,
    dataset: str,
    evaluator: str,
    training_config: dict[str, Any],
    *,
    display_name: str | None = None,
    chunk_size: int | None = None,
    inference_parameters: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """
    Create a Reinforcement Fine-Tuning job.

    POST /v1/accounts/{account_id}/reinforcementFineTuningJobs

    Args:
        account_id: Fireworks account ID (from dashboard).
        api_key: Fireworks API key.
        dataset: Dataset resource name, e.g. accounts/{account_id}/datasets/{dataset_id}.
        evaluator: Evaluator resource name, e.g. accounts/{account_id}/evaluators/{evaluator_id}.
        training_config: Must include baseModel; optional outputModel, epochs, batchSize, etc.
        display_name: Optional job display name.
        chunk_size: Optional chunk size for rollout (default 200 when dataset > 300 rows).
        inference_parameters: Optional inference params (temperature, maxOutputTokens, etc.).
    """
    url = f"{BASE_URL}/v1/accounts/{account_id}/reinforcementFineTuningJobs"
    body: dict[str, Any] = {
        "dataset": dataset,
        "evaluator": evaluator,
        "trainingConfig": training_config,
    }
    if display_name:
        body["displayName"] = display_name
    if chunk_size is not None:
        body["chunkSize"] = chunk_size
    if inference_parameters:
        body["inferenceParameters"] = inference_parameters

    with httpx.Client(timeout=60.0) as client:
        resp = client.post(url, json=body, headers=_headers(api_key))
        resp.raise_for_status()
        return resp.json()


def get_rft_job(account_id: str, job_id: str, api_key: str) -> dict[str, Any]:
    """GET /v1/accounts/{account_id}/reinforcementFineTuningJobs/{job_id}"""
    url = f"{BASE_URL}/v1/accounts/{account_id}/reinforcementFineTuningJobs/{job_id}"
    with httpx.Client(timeout=30.0) as client:
        resp = client.get(url, headers=_headers(api_key))
        resp.raise_for_status()
        return resp.json()


def dataset_resource_name(account_id: str, dataset_id: str) -> str:
    """Build full dataset resource name for RFT job."""
    return f"accounts/{account_id}/datasets/{dataset_id}"


def evaluator_resource_name(account_id: str, evaluator_id: str) -> str:
    """Build full evaluator resource name for RFT job."""
    if "/" in evaluator_id and evaluator_id.startswith("accounts/"):
        return evaluator_id
    return f"accounts/{account_id}/evaluators/{evaluator_id}"
