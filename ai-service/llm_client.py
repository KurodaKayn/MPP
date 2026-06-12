import os
from collections.abc import Mapping
from typing import Any

from fastapi import HTTPException
from langchain_core.messages import AIMessage, BaseMessage, HumanMessage
from langchain_openai import ChatOpenAI
from pydantic import BaseModel

from schemas import ChatMessage

LLM_PROVIDER_URL_ENV = "LLM_PROVIDER_URL"
LLM_MODEL_ENV = "LLM_MODEL"
LLM_PROVIDER_KEY_ENV = "LLM_PROVIDER_KEY"
LLM_REQUEST_TIMEOUT_ENV = "LLM_REQUEST_TIMEOUT_SECONDS"
LLM_MAX_RETRIES_ENV = "LLM_MAX_RETRIES"
LLM_STREAM_CHUNK_TIMEOUT_ENV = "LLM_STREAM_CHUNK_TIMEOUT_SECONDS"
LLM_INPUT_COST_PER_1K_TOKENS_ENV = "LLM_INPUT_COST_PER_1K_TOKENS"
LLM_OUTPUT_COST_PER_1K_TOKENS_ENV = "LLM_OUTPUT_COST_PER_1K_TOKENS"
LLM_COST_CURRENCY_ENV = "LLM_COST_CURRENCY"

DEFAULT_LLM_REQUEST_TIMEOUT_SECONDS = 90.0
DEFAULT_LLM_MAX_RETRIES = 2
DEFAULT_LLM_STREAM_CHUNK_TIMEOUT_SECONDS = 30.0
DEFAULT_LLM_COST_CURRENCY = "USD"


def build_llm() -> ChatOpenAI:
    provider_url = os.getenv(LLM_PROVIDER_URL_ENV, "").strip()
    model = os.getenv(LLM_MODEL_ENV, "").strip()
    provider_key = os.getenv(LLM_PROVIDER_KEY_ENV, "").strip()

    missing = [
        name
        for name, value in [
            (LLM_PROVIDER_URL_ENV, provider_url),
            (LLM_MODEL_ENV, model),
            (LLM_PROVIDER_KEY_ENV, provider_key),
        ]
        if not value
    ]
    if missing:
        raise HTTPException(
            status_code=503,
            detail=f"LLM provider is not configured: {', '.join(missing)}",
        )

    return ChatOpenAI(
        api_key=provider_key,
        base_url=provider_url,
        model=model,
        streaming=True,
        temperature=0,
        timeout=float_env(LLM_REQUEST_TIMEOUT_ENV, DEFAULT_LLM_REQUEST_TIMEOUT_SECONDS),
        max_retries=int_env(LLM_MAX_RETRIES_ENV, DEFAULT_LLM_MAX_RETRIES),
        stream_chunk_timeout=float_env(
            LLM_STREAM_CHUNK_TIMEOUT_ENV,
            DEFAULT_LLM_STREAM_CHUNK_TIMEOUT_SECONDS,
        ),
    )


def int_env(name: str, default: int) -> int:
    raw = os.getenv(name, "").strip()
    if not raw:
        return default
    try:
        value = int(raw)
    except ValueError:
        return default
    return value if value >= 0 else default


def float_env(name: str, default: float) -> float:
    raw = os.getenv(name, "").strip()
    if not raw:
        return default
    try:
        value = float(raw)
    except ValueError:
        return default
    return value if value > 0 else default


def conversation_to_messages(conversation: list[ChatMessage]) -> list[BaseMessage]:
    messages: list[BaseMessage] = []
    for item in conversation:
        content = item.content.strip()
        if not content:
            continue
        if item.role == "assistant":
            messages.append(AIMessage(content=content))
        else:
            messages.append(HumanMessage(content=content))
    return messages


def response_text(content: Any, *, strip: bool = True) -> str:
    def finish(value: str) -> str:
        return value.strip() if strip else value

    if isinstance(content, str):
        return finish(content)
    if isinstance(content, list):
        parts: list[str] = []
        for item in content:
            if isinstance(item, dict) and isinstance(item.get("text"), str):
                parts.append(item["text"])
            else:
                parts.append(str(item))
        return finish("".join(parts))
    return finish(str(content))


def response_usage(response: Any) -> dict[str, Any]:
    usage = getattr(response, "usage_metadata", None)
    if isinstance(usage, Mapping):
        input_tokens = int_value(usage.get("input_tokens"))
        output_tokens = int_value(usage.get("output_tokens"))
        total_tokens = int_value(usage.get("total_tokens"))
    else:
        metadata = getattr(response, "response_metadata", None)
        token_usage = metadata.get("token_usage") if isinstance(metadata, Mapping) else {}
        if not isinstance(token_usage, Mapping):
            token_usage = {}
        input_tokens = int_value(token_usage.get("prompt_tokens"))
        output_tokens = int_value(token_usage.get("completion_tokens"))
        total_tokens = int_value(token_usage.get("total_tokens"))

    if total_tokens == 0:
        total_tokens = input_tokens + output_tokens

    input_cost = float_env(LLM_INPUT_COST_PER_1K_TOKENS_ENV, 0) * input_tokens / 1000
    output_cost = float_env(LLM_OUTPUT_COST_PER_1K_TOKENS_ENV, 0) * output_tokens / 1000
    currency = os.getenv(LLM_COST_CURRENCY_ENV, DEFAULT_LLM_COST_CURRENCY).strip()
    if not currency:
        currency = DEFAULT_LLM_COST_CURRENCY

    return {
        "input_tokens": input_tokens,
        "output_tokens": output_tokens,
        "total_tokens": total_tokens,
        "cost": round(input_cost + output_cost, 8),
        "currency": currency.upper(),
    }


def int_value(value: Any) -> int:
    try:
        parsed = int(value or 0)
    except (TypeError, ValueError):
        return 0
    return parsed if parsed > 0 else 0


def adapted_content_dict(
    adapted_content: BaseModel | Mapping[str, Any],
) -> dict[str, Any]:
    if isinstance(adapted_content, BaseModel):
        return adapted_content.model_dump(mode="json", exclude_none=True)
    return dict(adapted_content)


def selected_adapted_text(
    adapted_content: BaseModel | Mapping[str, Any],
) -> tuple[str, str]:
    content = adapted_content_dict(adapted_content)
    requested_format = str(content.get("format") or "").strip().lower()
    for key in [requested_format, "html", "markdown", "text", "summary"]:
        if key in {"html", "markdown", "text", "summary"}:
            value = content.get(key)
            if isinstance(value, str) and value.strip():
                return key, value
    return requested_format or "text", ""
