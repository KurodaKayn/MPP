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


class GrowthPlatformDraft(BaseModel):
    platform: PublishPlatform
    adapted_content: AdaptedContent | None = None


class GrowthAudienceProfile(BaseModel):
    profile_id: str
    platform: PublishPlatform
    audience_summary: str
    ranking_signals: list[str] = Field(default_factory=list)
    content_guidance: list[str] = Field(default_factory=list)
    risk_warnings: list[str] = Field(default_factory=list)


class GrowthOptimizationRequest(BaseModel):
    title: str = ""
    source_content: str
    goal: str
    intensity: Literal["conservative", "balanced", "aggressive"] = "balanced"
    target_platforms: list[PublishPlatform]
    platform_drafts: list[GrowthPlatformDraft] = Field(default_factory=list)
    brand_profile: dict | None = None
    audience_profiles: list[GrowthAudienceProfile] = Field(default_factory=list)


class GrowthProposal(BaseModel):
    proposal_type: Literal["title_candidates", "source_rewrite", "prepublish_patch"]
    target_platform: str
    summary: str
    patch: str = ""
    full_content: str
    quality_checks: dict = Field(default_factory=dict)


class GrowthOptimizationResponse(BaseModel):
    model: str = "growth-optimizer"
    prompt_version: str = "growth-v1"
    quality_summary: str = ""
    proposals: list[GrowthProposal]
    usage: AIUsage = Field(default_factory=AIUsage)
