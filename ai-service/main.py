from contextlib import asynccontextmanager

from dotenv import load_dotenv
from fastapi import FastAPI

from observability import configure_observability
from routes import router

load_dotenv()


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.ready = True
    yield
    app.state.ready = False


app = FastAPI(title="Multi-Platform Poster AI Service", lifespan=lifespan)
app.state.ready = False
configure_observability(app)
app.include_router(router)


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)  # noqa: S104
