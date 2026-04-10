"""Base tool types for Tzafon Northstar."""


class ToolError(Exception):
    def __init__(self, message: str):
        self.message = message
        super().__init__(message)
