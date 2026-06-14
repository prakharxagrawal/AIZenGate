# ZenGate AI — System Architecture & Design Specification

This document provides a detailed technical specification of the ZenGate AI Distributed API Gateway, detailing system layout, data flow pipelines, configurations, and multi-agent development architecture.

---

## 🏗️ 1. System Topology Overview

ZenGate AI is structured as a stateless gateway proxy layer that coordinates with distributed state stores (Redis, etcd) to enforce access policies, perform real-time rate limiting, collect telemetry, and support runtime configuration hot reloading.

```mermaid
graph TD
    Client["🌐 Client (browser/curl/k6)"]
    CF["☁️ Cloudflare Tunnel / SSL"]
    GW1["🚀 ZenGate Node 1"]
    GW2["🚀 ZenGate Node 2"]
    Upstream["📦 Upstream Backend Service"]
    Redis[("💾 Redis Cluster\n(Rate Limiting & Audit Stream)")]
    Etcd[("🗂️ etcd Cluster\n(Configuration Store)")]
    Prom["📊 Prometheus\n(Metrics Collector)"]
    Grafana["🖥️ Grafana\n(Telemetry Dashboard)"]

    Client -->|HTTPS| CF
    CF -->|HTTP Proxy| GW1
    CF -->|HTTP Proxy| GW2
    
    GW1 -->|Reverse Proxy| Upstream
    GW2 -->|Reverse Proxy| Upstream

    GW1 <-->|Atomic sliding window Lua| Redis
    GW2 <-->|Atomic sliding window Lua| Redis
    
    GW1 <-->|etcd Watcher / sync.Map| Etcd
    GW2 <-->|etcd Watcher / sync.Map| Etcd

    Prom -.->|Scrape metrics /health & /metrics| GW1
    Prom -.->|Scrape metrics /health & /metrics| GW2
    Prom -.->|Query TSDB| Grafana
```

---

## 🔀 2. Request Lifecycle & Data Flow

When an HTTP request hits a ZenGate node, it flows through a middleware pipeline before reaching the reverse proxy handler.

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant GW as ZenGate Proxy
    participant RL as Rate Limiter (Redis)
    participant Auth as Auth Middleware (JWT)
    participant Upstream as Target Backend
    participant Prom as Prometheus Metrics

    Client->>GW: HTTP Request (e.g. GET /get)
    Note over GW: Request ID Middleware injects unique UUID/timestamp
    Note over GW: Log request initialization
    
    GW->>Auth: Validate JWT / API Key
    alt Invalid Credentials
        Auth-->>Client: 401 Unauthorized / 403 Forbidden
    end

    GW->>RL: Check Limits (Sliding Window Log via Lua)
    alt Rate Limit Exceeded
        RL-->>Client: 429 Too Many Requests
    end

    Note over GW: Start latency timers
    GW->>Upstream: Forward Request (preserve host, inject X-Forwarded headers)
    Upstream-->>GW: HTTP Response (e.g. 200 OK)
    Note over GW: Calculate roundtrip latency (X-Upstream-Latency)
    Note over GW: Append headers (X-Powered-By: ZenGate AI)

    GW->>Prom: Increment request counts & latency histograms
    GW-->>Client: Return HTTP Response
```

---

## ⚙️ 3. Dynamic Configuration & Hot Reload

ZenGate AI leverages etcd to enable distributed policy configuration without gateway restarts. The control plane writes rules to etcd, and gateway nodes reload policies instantly via watchers.

```mermaid
flowchart LR
    Admin["👤 Admin / CLI"]
    NLPolicy["💬 NL Input: 'rate limit 5k/min'"]
    Agent["🧠 Config Translator Agent"]
    CP["🛡️ Control Plane API"]
    Etcd[("🗂️ etcd Cluster")]
    GW["🚀 ZenGate Nodes (Watcher)"]
    Cache["💾 In-memory sync.Map"]

    Admin --> NLPolicy
    NLPolicy --> Agent
    Agent -->|Structured JSON Policy| CP
    CP -->|PUT /keys/policies| Etcd
    Etcd -->|gRPC Watch Stream Event| GW
    GW -->|Decode & Atomic Update| Cache
```

### Hot Reload Details:
1. **Startup:** Each ZenGate node loads configuration policies from etcd and populates a thread-safe `sync.Map`.
2. **Subscription:** Nodes spawn a background goroutine hosting an `etcdv3` watch client targeting the `/policies/` key prefix.
3. **Trigger:** When etcd pushes a modify/create/delete event, the watch client intercepts it, decodes the payload, and performs an atomic swap on the local `sync.Map` store.
4. **Performance:** Request routing lookups query the `sync.Map` cache directly. This guarantees `<1µs` local lookup overhead while executing live hot updates within `<500ms` cluster-wide propagation.

---

## 🧠 4. ADK Development Multi-Agent Pipeline

To automate coding, testing, and documentation tasks, ZenGate integrates a Python multi-agent pipeline scaffolded using Google ADK.

```mermaid
stateDiagram-v2
    [*] --> Architect : User Task
    Architect --> CodeGen : Technical Specification & Interfaces
    CodeGen --> Reviewer : Generated Go Code & Tests
    
    state Reviewer_Decision <<choice>>
    Reviewer --> Reviewer_Decision : Code Audit
    Reviewer_Decision --> CodeGen : Reject (Iterate with Feedback)
    Reviewer_Decision --> Tester : Approve
    
    state Tester_Decision <<choice>>
    Tester --> Tester_Decision : Run go test & golangci-lint
    Tester_Decision --> CodeGen : Tests/Lint Fail (Fix bugs)
    Tester_Decision --> DocWriter : Tests Pass
    
    DocWriter --> [*] : Updated README, API Docs, & Artifacts
```

### Agent Roles & Workspaces:
- **Orchestrator:** Manages execution flow, retry limits, and shared file state memory (`.zengate-adk-memory`).
- **Architect:** Designs interface structures and creates interface contracts.
- **CodeGen:** Generates Go code, handlers, middleware, and unit tests.
- **Reviewer:** Checks correctness, safety, concurrency bugs, and race conditions.
- **Tester:** Invokes linting checklists and runs `go test -race -cover`.
- **DocWriter:** Automatically drafts API specs, diagrams, and README references.
