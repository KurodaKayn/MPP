FROM python:3.12-alpine AS builder
RUN pip install uv -i https://pypi.tuna.tsinghua.edu.cn/simple
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy
WORKDIR /app
RUN --mount=type=cache,target=/root/.cache/uv \
    --mount=type=bind,source=uv.lock,target=uv.lock \
    --mount=type=bind,source=pyproject.toml,target=pyproject.toml \
    uv sync --frozen --no-install-project --no-dev
COPY . /app
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev

FROM python:3.12-alpine AS dev
RUN pip install uv -i https://pypi.tuna.tsinghua.edu.cn/simple
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen
COPY . /app
ENV PATH="/app/.venv/bin:$PATH"
EXPOSE 8000
CMD ["sh", "-c", "uv sync --frozen && uvicorn main:app --host 0.0.0.0 --port 8000 --reload"]

FROM python:3.12-alpine
WORKDIR /app
RUN addgroup -S app -g 10001 && adduser -S app -G app -u 10001 && chown app:app /app
COPY --from=builder --chown=app:app /app /app
ENV HOME=/home/app PATH="/app/.venv/bin:$PATH" PYTHONDONTWRITEBYTECODE=1
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8000/ready >/dev/null || exit 1
USER app
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
