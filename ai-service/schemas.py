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


class AIUsage(BaseModel):
    input_tokens: int = 0
    output_tokens: int = 0
    total_tokens: int = 0
    cost: float = 0
    currency: str = "USD"


class EditContentResponse(BaseModel):
    channel: str
    content: str
    usage: AIUsage = Field(default_factory=AIUsage)


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
    usage: AIUsage = Field(default_factory=AIUsage)


class CalibrateRequest(BaseModel):
    content: str
    platform: PublishPlatform
