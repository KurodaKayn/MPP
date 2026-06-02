from fastapi.testclient import TestClient

from main import app

client = TestClient(app)


def test_observability_propagates_request_id_and_exposes_metrics():
    response = client.get("/health", headers={"x-request-id": "trace-test"})

    assert response.status_code == 200
    assert response.headers["x-request-id"] == "trace-test"
    assert response.headers["x-trace-id"] == "trace-test"

    metrics = client.get("/metrics").text

    assert "mpp_service_info" in metrics
    assert 'service="ai-service"' in metrics
    assert "mpp_http_requests_total" in metrics
    assert 'route="/health"' in metrics
