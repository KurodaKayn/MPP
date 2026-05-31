from langchain_core.messages import BaseMessage, HumanMessage, SystemMessage

from llm_client import conversation_to_messages
from schemas import CalibrateRequest, EditContentRequest, EditPrepublishRequest


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
