# 🛠️ ZenGate AI Self-Healer

![Build Status](https://img.shields.io/badge/build-passing-brightgreen)
![Coverage](https://img.shields.io/badge/coverage-85%25-blue)
![License](https://img.shields.io/badge/license-Apache%202.0-orange)

The **Self-Healer** is an autonomous recovery system designed to detect system failures (such as crash loops or failed health checks) and automatically apply fixes using the reasoning capabilities of Google Gemini LLMs.

## 🚀 Overview

The system utilizes a "Brain" package that interfaces with Gemini to analyze error logs and system states, generating actionable recovery steps that the self-healer then executes against the infrastructure.

### Architecture Flow
