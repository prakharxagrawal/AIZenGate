# 🛡️ ZenGate AI - Request Authentication Guard

![Build Status](https://img.shields.io/badge/build-passing-brightgreen)
![Coverage](https://img.shields.io/badge/coverage-88%25-blue)
![License](https://img.shields.io/badge/license-Apache%202.0-orange)

The **Request Authentication Guard** is a mandatory security layer within the ZenGate AI Gateway. It ensures that no unauthorized or anonymous traffic reaches downstream microservices by enforcing strict token validation at the edge.

## 🚀 Overview

The Guard intercepts every incoming HTTP request and validates the presence and integrity of a Bearer token. If the token is missing, expired, or blacklisted, the gateway terminates the request immediately with a `401 Unauthorized` status.

### Key Features
- **Fail-Fast Architecture**: Blocks unauthorized requests at the outermost layer to protect internal resources.
- **Stateless Validation**: Uses JWT (JSON Web Tokens) for high-performance, database-less verification.
- **Token Revocation**: Integrated Redis-backed blacklist to handle immediate logouts and revoked credentials.
- **Context Enrichment**: Injects `UserIdentity` into the request context, allowing downstream services to access user metadata without re-validating tokens.

## 🛠️ Architecture
