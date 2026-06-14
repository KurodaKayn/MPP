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
            "Readers expect uplifting long-form value, clear emotional framing, "
            "and a strong opening retention path."
        ),
        "ranking_signals": [
            "opening retention",
            "read completion",
            "shareability",
            "account trust",
        ],
        "content_guidance": [
            "make the title more dramatic than the source title while staying tied to facts",
            "lead with a concrete problem and a hopeful direction",
            "rewrite the body with positive, constructive, and confidence-building language",
            "use scannable sections and emotionally clear transitions",
        ],
        "title_strategy": (
            "Use a bigger, curiosity-driven WeChat title. It may be heightened and "
            "more dramatic, but it must not invent facts."
        ),
        "body_strategy": (
            "Turn the article into an optimistic narrative: identify pressure, show "
            "progress, explain practical value, and end with forward-looking energy."
        ),
        "comparison_strategy": (
            "Use comparisons only when the source provides enough basis; otherwise "
            "frame them as qualitative differences to verify."
        ),
        "engagement_strategy": (
            "Invite readers to reflect, save, or share with a constructive CTA."
        ),
        "risk_warnings": ["Do not promise guaranteed platform traffic or recommendation lift."],
    },
    "zhihu": {
        "profile_id": "zhihu@growth-v1",
        "audience_summary": (
            "Readers reward expertise, balanced reasoning, evidence, and practical examples."
        ),
        "ranking_signals": ["answer credibility", "depth", "save rate", "comment quality"],
        "content_guidance": [
            "make the title specific to a technology, result, person, institution, or claim",
            "state the thesis early and explain why it matters",
            "quantify technical or result claims when the source provides numbers",
            "compare with alternatives or foreign peers when there is enough evidence",
            "avoid thin listicles and unsupported certainty",
        ],
        "title_strategy": (
            "Use a clear Zhihu-style explanatory title. For technical subjects, name "
            "the technology or achievement directly, for example: DeepSeek founder "
            "commented on X, broke through Y bottleneck, and achieved Z."
        ),
        "body_strategy": (
            "Polish the article into rigorous analysis: define the issue, explain the "
            "technical path, add evidence, quantify results where possible, and separate "
            "facts from interpretation."
        ),
        "comparison_strategy": (
            "When the source supports it, compare against competing models, companies, "
            "or international baselines. Mark missing data as a verification need."
        ),
        "engagement_strategy": (
            "Close with a reasoned question that invites technical discussion rather "
            "than emotional argument."
        ),
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
            "make the title plain and literal",
            "tell readers exactly what happened in simple language",
            "use short sentences and avoid decorative phrasing",
            "make one point per paragraph",
            "end with a low-friction factual CTA",
        ],
        "title_strategy": (
            "Use a plain X title that explicitly says what the article is about. "
            "Do not hide the topic or over-polish the wording."
        ),
        "body_strategy": (
            "Use the simplest possible language: what happened, why it matters, what "
            "changed, and what to watch next."
        ),
        "comparison_strategy": (
            "Keep comparisons direct and short. Avoid grand claims unless the source "
            "contains clear evidence."
        ),
        "engagement_strategy": (
            "Ask a simple reply question or invite readers to add missing context."
        ),
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
            "make the title indirect, metaphorical, pun-like, or double-layered",
            "hint at the topic without fully explaining it",
            "front-load contrast and conflict",
            "use vivid elevated language while staying connected to source facts",
            "add comparison points that invite disagreement and comments",
            "split ideas into short beats",
        ],
        "title_strategy": (
            "Use an indirect Douyin title with metaphor, pun, or double meaning. It "
            "should quietly reveal the subject and create curiosity."
        ),
        "body_strategy": (
            "Make the body vivid and high-retention: stronger imagery, sharper contrast, "
            "more dramatic pacing, and comment-worthy comparisons."
        ),
        "comparison_strategy": (
            "Compare with other players or familiar situations in a memorable way. "
            "Avoid fabricated data, but make qualitative contrasts feel vivid."
        ),
        "engagement_strategy": (
            "End with a comment-area hook that asks readers to choose sides or name a "
            "counterexample."
        ),
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
    drafts = [draft.model_dump(mode="json", exclude_none=True) for draft in request.platform_drafts]
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
                "Platform execution rules:\n"
                "- Every prepublish_patch must follow its target platform profile.\n"
                "- The summary must name the profile_id and the main platform tactic used.\n"
                "- The quality_checks object must include audience_profile, title_strategy, "
                "body_strategy, comparison_strategy, and risk_warnings.\n"
                "- If the source lacks evidence for a comparison or number, propose a "
                "verification warning instead of inventing data.\n\n"
                f"Brand profile:\n{request.brand_profile or {}}\n\n"
                f"Existing platform drafts:\n{drafts}\n\n"
                f"Source content:\n{request.source_content}\n\n"
                "Return JSON only."
            )
        ),
    ]
