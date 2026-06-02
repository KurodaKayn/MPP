import json
import time
from uuid import uuid4

from fastapi import FastAPI, Request, Response
from prometheus_client import (
    CONTENT_TYPE_LATEST,
    Counter,
    Gauge,
    Histogram,
    generate_latest,
)

REQUEST_ID_HEADER = "x-request-id"
TRACE_ID_HEADER = "x-trace-id"
SERVICE_NAME = "ai-service"

REQUESTS = Counter(
    "mpp_http_requests_total",
    "Total HTTP requests served by MPP services.",
    ("service", "method", "route", "status"),
)
DURATION = Histogram(
    "mpp_http_request_duration_seconds",
    "HTTP request duration by service, method, route, and status.",
    ("service", "method", "route", "status"),
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
)
IN_FLIGHT = Gauge(
    "mpp_http_in_flight_requests",
    "Current in-flight HTTP requests by MPP service.",
    ("service",),
)
INFO = Gauge(
    "mpp_service_info",
    "Static information marker for an MPP service.",
    ("service",),
)
INFO.labels(SERVICE_NAME).set(1)


def configure_observability(app: FastAPI) -> None:
    @app.middleware("http")
    async def request_observability(request: Request, call_next):
        trace_id = _trace_id_from_request(request)
        request.state.trace_id = trace_id
        request.state.request_id = trace_id
        started_at = time.perf_counter()
        status_code = 500
        error = ""
        response = None

        IN_FLIGHT.labels(SERVICE_NAME).inc()
        try:
            response = await call_next(request)
            status_code = response.status_code
            response.headers[REQUEST_ID_HEADER] = trace_id
            response.headers[TRACE_ID_HEADER] = trace_id
            return response
        except Exception as exc:
            error = str(exc)
            raise
        finally:
            IN_FLIGHT.labels(SERVICE_NAME).dec()
            route = _route_path(request)
            latency = time.perf_counter() - started_at
            REQUESTS.labels(SERVICE_NAME, request.method, route, str(status_code)).inc()
            DURATION.labels(
                SERVICE_NAME, request.method, route, str(status_code)
            ).observe(latency)
            _log_request(
                request, trace_id, route, status_code, latency, response, error
            )

    @app.get("/metrics", include_in_schema=False)
    async def metrics() -> Response:
        return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


def _trace_id_from_request(request: Request) -> str:
    request_id = request.headers.get(REQUEST_ID_HEADER, "").strip()
    if request_id:
        return request_id
    trace_id = request.headers.get(TRACE_ID_HEADER, "").strip()
    if trace_id:
        return trace_id
    return str(uuid4())


def _route_path(request: Request) -> str:
    route = request.scope.get("route")
    route_path = getattr(route, "path", None)
    return route_path or request.url.path or "unknown"


def _log_request(
    request: Request,
    trace_id: str,
    route: str,
    status_code: int,
    latency: float,
    response,
    error: str,
) -> None:
    event = {
        "time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "service": SERVICE_NAME,
        "trace_id": trace_id,
        "request_id": trace_id,
        "method": request.method,
        "path": request.url.path,
        "route": route,
        "status": status_code,
        "latency_ms": round(latency * 1000, 3),
        "remote_ip": request.client.host if request.client else "",
        "user_agent": request.headers.get("user-agent", ""),
        "bytes_in": _content_length(request),
        "bytes_out": _response_size(response),
    }
    if error:
        event["error"] = error
    print(json.dumps(event, separators=(",", ":")), flush=True)


def _content_length(request: Request) -> int:
    value = request.headers.get("content-length", "").strip()
    if not value:
        return 0
    try:
        return int(value)
    except ValueError:
        return 0


def _response_size(response) -> int:
    if response is None:
        return 0
    value = response.headers.get("content-length", "").strip()
    if not value:
        return 0
    try:
        return int(value)
    except ValueError:
        return 0
