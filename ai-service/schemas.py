from typing import Any, Literal

from pydantic import BaseModel


class ChatMessage(BaseModel):
    role: Literal["user", "assistant"]
    content: str


class EditContentRequest(BaseModel):
    content: str
    message: str
    title: str = ""
    conversation: list[ChatMessage] = []


class EditContentResponse(BaseModel):
    channel: str
    content: str


class EditPrepublishRequest(BaseModel):
    adapted_content: dict[str, Any]
    message: str
    platform: str
    title: str = ""
    conversation: list[ChatMessage] = []


class EditPrepublishResponse(BaseModel):
    channel: str
    platform: str
    adapted_content: dict[str, Any]
    content: str


class CalibrateRequest(BaseModel):
    content: str
    platform: str
