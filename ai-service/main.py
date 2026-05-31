from dotenv import load_dotenv
from fastapi import FastAPI

from routes import router

load_dotenv()

app = FastAPI(title="Multi-Platform Poster AI Service")
app.include_router(router)


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)
