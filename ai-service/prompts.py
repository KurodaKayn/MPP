from langchain_core.messages import BaseMessage, HumanMessage, SystemMessage

from llm_client import conversation_to_messages
from schemas import (
    CalibrateRequest,
    EditContentRequest,
    EditPrepublishRequest,
    GrowthOptimizationRequest,
)


GROWTH_AUDIENCE_PROFILES = {
    "wechat": {
        "profile_id": "wechat@growth-v1",
        "audience_summary": (
            "Readers expect credible long-form value, clear framing, and a strong "
            "opening retention path."
        ),
        "ranking_signals": [
            "opening retention",
            "read completion",
            "shareability",
            "account trust",
        ],
        "content_guidance": [
            "lead with a concrete problem",
            "use scannable sections",
            "avoid exaggerated traffic promises",
        ],
        "risk_warnings": ["Do not promise guaranteed platform traffic or recommendation lift."],
    },
    "zhihu": {
        "profile_id": "zhihu@growth-v1",
        "audience_summary": (
            "Readers reward expertise, balanced reasoning, evidence, and practical examples."
        ),
        "ranking_signals": ["answer credibility", "depth", "save rate", "comment quality"],
        "content_guidance": [
            "state the thesis early",
            "add evidence-oriented structure",
            "avoid thin listicles",
        ],
        "risk_warnings": ["Check unsupported claims before publishing."],
    },
    "x": {
        "profile_id": "x@growth-v1",
        "audience_summary": (
            "Readers scan quickly and respond to concise hooks, clear opinions, "
            "and conversational CTAs."
        ),
        "ranking_signals": ["hook clarity", "reply potential", "repost value", "length discipline"],
        "content_guidance": [
            "compress the lead",
            "make one point per paragraph",
            "end with a low-friction CTA",
        ],
        "risk_warnings": ["Avoid engagement bait that misrepresents the article."],
    },
    "douyin": {
        "profile_id": "douyin@growth-v1",
        "audience_summary": (
            "Viewers respond to fast pacing, image-text rhythm, curiosity gaps, "
            "and concrete takeaways."
        ),
        "ranking_signals": [
            "first-screen hook",
            "completion potential",
            "interaction prompt",
            "visual pacing",
        ],
        "content_guidance": [
            "front-load the conflict",
            "split ideas into short beats",
            "make the CTA visual and specific",
        ],
        "risk_warnings": ["Avoid absolute performance claims."],
    },
}


def build_edit_content_messages(request: EditContentRequest) -> list[BaseMessage]:
    return [
        SystemMessage(
            content=(
                "You are an editorial assistant for multi-platform posts. "
                "Create or rewrite content according to the user's latest request. "
                "Preserve the original language and meaningful formatting. "
                "If current content is provided, edit it instead of starting over unless asked. "
                "If current content is empty, draft new content from the user request. "
                "If the content is HTML, return valid edited HTML only. "
                "Do not add explanations, markdown fences, or commentary."
            )
        ),
        *conversation_to_messages(request.conversation),
        HumanMessage(
            content=(
                f"Title: {request.title}\n\n"
                f"Current content:\n{request.content or '(empty)'}\n\n"
                f"User request:\n{request.message}\n\n"
                "Return only the generated or edited content."
            )
        ),
    ]


def build_edit_prepublish_messages(
    request: EditPrepublishRequest,
    content_key: str,
    current_text: str,
) -> list[BaseMessage]:
    return [
        SystemMessage(
            content=(
                "You are an assistant editing platform-specific prepublish drafts. "
                "Rewrite only the draft text according to the user's latest request. "
                "Respect the target platform, keep the same output format, and avoid explanations. "
                "For HTML return valid HTML only; for markdown return markdown only; "
                "for plain text return plain text only."
            )
        ),
        *conversation_to_messages(request.conversation),
        HumanMessage(
            content=(
                f"Platform: {request.platform}\n"
                f"Title: {request.title}\n"
                f"Format: {content_key}\n\n"
                f"Current draft:\n{current_text}\n\n"
                f"User request:\n{request.message}\n\n"
                "Return only the edited draft."
            )
        ),
    ]


def build_calibrate_messages(request: CalibrateRequest) -> list[BaseMessage]:
    return [
        SystemMessage(
            content=(
                "You are an expert social media manager. Calibrate the following "
                "content for the requested platform rules and style."
            )
        ),
        HumanMessage(content=f"Platform: {request.platform}\n\n{request.content}"),
    ]


def audience_profiles_for(platforms: list[str]) -> list[dict]:
    return [
        GROWTH_AUDIENCE_PROFILES[platform]
        for platform in platforms
        if platform in GROWTH_AUDIENCE_PROFILES
    ]


def build_growth_optimization_messages(
    request: GrowthOptimizationRequest,
) -> list[BaseMessage]:
    profiles = request.audience_profiles or audience_profiles_for(
        [str(platform) for platform in request.target_platforms]
    )
    drafts = [
        draft.model_dump(mode="json", exclude_none=True)
        for draft in request.platform_drafts
    ]
    return [
        SystemMessage(
            content=(
                "You are a growth editor for multi-platform content. "
                "Create reviewable proposal artifacts only; never claim guaranteed "
                "traffic, views, or recommendation lift. Return strict JSON with "
                "keys model, prompt_version, quality_summary, proposals, and usage. "
                "Each proposal must include proposal_type, target_platform, summary, "
                "patch, full_content, and quality_checks. Emit at least one "
                "title_candidates proposal, one source_rewrite proposal, and one "
                "prepublish_patch proposal per target platform."
            )
        ),
        HumanMessage(
            content=(
                f"Title: {request.title}\n"
                f"Goal: {request.goal}\n"
                f"Intensity: {request.intensity}\n"
                "Target platforms: "
                f"{', '.join(str(platform) for platform in request.target_platforms)}\n\n"
                f"Audience profiles:\n{profiles}\n\n"
                f"Brand profile:\n{request.brand_profile or {}}\n\n"
                f"Existing platform drafts:\n{drafts}\n\n"
                f"Source content:\n{request.source_content}\n\n"
                "Return JSON only."
            )
        ),
    ]
