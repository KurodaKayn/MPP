import json
import logging
import os
import secrets
from collections.abc import AsyncIterator
from typing import Annotated

from fastapi import APIRouter, Depends, Header, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import BaseMessage

from llm_client import (
    adapted_content_dict,
    build_llm,
    response_text,
    response_usage,
    selected_adapted_text,
)
from observability import record_ai_usage
from prompts import (
    audience_profiles_for,
    build_calibrate_messages,
    build_edit_content_messages,
    build_edit_prepublish_messages,
    build_growth_optimization_messages,
)
from schemas import (
    AdaptedContent,
    CalibrateRequest,
    EditContentRequest,
    EditContentResponse,
    EditPrepublishRequest,
    EditPrepublishResponse,
    GrowthOptimizationRequest,
    GrowthOptimizationResponse,
    GrowthProposal,
)

router = APIRouter()
logger = logging.getLogger(__name__)

AI_SERVICE_INTERNAL_TOKEN_ENV = "AI_SERVICE_INTERNAL_TOKEN"  # noqa: S105
GENERIC_AI_FAILURE_DETAIL = "AI service request failed"


async def require_internal_bearer_token(
    authorization: Annotated[str | None, Header()] = None,
) -> None:
    expected_token = os.getenv(AI_SERVICE_INTERNAL_TOKEN_ENV, "").strip()
    if not expected_token:
        raise HTTPException(
            status_code=503,
            detail="AI service internal token is not configured",
        )

    scheme, _, token = (authorization or "").partition(" ")
    if scheme.lower() != "bearer" or not secrets.compare_digest(
        token,
        expected_token,
    ):
        raise HTTPException(status_code=401, detail="unauthorized")


internal_auth = [Depends(require_internal_bearer_token)]


async def stream_response_text(
    llm: BaseChatModel,
    messages: list[BaseMessage],
) -> AsyncIterator[str]:
    async for chunk in llm.astream(messages):
        text = response_text(chunk.content, strip=False)
        if text:
            yield text


async def stream_markdown_response(
    llm: BaseChatModel,
    messages: list[BaseMessage],
) -> StreamingResponse:
    stream = stream_response_text(llm, messages)

    try:
        first_chunk = await anext(stream)
    except StopAsyncIteration as e:
        raise HTTPException(status_code=502, detail="LLM returned empty content") from e
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("AI stream failed before first chunk")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL) from e

    async def with_first_chunk() -> AsyncIterator[str]:
        yield first_chunk
        async for chunk in stream:
            yield chunk

    return StreamingResponse(
        with_first_chunk(),
        media_type="text/markdown; charset=utf-8",
    )


@router.get("/health")
async def health():
    return {"status": "healthy"}


@router.get("/ready")
async def ready(request: Request):
    if not getattr(request.app.state, "ready", False):
        return JSONResponse(status_code=503, content={"status": "not_ready"})
    return {"status": "ready"}


