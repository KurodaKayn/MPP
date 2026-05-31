from fastapi import APIRouter, HTTPException
from langchain_core.messages import BaseMessage, HumanMessage, SystemMessage

from llm_client import (
    build_llm,
    conversation_to_messages,
    response_text,
    selected_adapted_text,
)
from schemas import (
    CalibrateRequest,
    EditContentRequest,
    EditContentResponse,
    EditPrepublishRequest,
    EditPrepublishResponse,
)

router = APIRouter()


@router.get("/health")
async def health():
    return {"status": "healthy"}


@router.post("/content/edit", response_model=EditContentResponse)
async def edit_content(request: EditContentRequest):
    if not request.content.strip() or not request.message.strip():
        raise HTTPException(status_code=400, detail="content and message are required")

    try:
        messages: list[BaseMessage] = [
            SystemMessage(
                content=(
                    "You are an editorial assistant for multi-platform posts. "
                    "Rewrite only the supplied content according to the user's latest request. "
                    "Preserve the original language and meaningful formatting. "
                    "If the content is HTML, return valid edited HTML only. "
                    "Do not add explanations, markdown fences, or commentary."
                )
            ),
            *conversation_to_messages(request.conversation),
            HumanMessage(
                content=(
                    f"Title: {request.title}\n\n"
                    f"Current content:\n{request.content}\n\n"
                    f"User request:\n{request.message}\n\n"
                    "Return only the edited content."
                )
            ),
        ]

        response = build_llm().invoke(messages)
        edited_content = response_text(response.content)
        if not edited_content:
            raise HTTPException(status_code=502, detail="LLM returned empty content")

        return EditContentResponse(channel="content", content=edited_content)
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=502, detail=str(e))


@router.post("/prepublish/edit", response_model=EditPrepublishResponse)
async def edit_prepublish(request: EditPrepublishRequest):
    if not request.platform.strip() or not request.message.strip():
        raise HTTPException(status_code=400, detail="platform and message are required")

    content_key, current_text = selected_adapted_text(request.adapted_content)
    if not current_text.strip():
        raise HTTPException(status_code=400, detail="adapted_content text is required")

    try:
        messages: list[BaseMessage] = [
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

        response = build_llm().invoke(messages)
        edited_text = response_text(response.content)
        if not edited_text:
            raise HTTPException(status_code=502, detail="LLM returned empty content")

        adapted_content = dict(request.adapted_content)
        adapted_content[content_key] = edited_text
        if content_key in {"html", "markdown", "text"}:
            adapted_content["format"] = content_key

        return EditPrepublishResponse(
            channel="prepublish",
            platform=request.platform,
            adapted_content=adapted_content,
            content=edited_text,
        )
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=502, detail=str(e))


@router.post("/calibrate")
async def calibrate(request: CalibrateRequest):
    try:
        messages: list[BaseMessage] = [
            SystemMessage(
                content=(
                    "You are an expert social media manager. Calibrate the following "
                    "content for the requested platform rules and style."
                )
            ),
            HumanMessage(content=f"Platform: {request.platform}\n\n{request.content}"),
        ]

        response = build_llm().invoke(messages)

        return {
            "platform": request.platform,
            "calibrated_content": response_text(response.content),
        }
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=502, detail=str(e))
