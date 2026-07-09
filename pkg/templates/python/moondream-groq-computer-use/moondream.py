"""Async client for the Moondream API."""

from __future__ import annotations

import base64
from dataclasses import dataclass
from typing import Any, Optional

import httpx


@dataclass
class MoondreamPoint:
    x: float
    y: float


class MoondreamError(RuntimeError):
    pass


class MoondreamClient:
    def __init__(
        self,
        api_key: str,
        base_url: str = "https://api.moondream.ai/v1",
        timeout: float = 30.0,
    ) -> None:
        self._client = httpx.AsyncClient(
            base_url=base_url,
            timeout=timeout,
            headers={
                "Content-Type": "application/json",
                "X-Moondream-Auth": api_key,
            },
        )

    async def __aenter__(self) -> "MoondreamClient":
        return self

    async def __aexit__(self, exc_type, exc, tb) -> None:
        await self.close()

    async def close(self) -> None:
        await self._client.aclose()

    async def query(self, image_base64: str, question: str, reasoning: Optional[bool] = None) -> str:
        payload: dict[str, Any] = {
            "image_url": _to_data_url(image_base64),
            "question": question,
        }
        if reasoning is not None:
            payload["reasoning"] = reasoning

        data = await self._post("/query", payload)
        answer = data.get("answer")
        if not isinstance(answer, str):
            raise MoondreamError("Moondream query returned an invalid response")
        return answer

    async def caption(self, image_base64: str, length: str = "normal") -> str:
        payload = {
            "image_url": _to_data_url(image_base64),
            "length": length,
            "stream": False,
        }
        data = await self._post("/caption", payload)
        caption = data.get("caption")
        if not isinstance(caption, str):
            raise MoondreamError("Moondream caption returned an invalid response")
        return caption

    async def point(self, image_base64: str, obj: str) -> Optional[MoondreamPoint]:
        payload = {
            "image_url": _to_data_url(image_base64),
            "object": obj,
        }
        data = await self._post("/point", payload)
        points = data.get("points")
        if not isinstance(points, list) or not points:
            return None
        point = points[0]
        if not isinstance(point, dict):
            return None
        x = point.get("x")
        y = point.get("y")
        if not isinstance(x, (int, float)) or not isinstance(y, (int, float)):
            return None
        return MoondreamPoint(x=float(x), y=float(y))

    async def detect(self, image_base64: str, obj: str) -> list[dict[str, float]]:
        payload = {
            "image_url": _to_data_url(image_base64),
            "object": obj,
        }
        data = await self._post("/detect", payload)
        objects = data.get("objects")
        if not isinstance(objects, list):
            return []
        results: list[dict[str, float]] = []
        for item in objects:
            if not isinstance(item, dict):
                continue
            x_min = item.get("x_min")
            y_min = item.get("y_min")
            x_max = item.get("x_max")
            y_max = item.get("y_max")
            if all(isinstance(v, (int, float)) for v in (x_min, y_min, x_max, y_max)):
                results.append(
                    {
                        "x_min": float(x_min),
                        "y_min": float(y_min),
                        "x_max": float(x_max),
                        "y_max": float(y_max),
                    }
                )
        return results

    async def _post(self, path: str, payload: dict[str, Any]) -> dict[str, Any]:
        response = await self._client.post(path, json=payload)
        if response.status_code >= 400:
            text = response.text
            raise MoondreamError(f"Moondream API error {response.status_code}: {text}")
        data = response.json()
        if not isinstance(data, dict):
            raise MoondreamError("Moondream API returned unexpected response type")
        return data


def _to_data_url(image_base64: str) -> str:
    # Kernel screenshots are PNG by default
    return f"data:image/png;base64,{image_base64}"


def encode_image_bytes(image_bytes: bytes) -> str:
    return base64.b64encode(image_bytes).decode("utf-8")
