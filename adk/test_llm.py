import os
import asyncio
from config import run_agent_async, get_adk_model

async def test():
    print("Environment variables:")
    print("  ADK_LLM_MODEL_ID:", os.getenv("ADK_LLM_MODEL_ID"))
    print("  ADK_LLM_BASE_URL:", os.getenv("ADK_LLM_BASE_URL"))
    print("  GEMINI_API_KEY is configured:", bool(os.getenv("GEMINI_API_KEY")))
    
    model = get_adk_model()
    print("\nModel initialized:", type(model))
    
    print("\nExecuting ADK Agent call through run_agent_async...")
    try:
        response = await run_agent_async("architect", "Say hello in 3 words")
        print("\nADK Agent Response:")
        print(response)
    except Exception as e:
        print("\nError in ADK Agent execution:", e)

if __name__ == "__main__":
    asyncio.run(test())
