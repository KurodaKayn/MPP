import json
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


def sse_events(response_text: str) -> list[dict]:
    events = []
    for block in response_text.strip().split("\n\n"):
        event = {}
        for line in block.splitlines():
            if line.startswith("event:"):
                event["event"] = line.removeprefix("event:").strip()
            elif line.startswith("data:"):
                event["data"] = json.loads(line.removeprefix("data:").strip())
        if event:
            events.append(event)
    return events


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
    assert "Dramatic but fact-bound WeChat title" in response.text
    assert "Specific technical title" in response.text
    assert "Indirect metaphor or double-meaning title" in response.text
    assert "Plain title that directly states" in response.text
    assert '"status": "ready"' in response.text
    fallback_proposal = next(
        event["data"] for event in sse_events(response.text) if event["event"] == "proposal"
    )
    assert "brand_consistency" in fallback_proposal["quality_checks"]
    assert "platform_format" in fallback_proposal["quality_checks"]
    assert "risk_statements" in fallback_proposal["quality_checks"]
    assert fake_llm.messages is not None
    assert "wechat@growth-v1" in fake_llm.messages[-1].content
    assert "title_strategy" in fake_llm.messages[-1].content
    assert "verification warning instead of inventing data" in fake_llm.messages[-1].content


def test_stream_growth_optimization_streams_valid_json_proposals(monkeypatch):
    fake_llm = FakeLLM(
        invoke_content=json.dumps(
            {
                "model": "test-growth-model",
                "prompt_version": "growth-v1",
                "quality_summary": "Ready for review",
                "proposals": [
                    {
                        "proposal_type": "prepublish_patch",
                        "target_platform": "zhihu",
                        "summary": "zhihu@growth-v1 technical proposal",
                        "patch": "",
                        "full_content": "A rigorous Zhihu rewrite",
                        "quality_checks": {
                            "audience_profile": "zhihu@growth-v1",
                            "title_strategy": "specific technical title",
                        },
                    }
                ],
                "usage": {
                    "input_tokens": 1,
                    "output_tokens": 2,
                    "total_tokens": 3,
                    "cost": 0,
                    "currency": "USD",
                },
            }
        ),
        usage_metadata={"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
    )
    monkeypatch.setattr(routes, "build_llm", lambda: fake_llm)

    response = client.post(
        "/growth/optimize/stream",
        headers=auth_headers(),
        json={
            "title": "Domestic model report",
            "source_content": "DeepSeek article source",
            "goal": "make it rigorous",
            "target_platforms": ["zhihu"],
        },
    )

    assert response.status_code == 200
    assert "A rigorous Zhihu rewrite" in response.text
    assert "test-growth-model" in response.text
    assert '"total_tokens": 15' in response.text


def test_stream_growth_optimization_adds_deterministic_quality_checks(monkeypatch):
    fake_llm = FakeLLM(
        invoke_content=json.dumps(
            {
                "model": "test-growth-model",
                "prompt_version": "growth-v1",
                "quality_summary": "Ready for review",
                "proposals": [
                    {
                        "proposal_type": "prepublish_patch",
                        "target_platform": "x",
                        "summary": "x@growth-v1 concise proposal",
                        "patch": "",
                        "full_content": (
                            "Forbidden claim with guaranteed traffic and no requested CTA."
                        ),
                        "quality_checks": {"audience_profile": "x@growth-v1"},
                    }
                ],
                "usage": {
                    "input_tokens": 1,
                    "output_tokens": 2,
                    "total_tokens": 3,
                    "cost": 0,
                    "currency": "USD",
                },
            }
        )
    )
    monkeypatch.setattr(routes, "build_llm", lambda: fake_llm)

    response = client.post(
        "/growth/optimize/stream",
        headers=auth_headers(),
        json={
            "title": "Launch note",
            "source_content": "Source article",
            "goal": "make it concise",
            "target_platforms": ["x"],
            "brand_profile": {
                "voice": "precise",
                "audience": "technical founders",
                "banned_words": ["Forbidden"],
                "cta": "Read the full report",
            },
        },
    )

    assert response.status_code == 200
    proposal = next(
        event["data"] for event in sse_events(response.text) if event["event"] == "proposal"
    )
    checks = proposal["quality_checks"]
    assert checks["audience_profile"] == "x@growth-v1"
    assert checks["brand_consistency"]["status"] == "fail"
    assert checks["brand_consistency"]["voice"] == "precise"
    assert checks["banned_words"] == {"status": "fail", "matches": ["Forbidden"]}
    assert checks["cta"]["status"] == "warning"
    assert checks["cta"]["required"] == "Read the full report"
    assert checks["length"]["status"] == "pass"
    assert checks["platform_format"]["expected_format"] == "plain_text_short_form"
    assert checks["risk_statements"]["status"] == "fail"


def test_growth_quality_checks_avoid_common_false_positives():
    proposal = routes.GrowthProposal(
        proposal_type="prepublish_patch",
        target_platform="x",
        summary="x@growth-v1 concise proposal",
        full_content="What changed in concatenated metrics? CPU usage < 5% after tuning.",
    )
    request = routes.GrowthOptimizationRequest(
        title="Launch note",
        source_content="Source article",
        goal="make it concise",
        target_platforms=["x"],
        brand_profile={"banned_words": ["cat"]},
    )

    checks = routes.growth_quality_checks_for(proposal, request)

    assert checks["banned_words"] == {"status": "pass", "matches": []}
    assert checks["cta"]["status"] == "warning"
    assert checks["platform_format"]["status"] == "pass"


def test_growth_quality_checks_detect_html_tags_for_x():
    proposal = routes.GrowthProposal(
        proposal_type="prepublish_patch",
        target_platform="x",
        summary="x@growth-v1 concise proposal",
        full_content="<p>HTML is not a plain-text X proposal.</p>",
    )
    request = routes.GrowthOptimizationRequest(
        title="Launch note",
        source_content="Source article",
        goal="make it concise",
        target_platforms=["x"],
    )

    checks = routes.growth_quality_checks_for(proposal, request)

    assert checks["platform_format"]["status"] == "warning"
    assert checks["platform_format"]["warnings"] == ["X proposal should be plain text, not HTML."]
