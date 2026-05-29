import os
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from dotenv import load_dotenv
from langchain_openai import ChatOpenAI
from langchain_openai import OpenAIEmbeddings
from langchain_core.prompts import ChatPromptTemplate
from langchain.agents import create_openai_tools_agent, AgentExecutor

load_dotenv()

app = FastAPI(title="Multi-Platform Poster AI Service")

# Initialize LLM
llm = ChatOpenAI(model="gpt-4o", temperature=0)

class CalibrateRequest(BaseModel):
    content: str
    platform: str

@app.get("/health")
async def health():
    return {"status": "healthy"}

@app.post("/calibrate")
async def calibrate(request: CalibrateRequest):
    try:
        # Simple prompt for format calibration
        prompt = ChatPromptTemplate.from_messages([
            ("system", "You are an expert social media manager. Calibrate the following content for {platform} rules and style."),
            ("user", "{content}")
        ])
        
        chain = prompt | llm
        response = chain.invoke({"platform": request.platform, "content": request.content})
        
        return {
            "platform": request.platform,
            "calibrated_content": response.content
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
