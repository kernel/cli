"""
Custom actions for Agent Auth (login discovery).

Provides the FoundInputsAction for reporting discovered login form fields.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, ClassVar, Literal

from core.actions import Action


FieldType = Literal["text", "email", "password", "tel", "number", "url", "code"]


@dataclass
class FoundField:
    """A discovered form field."""

    name: str
    type: FieldType
    placeholder: str = ""
    required: bool = True


@dataclass
class FoundInputsAction(Action):
    """
    Report discovered input fields.

    Used by login discovery agents to indicate they found a login form
    and are reporting what inputs were discovered.
    """

    fields: list[FoundField] = field(default_factory=list)

    action_type: ClassVar[str] = "found_inputs"
    description: ClassVar[str] = (
        "Report discovered input fields from a form (for login discovery). "
        "Use when you've found a form and want to report the discovered fields. "
        "This action completes the task."
    )
    is_terminal: ClassVar[bool] = True
    parameters: ClassVar[dict[str, dict[str, Any]]] = {
        "fields": {
            "type": "array",
            "description": "List of discovered form fields.",
            "items": {
                "type": "object",
                "description": "A discovered form field",
                "properties": {
                    "name": {
                        "type": "string",
                        "description": "Field name",
                        "example": "email",
                    },
                    "type": {
                        "type": "string",
                        "enum": ["text", "email", "password", "tel", "number", "url", "code"],
                        "description": "Field type",
                        "example": "email",
                    },
                    "placeholder": {
                        "type": "string",
                        "description": "Field placeholder",
                        "example": "you@example.com",
                    },
                    "required": {
                        "type": "boolean",
                        "description": "Whether field is required",
                        "default": True,
                        "example": True,
                    },
                },
                "required": ["name", "type", "required"],
            },
            "required": True,
        }
    }

    @classmethod
    def parse_args(cls, args: dict) -> "FoundInputsAction | None":
        fields_data = args.get("fields", [])
        fields = [
            FoundField(
                name=f.get("name", ""),
                type=f.get("type", "text"),
                placeholder=f.get("placeholder", ""),
                required=f.get("required", True),
            )
            for f in fields_data
        ]
        return cls(fields=fields)

    def to_description(self) -> str:
        field_names = [f.name for f in self.fields]
        return f"Found inputs: {', '.join(field_names)}"

    def to_tool_args(self) -> dict:
        return {
            "action": self.action_type,
            "fields": [
                {
                    "name": f.name,
                    "type": f.type,
                    "placeholder": f.placeholder,
                    "required": f.required,
                }
                for f in self.fields
            ],
        }


# List of custom actions for Agent Auth
AGENT_AUTH_ACTIONS: list[type[Action]] = [FoundInputsAction]
