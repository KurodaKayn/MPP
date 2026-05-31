import pytest
from fastapi import HTTPException
from langchain_core.messages import AIMessage, HumanMessage

from llm_client import (
    LLM_MODEL_ENV,
    LLM_PROVIDER_KEY_ENV,
    LLM_PROVIDER_URL_ENV,
    build_llm,
    conversation_to_messages,
    response_text,
    selected_adapted_text,
)
from schemas import ChatMessage


def test_build_llm_reports_missing_configuration(monkeypatch):
    for name in [LLM_PROVIDER_URL_ENV, LLM_MODEL_ENV, LLM_PROVIDER_KEY_ENV]:
        monkeypatch.delenv(name, raising=False)

    with pytest.raises(HTTPException) as exc_info:
        build_llm()

    assert exc_info.value.status_code == 503
    assert LLM_PROVIDER_URL_ENV in exc_info.value.detail
    assert LLM_MODEL_ENV in exc_info.value.detail
    assert LLM_PROVIDER_KEY_ENV in exc_info.value.detail


def test_conversation_to_messages_trims_and_filters_empty_messages():
    messages = conversation_to_messages(
        [
            ChatMessage(role="user", content="  hello  "),
            ChatMessage(role="assistant", content="  hi there  "),
            ChatMessage(role="user", content="   "),
        ]
    )

    assert len(messages) == 2
    assert isinstance(messages[0], HumanMessage)
    assert messages[0].content == "hello"
    assert isinstance(messages[1], AIMessage)
    assert messages[1].content == "hi there"


def test_response_text_normalizes_string_and_list_content():
    assert response_text("  edited content  ") == "edited content"
    assert response_text("  keep spacing  ", strip=False) == "  keep spacing  "
    assert response_text([{"text": "hello"}, {"text": " world"}]) == "hello world"


def test_selected_adapted_text_prefers_requested_format_then_fallbacks():
    assert selected_adapted_text(
        {
            "format": "markdown",
            "markdown": "  **Markdown**  ",
            "html": "<p>HTML</p>",
        }
    ) == ("markdown", "  **Markdown**  ")

    assert selected_adapted_text(
        {
            "format": "unknown",
            "html": "<p>HTML</p>",
            "text": "Plain text",
        }
    ) == ("html", "<p>HTML</p>")

    assert selected_adapted_text({"format": "markdown"}) == ("markdown", "")
