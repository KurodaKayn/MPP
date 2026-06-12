import pytest
from fastapi import HTTPException
from langchain_core.messages import AIMessage, HumanMessage

from llm_client import (
    DEFAULT_LLM_MAX_RETRIES,
    DEFAULT_LLM_REQUEST_TIMEOUT_SECONDS,
    DEFAULT_LLM_STREAM_CHUNK_TIMEOUT_SECONDS,
    LLM_MAX_RETRIES_ENV,
    LLM_MODEL_ENV,
    LLM_PROVIDER_KEY_ENV,
    LLM_PROVIDER_URL_ENV,
    LLM_REQUEST_TIMEOUT_ENV,
    LLM_STREAM_CHUNK_TIMEOUT_ENV,
    build_llm,
    conversation_to_messages,
    float_env,
    int_env,
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


def test_build_llm_applies_resilience_options(monkeypatch):
    monkeypatch.setenv(LLM_PROVIDER_URL_ENV, "https://llm.example.test/v1")
    monkeypatch.setenv(LLM_MODEL_ENV, "test-model")
    monkeypatch.setenv(LLM_PROVIDER_KEY_ENV, "test-key")
    monkeypatch.setenv(LLM_REQUEST_TIMEOUT_ENV, "12.5")
    monkeypatch.setenv(LLM_MAX_RETRIES_ENV, "4")
    monkeypatch.setenv(LLM_STREAM_CHUNK_TIMEOUT_ENV, "6")

    llm = build_llm()

    assert llm.request_timeout == 12.5
    assert llm.max_retries == 4
    assert llm.stream_chunk_timeout == 6.0


def test_resilience_env_helpers_fallback_to_defaults(monkeypatch):
    monkeypatch.setenv(LLM_REQUEST_TIMEOUT_ENV, "-1")
    monkeypatch.setenv(LLM_MAX_RETRIES_ENV, "-1")
    monkeypatch.setenv(LLM_STREAM_CHUNK_TIMEOUT_ENV, "not-a-number")

    assert (
        float_env(LLM_REQUEST_TIMEOUT_ENV, DEFAULT_LLM_REQUEST_TIMEOUT_SECONDS)
        == DEFAULT_LLM_REQUEST_TIMEOUT_SECONDS
    )
    assert int_env(LLM_MAX_RETRIES_ENV, DEFAULT_LLM_MAX_RETRIES) == DEFAULT_LLM_MAX_RETRIES
    assert (
        float_env(LLM_STREAM_CHUNK_TIMEOUT_ENV, DEFAULT_LLM_STREAM_CHUNK_TIMEOUT_SECONDS)
        == DEFAULT_LLM_STREAM_CHUNK_TIMEOUT_SECONDS
    )


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
