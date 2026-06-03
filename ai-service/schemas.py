from typing import Literal

from pydantic import BaseModel, Field

from contract_schemas import AdaptedContent, PublishPlatform


class ChatMessage(BaseModel):
    role: Literal["user", "assistant"]
    content: str


class EditContentRequest(BaseModel):
    content: str
    message: str
    title: str = ""
    conversation: list[ChatMessage] = Field(default_factory=list)


class EditContentResponse(BaseModel):
    channel: str
    content: str


class EditPrepublishRequest(BaseModel):
    adapted_content: AdaptedContent
    message: str
    platform: PublishPlatform
    title: str = ""
    conversation: list[ChatMessage] = Field(default_factory=list)


class EditPrepublishResponse(BaseModel):
    channel: str
    platform: PublishPlatform
    adapted_content: AdaptedContent
    content: str


class CalibrateRequest(BaseModel):
    content: str
    platform: PublishPlatform
