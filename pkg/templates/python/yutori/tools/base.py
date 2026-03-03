"""Base tool types for Yutori n1."""

from typing import TypedDict


class ToolError(Exception):
    """Error raised when a tool execution fails."""

    def __init__(self, message: str):
        self.message = message
        super().__init__(message)


class ToolResult(TypedDict, total=False):
    base64_image: str
    output: str
    error: str
