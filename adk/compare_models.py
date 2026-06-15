import os
import asyncio
import time
import google.genai as genai

async def test_model(model_name: str, prompt: str, api_key: str):
    client = genai.Client(api_key=api_key)
    start = time.time()
    try:
        response = await client.aio.models.generate_content(
            model=model_name,
            contents=prompt,
        )
        duration = time.time() - start
        parts = response.candidates[0].content.parts if response.candidates else []
        text = "".join(p.text for p in parts if p.text)
        print(f"[{model_name}] Succeeded in {duration:.2f}s")
        print(f"  Response: {text[:100]}...\n")
        return duration
    except Exception as e:
        print(f"[{model_name}] Failed: {e}\n")
        return None

async def main():
    api_key = os.getenv("GEMINI_API_KEY", "")
    if not api_key:
        from dotenv import load_dotenv
        from pathlib import Path
        load_dotenv(dotenv_path=Path(__file__).resolve().parent.parent / ".env")
        api_key = os.getenv("GEMINI_API_KEY", "")
        
    print(f"GEMINI_API_KEY present: {bool(api_key)}")
    prompt = "Write a short summary of how a rate limiter works in 2 sentences."
    
    # Test Gemma-4-31b-it
    await test_model("gemma-4-31b-it", prompt, api_key)
    
    # Test Gemini-3.1-flash-lite
    await test_model("gemini-3.1-flash-lite", prompt, api_key)

if __name__ == "__main__":
    asyncio.run(main())
