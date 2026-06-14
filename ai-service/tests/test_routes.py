import logging
from types import SimpleNamespace

import pytest
from fastapi.testclient import TestClient

import routes
from main import app


class FakeLLM:
    def __init__(self, *, invoke_content="", stream_contents=None, usage_metadata=None):
        self.invoke_content = invoke_content
        self.stream_contents = stream_contents or []
        self.usage_metadata = usage_metadata or {}
        self.messages = None

    async def ainvoke(self, messages):
        self.messages = messages
        return SimpleNamespace(
            content=self.invoke_content,
            usage_metadata=self.usage_metadata,
        )

    async def astream(self, messages):
        self.messages = messages
        for content in self.stream_contents:
            yield SimpleNamespace(content=content)


client = TestClient(app)
INTERNAL_TOKEN = "test-ai-service-token"  # noqa: S105


@pytest.fixture(autouse=True)
def ai_internal_token(monkeypatch):
    monkeypatch.setenv(routes.AI_SERVICE_INTERNAL_TOKEN_ENV, INTERNAL_TOKEN)


def auth_headers():
    return {"Authorization": f"Bearer {INTERNAL_TOKEN}"}


def test_health_returns_status():
    response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"status": "healthy"}


def test_ready_returns_status_during_lifespan():
    with TestClient(app) as lifespan_client:
        response = lifespan_client.get("/ready")

    assert response.status_code == 200
    assert response.json() == {"status": "ready"}


def test_edit_content_returns_llm_content(monkeypatch):
    fake_llm = FakeLLM(
        invoke_content="  edited copy  ",
        usage_metadata={"input_tokens": 12, "output_tokens": 3, "total_tokens": 15},
    )
    monkeypatch.setattr(routes, "build_llm", lambda: fake_llm)
    monkeypatch.setenv("LLM_INPUT_COST_PER_1K_TOKENS", "0.01")
    monkeypatch.setenv("LLM_OUTPUT_COST_PER_1K_TOKENS", "0.02")

    response = client.post(
        "/content/edit",
        headers=auth_headers(),
        json={
            "content": "Original content",
            "message": "Make it shorter",
            "title": "Launch note",
            "conversation": [{"role": "assistant", "content": "Previous advice"}],
        },
    )

    assert response.status_code == 200
    assert response.json() == {
        "channel": "content",
        "content": "edited copy",
        "usage": {
            "input_tokens": 12,
            "output_tokens": 3,
            "total_tokens": 15,
            "cost": 0.00018,
            "currency": "USD",
        },
    }
    assert fake_llm.messages is not None
    assert "Make it shorter" in fake_llm.messages[-1].content


def test_edit_content_rejects_blank_message():
    response = client.post(
        "/content/edit",
        headers=auth_headers(),
        json={"content": "Original content", "message": "   "},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "message is required"


def test_edit_prepublish_updates_selected_adapted_content(monkeypatch):
    fake_llm = FakeLLM(
        invoke_content="<p>Edited HTML</p>",
        usage_metadata={"input_tokens": 20, "output_tokens": 5, "total_tokens": 25},
    )
    monkeypatch.setattr(routes, "build_llm", lambda: fake_llm)

    response = client.post(
        "/prepublish/edit",
        headers=auth_headers(),
        json={
            "platform": "wechat",
            "title": "Release",
            "message": "Polish this",
            "adapted_content": {
                "format": "html",
                "html": "<p>Original HTML</p>",
                "text": "Original text",
            },
        },
    )

    assert response.status_code == 200
    body = response.json()
    assert body["channel"] == "prepublish"
    assert body["platform"] == "wechat"
    assert body["content"] == "<p>Edited HTML</p>"
    assert body["usage"]["input_tokens"] == 20
    assert body["usage"]["output_tokens"] == 5
    assert body["usage"]["total_tokens"] == 25
    assert body["adapted_content"]["format"] == "html"
    assert body["adapted_content"]["html"] == "<p>Edited HTML</p>"
    assert body["adapted_content"]["text"] == "Original text"
    assert fake_llm.messages is not None
    assert "Platform: wechat" in fake_llm.messages[-1].content


def test_edit_prepublish_rejects_empty_adapted_text():
    response = client.post(
        "/prepublish/edit",
        headers=auth_headers(),
        json={
            "platform": "wechat",
            "message": "Polish this",
            "adapted_content": {"format": "markdown", "markdown": "   "},
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "adapted_content text is required"


def test_stream_edit_content_streams_llm_chunks(monkeypatch):
    fake_llm = FakeLLM(stream_contents=[[{"text": "hello"}], " world"])
    monkeypatch.setattr(routes, "build_llm", lambda: fake_llm)

    response = client.post(
        "/content/edit/stream",
        headers=auth_headers(),
        json={"content": "Original content", "message": "Stream it"},
    )

    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/markdown")
    assert response.text == "hello world"
    assert fake_llm.messages is not None
    assert "Stream it" in fake_llm.messages[-1].content


def test_ai_business_routes_require_internal_bearer_token():
    response = client.post(
        "/content/edit",
        json={"content": "Original content", "message": "Edit it"},
    )

    assert response.status_code == 401
    assert response.json()["detail"] == "unauthorized"


def test_ai_business_routes_reject_wrong_internal_bearer_token():
    response = client.post(
        "/content/edit",
        headers={"Authorization": "Bearer wrong-token"},
        json={"content": "Original content", "message": "Edit it"},
    )

    assert response.status_code == 401
    assert response.json()["detail"] == "unauthorized"


def test_ai_business_routes_fail_closed_when_internal_token_missing(monkeypatch):
    monkeypatch.delenv(routes.AI_SERVICE_INTERNAL_TOKEN_ENV, raising=False)

    response = client.post(
        "/content/edit",
        headers=auth_headers(),
        json={"content": "Original content", "message": "Edit it"},
    )

    assert response.status_code == 503
    assert response.json()["detail"] == "AI service internal token is not configured"


def test_edit_content_returns_generic_detail_for_runtime_errors(monkeypatch, caplog):
    def raise_provider_error():
        raise RuntimeError("provider key sk-test-secret failed at internal.host")

    monkeypatch.setattr(routes, "build_llm", raise_provider_error)

    with caplog.at_level(logging.ERROR):
        response = client.post(
            "/content/edit",
            headers=auth_headers(),
            json={"content": "Original content", "message": "Edit it"},
        )

    assert response.status_code == 502
    assert response.json()["detail"] == routes.GENERIC_AI_FAILURE_DETAIL
    assert "sk-test-secret" not in response.text
    assert "internal.host" not in response.text
    assert "provider key sk-test-secret" in caplog.text


def test_stream_growth_optimization_emits_reviewable_proposals(monkeypatch):
    fake_llm = FakeLLM(invoke_content="not json")
    monkeypatch.setattr(routes, "build_llm", lambda: fake_llm)

    response = client.post(
        "/growth/optimize/stream",
        headers=auth_headers(),
        json={
            "title": "Launch note",
            "source_content": "Original long-form article",
            "goal": "improve platform fit",
            "intensity": "balanced",
            "target_platforms": ["wechat", "zhihu", "x", "douyin"],
        },
    )

    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/event-stream")
    assert "event: status" in response.text
    assert "event: proposal" in response.text
    assert "wechat@growth-v1" in response.text
    assert "zhihu@growth-v1" in response.text
    assert "x@growth-v1" in response.text
    assert "douyin@growth-v1" in response.text
    assert '"status": "ready"' in response.text
    assert fake_llm.messages is not None
    assert "wechat@growth-v1" in fake_llm.messages[-1].content
