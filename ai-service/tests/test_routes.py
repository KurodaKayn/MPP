from types import SimpleNamespace

from fastapi.testclient import TestClient

import routes
from main import app


class FakeLLM:
    def __init__(self, *, invoke_content="", stream_contents=None):
        self.invoke_content = invoke_content
        self.stream_contents = stream_contents or []
        self.messages = None

    async def ainvoke(self, messages):
        self.messages = messages
        return SimpleNamespace(content=self.invoke_content)

    async def astream(self, messages):
        self.messages = messages
        for content in self.stream_contents:
            yield SimpleNamespace(content=content)


client = TestClient(app)


def test_health_returns_status():
    response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"status": "healthy"}


def test_edit_content_returns_llm_content(monkeypatch):
    fake_llm = FakeLLM(invoke_content="  edited copy  ")
    monkeypatch.setattr(routes, "build_llm", lambda: fake_llm)

    response = client.post(
        "/content/edit",
        json={
            "content": "Original content",
            "message": "Make it shorter",
            "title": "Launch note",
            "conversation": [{"role": "assistant", "content": "Previous advice"}],
        },
    )

    assert response.status_code == 200
    assert response.json() == {"channel": "content", "content": "edited copy"}
    assert fake_llm.messages is not None
    assert "Make it shorter" in fake_llm.messages[-1].content


def test_edit_content_rejects_blank_message():
    response = client.post(
        "/content/edit",
        json={"content": "Original content", "message": "   "},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "message is required"


def test_edit_prepublish_updates_selected_adapted_content(monkeypatch):
    fake_llm = FakeLLM(invoke_content="<p>Edited HTML</p>")
    monkeypatch.setattr(routes, "build_llm", lambda: fake_llm)

    response = client.post(
        "/prepublish/edit",
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
    assert body["adapted_content"]["format"] == "html"
    assert body["adapted_content"]["html"] == "<p>Edited HTML</p>"
    assert body["adapted_content"]["text"] == "Original text"
    assert fake_llm.messages is not None
    assert "Platform: wechat" in fake_llm.messages[-1].content


def test_edit_prepublish_rejects_empty_adapted_text():
    response = client.post(
        "/prepublish/edit",
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
        json={"content": "Original content", "message": "Stream it"},
    )

    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/markdown")
    assert response.text == "hello world"
    assert fake_llm.messages is not None
    assert "Stream it" in fake_llm.messages[-1].content
