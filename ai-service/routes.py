from collections.abc import AsyncIterator
import logging
import os
import secrets
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
    build_calibrate_messages,
    build_edit_content_messages,
    build_edit_prepublish_messages,
)
from schemas import (
    AdaptedContent,
    CalibrateRequest,
    EditContentRequest,
    EditContentResponse,
    EditPrepublishRequest,
    EditPrepublishResponse,
)

router = APIRouter()
logger = logging.getLogger(__name__)

AI_SERVICE_INTERNAL_TOKEN_ENV = "AI_SERVICE_INTERNAL_TOKEN"
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
    except StopAsyncIteration:
        raise HTTPException(status_code=502, detail="LLM returned empty content")
    except HTTPException:
        raise
    except Exception:
        logger.exception("AI stream failed before first chunk")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL)

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

        return EditContentResponse(
            channel="content", content=edited_content, usage=usage
        )
    except HTTPException:
        raise
    except Exception:
        logger.exception("AI content edit failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL)


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
    except Exception:
        logger.exception("AI content edit stream failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL)


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
    except Exception:
        logger.exception("AI prepublish edit failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL)


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
    except Exception:
        logger.exception("AI prepublish edit stream failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL)


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
    except Exception:
        logger.exception("AI calibration failed")
        raise HTTPException(status_code=502, detail=GENERIC_AI_FAILURE_DETAIL)