@router.post(
    "/content/edit",
    response_model=EditContentResponse,
    dependencies=internal_auth,
)
async def edit_content(request: EditContentRequest):
    if not request.message.strip():
        raise HTTPException(status_code=400, detail="message is required")

    try:
        response = await build_llm().ainvoke(build_edit_content_messages(request))
        edited_content = response_text(response.content)
        if not edited_content:
            raise HTTPException(status_code=502, detail="LLM returned empty content")
        usage = response_usage(response)
        record_ai_usage("/content/edit", usage)

        return EditContentResponse(channel="content", content=edited_content, usage=usage)
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("AI content edit failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL) from e


@router.post("/content/edit/stream", dependencies=internal_auth)
async def stream_edit_content(request: EditContentRequest):
    if not request.message.strip():
        raise HTTPException(status_code=400, detail="message is required")

    try:
        llm = build_llm()
        messages = build_edit_content_messages(request)
        return await stream_markdown_response(llm, messages)
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("AI content edit stream failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL) from e


@router.post(
    "/prepublish/edit",
    response_model=EditPrepublishResponse,
    dependencies=internal_auth,
)
async def edit_prepublish(request: EditPrepublishRequest):
    if not request.platform.strip() or not request.message.strip():
        raise HTTPException(status_code=400, detail="platform and message are required")

    content_key, current_text = selected_adapted_text(request.adapted_content)
    if not current_text.strip():
        raise HTTPException(status_code=400, detail="adapted_content text is required")

    try:
        response = await build_llm().ainvoke(
            build_edit_prepublish_messages(request, content_key, current_text)
        )
        edited_text = response_text(response.content)
        if not edited_text:
            raise HTTPException(status_code=502, detail="LLM returned empty content")
        usage = response_usage(response)
        record_ai_usage("/prepublish/edit", usage)

        adapted_content = adapted_content_dict(request.adapted_content)
        adapted_content[content_key] = edited_text
        if content_key in {"html", "markdown", "text"}:
            adapted_content["format"] = content_key

        return EditPrepublishResponse(
            channel="prepublish",
            platform=request.platform,
            adapted_content=AdaptedContent.model_validate(adapted_content),
            content=edited_text,
            usage=usage,
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("AI prepublish edit failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL) from e


@router.post("/prepublish/edit/stream", dependencies=internal_auth)
async def stream_edit_prepublish(request: EditPrepublishRequest):
    if not request.platform.strip() or not request.message.strip():
        raise HTTPException(status_code=400, detail="platform and message are required")

    content_key, current_text = selected_adapted_text(request.adapted_content)
    if not current_text.strip():
        raise HTTPException(status_code=400, detail="adapted_content text is required")

    try:
        llm = build_llm()
        messages = build_edit_prepublish_messages(request, content_key, current_text)
        return await stream_markdown_response(llm, messages)
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("AI prepublish edit stream failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL) from e


@router.post("/calibrate", dependencies=internal_auth)
async def calibrate(request: CalibrateRequest):
    try:
        response = await build_llm().ainvoke(build_calibrate_messages(request))

        return {
            "platform": request.platform,
            "calibrated_content": response_text(response.content),
        }
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("AI calibration failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL) from e


@router.post("/growth/optimize/stream", dependencies=internal_auth)
async def stream_growth_optimization(request: GrowthOptimizationRequest):
    if not request.source_content.strip() or not request.goal.strip():
        raise HTTPException(
            status_code=400,
            detail="source_content and goal are required",
        )
    if not request.target_platforms:
        raise HTTPException(status_code=400, detail="target_platforms is required")

    if not request.audience_profiles:
        request.audience_profiles = audience_profiles_for(
            [str(platform) for platform in request.target_platforms]
        )

    try:
        response = await build_llm().ainvoke(build_growth_optimization_messages(request))
        usage = response_usage(response)
        result = parse_growth_response(response_text(response.content), request, usage)
        record_ai_usage("/growth/optimize/stream", usage)
        return StreamingResponse(
            growth_event_stream(result),
            media_type="text/event-stream; charset=utf-8",
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("AI growth optimization stream failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL) from e


def parse_growth_response(
    raw_content: str,
    request: GrowthOptimizationRequest,
    usage: dict,
) -> GrowthOptimizationResponse:
    try:
        parsed = json.loads(raw_content)
        response = GrowthOptimizationResponse.model_validate(parsed)
        response.usage = usage
        return response
    except Exception:
        return GrowthOptimizationResponse(
            quality_summary=(
                "Generated fallback growth proposals. Review claims and platform fit "
                "before applying."
            ),
            proposals=fallback_growth_proposals(request),
            usage=usage,
        )


def fallback_growth_proposals(
    request: GrowthOptimizationRequest,
) -> list[GrowthProposal]:
    warnings = {
        "risk_warnings": [
            "Review quantitative claims before publishing.",
            "Growth optimization should not promise guaranteed platform traffic.",
        ]
    }
    title = request.title.strip() or "Untitled"
    proposals = [
        GrowthProposal(
            proposal_type="title_candidates",
            target_platform="source",
            summary="Growth-oriented title candidates",
            full_content="\n".join(
                [
                    f"{title}: practical takeaways",
                    f"Why {title} matters now",
                    f"{title} without the common mistakes",
                ]
            ),
            quality_checks=warnings,
        ),
        GrowthProposal(
            proposal_type="source_rewrite",
            target_platform="source",
            summary="Source rewrite optimized for clearer opening retention",
            patch="",
            full_content=(
                f"{title}\n\n"
                f"Goal: {request.goal}\n\n"
                f"{request.source_content.strip()}"
            ),
            quality_checks=warnings,
        ),
    ]
    for platform in request.target_platforms:
        platform_key = str(platform)
        profile_id = f"{platform_key}@growth-v1"
        platform_plan = fallback_platform_plan(platform_key, title, request.source_content)
        proposals.append(
            GrowthProposal(
                proposal_type="prepublish_patch",
                target_platform=platform_key,
                summary=platform_plan["summary"],
                patch="",
                full_content=platform_plan["content"],
                quality_checks=warnings
                | {
                    "audience_profile": profile_id,
                    "title_strategy": platform_plan["title_strategy"],
                    "body_strategy": platform_plan["body_strategy"],
                    "comparison_strategy": platform_plan["comparison_strategy"],
                },
            )
        )
    return proposals


def fallback_platform_plan(platform: str, title: str, source_content: str) -> dict[str, str]:
    source = source_content.strip()
    plans = {
        "wechat": {
            "summary": (
                "wechat@growth-v1 platform draft proposal using a heightened positive "
                "narrative."
            ),
            "title_strategy": "Dramatic but fact-bound WeChat title.",
            "body_strategy": "Positive, constructive, confidence-building long-form rewrite.",
            "comparison_strategy": "Use only source-supported comparisons or mark them to verify.",
            "content": (
                f"{title}: this change may be bigger than it first appears\n\n"
                "Start with the pressure readers feel, then turn it toward progress.\n\n"
                f"{source}\n\n"
                "Close with a constructive takeaway and a forward-looking CTA."
            ),
        },
        "zhihu": {
            "summary": (
                "zhihu@growth-v1 platform draft proposal using specific technical "
                "framing and evidence checks."
            ),
            "title_strategy": "Specific technical title naming the claim, bottleneck, or result.",
            "body_strategy": "Rigorous analysis with thesis, mechanism, evidence, and conclusion.",
            "comparison_strategy": "Quantify and compare only when the source supports it.",
            "content": (
                f"{title}: the specific technical result, constraint, and evidence to verify\n\n"
                "Thesis: state what changed and why it matters.\n\n"
                f"{source}\n\n"
                "Evidence: add numbers, benchmarks, or peer comparisons only where "
                "the source supports them; otherwise list verification needs."
            ),
        },
        "douyin": {
            "summary": (
                "douyin@growth-v1 platform draft proposal using metaphor, contrast, "
                "and comment hooks."
            ),
            "title_strategy": "Indirect metaphor or double-meaning title that hints at the topic.",
            "body_strategy": "Vivid, high-retention rewrite with sharper contrast and pacing.",
            "comparison_strategy": "Use memorable qualitative contrasts without fabricating data.",
            "content": (
                f"When the quiet door opens, everyone hears the hinge\n\n"
                "Hint at the topic first, then reveal the conflict in short beats.\n\n"
                f"{source}\n\n"
                "Add a vivid comparison and end by asking readers which side they "
                "think has the real advantage."
            ),
        },
        "x": {
            "summary": "x@growth-v1 platform draft proposal using plain factual wording.",
            "title_strategy": "Plain title that directly states the article topic.",
            "body_strategy": "Simple language: what happened, why it matters, what is next.",
            "comparison_strategy": "Keep comparisons short, direct, and evidence-bound.",
            "content": (
                f"{title}\n\n"
                "What happened:\n"
                f"{source}\n\n"
                "Why it matters: explain the concrete change in plain language.\n"
                "What to watch next: name the next fact or signal readers should check."
            ),
        },
    }
    return plans.get(
        platform,
        {
            "summary": f"{platform}@growth-v1 platform draft proposal",
            "title_strategy": "Use a clear platform-specific title.",
            "body_strategy": "Rewrite for platform fit while preserving source facts.",
            "comparison_strategy": "Avoid unsupported comparisons.",
            "content": f"{title}\n\n{source}",
        },
    )


async def growth_event_stream(
    result: GrowthOptimizationResponse,
) -> AsyncIterator[str]:
    yield sse("status", {"status": "running", "prompt_version": result.prompt_version})
    for proposal in result.proposals:
        yield sse("proposal", proposal.model_dump(mode="json"))
    yield sse(
        "status",
        {
            "status": "ready",
            "model": result.model,
            "prompt_version": result.prompt_version,
            "quality_summary": result.quality_summary,
            "usage": result.usage.model_dump(mode="json")
            if hasattr(result.usage, "model_dump")
            else result.usage,
        },
    )


def sse(event: str, data: dict) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"
