# 🚀 ZenGate AI

**AI-Native Distributed API Gateway & Rate Limiter**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Python](https://img.shields.io/badge/Python-3.11+-3776AB?logo=python&logoColor=white)](https://python.org)
[![Google ADK](https://img.shields.io/badge/Google_ADK-1.0+-4285F4?logo=google&logoColor=white)](https://google.github.io/adk-docs/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

> A production-grade distributed API Gateway with embedded AI agents, built using an AI-first development workflow powered by Google ADK.

---

## ✨ Features

### Gateway Core
- ⚡ **High-performance reverse proxy** — Go `net/http` with configurable timeouts
- 🔒 **Rate limiting** — Atomic Redis Lua scripts (Sliding Window Log)
- 🔄 **Dynamic configuration** — etcd watchers with zero-downtime hot reload
- 🔑 **JWT authentication** — Edge-level auth offloading
- 📊 **Prometheus metrics** — TPS, P99 latency, error rates
- 🩺 **Health checks** — Rich JSON health endpoint

### AI Brain (Embedded Agents)
- 📈 **Traffic Analyzer** — Detects anomalous traffic patterns (> 3σ spike)
- 🔧 **Self-Healer** — Routes around failing upstreams automatically
- 💬 **Config Translator** — Natural language → JSON policy ("set limit to 500/min")

### AI Development Workflow
- 🤖 **Google ADK Pipeline** — Architect → CodeGen → Reviewer → Tester → DocWriter
- 🔁 **Conditional branching** — Reviewer rejects → auto re-route to CodeGen
- 🔄 **Retry with backoff** — Exponential backoff on agent failures
- 👤 **Human-in-the-loop** — Review gates at every stage

---

## 🏗️ Architecture

```
User → Cloudflare → HAProxy → [Gateway Cluster] → Backend Services
                                     ↕
                              Redis (Rate Limit)
                              etcd (Config)
                              Redis Streams (Audit)
                                     ↕
                              Prometheus → Grafana
```

---

## 🚀 Quick Start

### Prerequisites
- **Go** 1.22+ ([install](https://go.dev/dl/))
- **Python** 3.11+ ([install](https://python.org/downloads/))
- **Docker** (optional, for full stack)

### Run the Gateway

```bash
# Build
go build -o bin/zengate ./cmd/gateway

# Run (with a mock upstream)
ZENGATE_UPSTREAM_URL=http://httpbin.org ./bin/zengate
```

### Run with Docker Compose

```bash
# Start full stack (gateway + Redis + Prometheus + Grafana)
docker compose up -d

# View logs
docker compose logs -f gateway

# Open Grafana dashboard
open http://localhost:3000  # admin / zengate
```

### Test the Gateway

```bash
# Health check
curl http://localhost:8080/health

# Proxy a request
curl http://localhost:8080/get

# View metrics
curl http://localhost:8080/metrics
```

---

## 📁 Project Structure

```
zengate/
├── cmd/gateway/          # Gateway entry point
│   └── main.go
├── internal/
│   ├── config/           # Configuration management
│   ├── proxy/            # Reverse proxy + middleware chain
│   ├── health/           # Health check endpoint
│   ├── metrics/          # Prometheus metrics
│   ├── ratelimit/        # Rate limiting engine (Phase 2)
│   ├── controlplane/     # Admin API + etcd (Phase 2)
│   ├── auth/             # JWT validation (Phase 2)
│   └── ai/               # Embedded AI agents (Phase 3)
├── adk/                  # Google ADK development agents
│   ├── agents/
│   │   ├── orchestrator.py
│   │   ├── architect.py
│   │   ├── codegen.py
│   │   ├── reviewer.py
│   │   ├── tester.py
│   │   └── docwriter.py
│   └── config.py
├── deploy/
│   └── prometheus/
├── scripts/              # Redis Lua scripts (Phase 2)
├── k6/                   # Load tests (Phase 4)
├── docs/                 # Documentation
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── go.mod
```

---

## 🧠 AI Development Pipeline

The project is built using a multi-agent pipeline powered by Google ADK:

```
📋 Your Task
    ↓
🏛️ Architect Agent → Architecture doc + interface contracts
    ↓
💻 CodeGen Agent → Go source files + tests
    ↓
🔍 Reviewer Agent → Approve ✓ or Reject ✗ (→ loop to CodeGen)
    ↓
🧪 Tester Agent → go test + lint + coverage
    ↓
📝 DocWriter Agent → README + API docs
    ↓
👤 Your Review → Merge or iterate
```

---

## 🛠️ Tech Stack

| Component | Technology | Why |
|:---|:---|:---|
| Gateway | **Go** | Low memory, fast goroutines, stdlib `net/http` |
| Rate Limiting | **Redis Lua** | Atomic sliding window, < 2ms p99 |
| Config Store | **etcd** | Real-time push via gRPC watchers |
| Message Queue | **Redis Streams** | Lightweight, reuses existing Redis |
| AI Orchestration | **Google ADK** | Multi-agent DAG with conditional routing |
| LLM | **DeepSeek + Gemini** | Free-tier dual fallback chain |
| Monitoring | **Prometheus + Grafana** | Industry standard observability |
| Hosting | **Oracle Cloud Free** | 4 ARM cores, 24GB RAM, always-on |

---

## 📄 License

MIT License — see [LICENSE](LICENSE) for details.

---

## 🤝 Contributing

This is a personal portfolio project. Issues and discussions welcome!
