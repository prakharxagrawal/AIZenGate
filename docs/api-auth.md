# 🔐 Authentication API Documentation

The ZenGate AI Gateway enforces a strict authentication policy. All requests to protected endpoints must be authenticated using the `Authorization` header.

## Authentication Scheme

The gateway uses the **Bearer Token** authentication scheme.

**Header:** `Authorization: Bearer <token>`

## Request Lifecycle

1. **Extraction**: The gateway looks for the `Authorization` header.
2. **Format Check**: It verifies the header starts with `Bearer ` (case-insensitive).
3. **Validation**: The token is checked for:
    - Valid cryptographic signature.
    - Expiration date (`exp` claim).
    - Presence in the Redis revocation blacklist.
    - Required claims (`sub` for UserID, `tid` for TenantID).

## Error Responses

When authentication fails, the gateway returns a `401 Unauthorized` status code with a JSON body.

### 1. Missing or Malformed Token
Returned when the `Authorization` header is missing or does not follow the `Bearer <token>` format.

**Response:**
- **Status**: `401 Unauthorized`
- **Body**: