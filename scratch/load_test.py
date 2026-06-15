import asyncio
import time
import httpx
from rich.console import Console
from rich.table import Table

BASE_URL = "http://localhost:8080"
CONCURRENT_REQUESTS = 50
TOTAL_REQUESTS = 300

async def make_request(client, request_id):
    start = time.time()
    try:
        # We hit the health endpoint, which is fast and does not require an active upstream
        response = await client.get(f"{BASE_URL}/health", timeout=5.0)
        latency = (time.time() - start) * 1000
        return response.status_code, latency
    except Exception as e:
        latency = (time.time() - start) * 1000
        return "Error", latency

async def run_load_test():
    console = Console()
    console.print(f"\n[bold cyan]🚀 Starting Python Load Test against {BASE_URL}...[/bold cyan]")
    console.print(f"Total Requests: {TOTAL_REQUESTS} | Concurrency: {CONCURRENT_REQUESTS}\n")
    
    limits = httpx.Limits(max_keepalive_connections=CONCURRENT_REQUESTS, max_connections=CONCURRENT_REQUESTS)
    async with httpx.AsyncClient(limits=limits) as client:
        # Create a queue of tasks
        sem = asyncio.Semaphore(CONCURRENT_REQUESTS)
        
        async def worker(req_id):
            async with sem:
                return await make_request(client, req_id)
                
        start_time = time.time()
        results = await asyncio.gather(*(worker(i) for i in range(TOTAL_REQUESTS)))
        total_duration = time.time() - start_time
        
    # Process results
    status_counts = {}
    latencies = []
    errors = 0
    
    for status, latency in results:
        status_counts[status] = status_counts.get(status, 0) + 1
        if status == "Error":
            errors += 1
        else:
            latencies.append(latency)
            
    avg_latency = sum(latencies) / len(latencies) if latencies else 0
    min_latency = min(latencies) if latencies else 0
    max_latency = max(latencies) if latencies else 0
    rps = TOTAL_REQUESTS / total_duration
    
    # Print Results
    table = Table(title="Load Test Summary")
    table.add_column("Metric", style="cyan")
    table.add_column("Value", style="bold")
    
    table.add_row("Total Time Elapsed", f"{total_duration:.2f} seconds")
    table.add_row("Requests Per Second (RPS)", f"{rps:.2f} req/sec")
    table.add_row("Average Latency", f"{avg_latency:.1f} ms")
    table.add_row("Min Latency", f"{min_latency:.1f} ms")
    table.add_row("Max Latency", f"{max_latency:.1f} ms")
    table.add_row("Failed Requests", f"{errors}")
    
    for status, count in status_counts.items():
        if status != "Error":
            table.add_row(f"HTTP Status {status}", f"{count}")
            
    console.print(table)

if __name__ == "__main__":
    asyncio.run(run_load_test())
