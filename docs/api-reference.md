# Winnow REST API Reference

Base URL: `https://winnow-api.xferops.dev`

## Authentication

All endpoints (except `/health` and `/v1/keys`) require:

```
Authorization: Bearer ctx_your-org_YOUR_KEY_HERE
```

### Error format

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "context chunk not found"
  }
}
```

---

## Endpoints

### `GET /health`

Health check. No auth required.

**Response 200:**
```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

---

### `POST /v1/keys`

Create a new API key. No auth required (bootstrap endpoint).

**Request:**
```json
{
  "org_slug": "my-team"
}
```

**Response 201:**
```json
{
  "key": "ctx_my-team_abc123..."
}
```

> ⚠️ The key is only returned once. Store it securely.

---

### `POST /v1/context`

Write a new context chunk.

**Request:**
```json
{
  "query_key": "auth-flow",
  "title": "JWT middleware validates tokens at every request",
  "content": "The auth middleware in internal/auth/middleware.go validates JWTs on every protected route. It checks expiry, signature, and extracts claims...",
  "source_file": "internal/auth/middleware.go",
  "source_lines": [42, 67],
  "gotchas": ["Token expiry is checked server-side, not just decoded"],
  "related": ["chunk-id-for-login-handler"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query_key` | string | ✅ | Namespace tag for this chunk |
| `title` | string | ✅ | Short descriptor |
| `content` | string | ✅ | The actual knowledge content |
| `source_file` | string | — | File path this chunk relates to |
| `source_lines` | [int, int] | — | Line range `[start, end]` |
| `gotchas` | string[] | — | Known pitfalls or warnings |
| `related` | string[] | — | IDs of related chunks |

**Response 201:**
```json
{
  "id": "cuid_abc123",
  "query_key": "auth-flow",
  "title": "JWT middleware validates tokens at every request",
  "created_at": "2026-03-10T00:00:00Z"
}
```

---

### `GET /v1/context/search`

Semantic vector search across chunks.

**Query params:**

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | ✅ | Natural language search query |
| `limit` | int | — | Max results (default: 10) |
| `query_key` | string | — | Restrict to this namespace |

**Example:**
```
GET /v1/context/search?query=how+does+authentication+work&limit=5&query_key=auth-flow
```

**Response 200:**
```json
{
  "results": [
    {
      "id": "cuid_abc123",
      "query_key": "auth-flow",
      "title": "JWT middleware validates tokens at every request",
      "content": "...",
      "source_file": "internal/auth/middleware.go",
      "source_lines": [42, 67],
      "gotchas": ["Token expiry is checked server-side"],
      "related": [],
      "score": 0.92,
      "version": 2,
      "created_at": "2026-03-10T00:00:00Z",
      "updated_at": "2026-03-11T00:00:00Z"
    }
  ]
}
```

Results are sorted by relevance score (cosine similarity via pgvector) × recency.

---

### `GET /v1/context/compact`

Fetch chunks for client-side compaction. Returns the most relevant chunks for summarization.

**Query params:**

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | ✅ | Topic to compact around |
| `limit` | int | — | Max chunks to return (default: 20) |

**Example:**
```
GET /v1/context/compact?query=authentication+system&limit=15
```

**Response 200:**
```json
{
  "chunks": [
    {
      "id": "cuid_abc123",
      "query_key": "auth-flow",
      "title": "...",
      "content": "...",
      ...
    }
  ],
  "total": 15
}
```

> After receiving chunks, your agent should summarize them into a single new chunk via `POST /v1/context`, then delete the source chunks if desired.

---

### `GET /v1/context/:id`

Retrieve a specific context chunk with its full version history.

**Response 200:**
```json
{
  "id": "cuid_abc123",
  "query_key": "auth-flow",
  "title": "JWT middleware validates tokens at every request",
  "content": "...",
  "source_file": "internal/auth/middleware.go",
  "source_lines": [42, 67],
  "gotchas": ["Token expiry is checked server-side"],
  "related": [],
  "score": 0,
  "version": 2,
  "created_at": "2026-03-10T00:00:00Z",
  "updated_at": "2026-03-11T00:00:00Z",
  "versions": [
    {
      "version": 1,
      "change_note": "Initial write",
      "created_at": "2026-03-10T00:00:00Z"
    },
    {
      "version": 2,
      "change_note": "Updated after discovering expiry bug",
      "created_at": "2026-03-11T00:00:00Z"
    }
  ]
}
```

**Response 404:**
```json
{
  "error": { "code": "NOT_FOUND", "message": "..." }
}
```

---

### `PATCH /v1/context/:id`

Update a context chunk. Creates a new version, preserving history.

**Request** (all fields optional except `change_note`):
```json
{
  "title": "Updated title",
  "content": "Updated content reflecting new understanding...",
  "source_file": "internal/auth/middleware.go",
  "source_lines": [42, 90],
  "gotchas": ["Token expiry checked server-side", "New gotcha discovered"],
  "related": ["other-chunk-id"],
  "change_note": "Updated after code review revealed edge case"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | — | New title |
| `content` | string | — | New content |
| `source_file` | string | — | Updated file path |
| `source_lines` | [int, int] | — | Updated line range |
| `gotchas` | string[] | — | Replaces existing gotchas |
| `related` | string[] | — | Replaces existing related IDs |
| `change_note` | string | ✅ | Why this was updated |

**Response 200:**
```json
{
  "id": "cuid_abc123",
  "version": 3,
  "updated_at": "2026-03-12T00:00:00Z"
}
```

---

### `DELETE /v1/context/:id`

Permanently delete a context chunk.

**Response 200:**
```json
{
  "deleted": true,
  "id": "cuid_abc123"
}
```

---

### `GET /v1/context/:id/versions`

Retrieve the version history of a chunk.

**Query params:**

| Param | Type | Description |
|-------|------|-------------|
| `limit` | int | Max versions to return (default: all) |

**Response 200:**
```json
{
  "versions": [
    {
      "version": 1,
      "change_note": "Initial write",
      "created_at": "2026-03-10T00:00:00Z"
    },
    {
      "version": 2,
      "change_note": "Updated after discovering expiry bug",
      "created_at": "2026-03-11T00:00:00Z"
    }
  ]
}
```

---

### `POST /v1/context/:id/review`

Submit a quality review for a chunk.

**Request:**
```json
{
  "chunk_id": "cuid_abc123",
  "task": "Implementing OAuth login flow",
  "usefulness": 4,
  "usefulness_note": "Helped me understand the token validation flow",
  "correctness": 5,
  "correctness_note": "Verified against the actual source",
  "action": "useful"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `chunk_id` | string | ✅ | ID of chunk being reviewed |
| `task` | string | — | What task you were doing |
| `usefulness` | int 1-5 | ✅ | How useful was this chunk? |
| `usefulness_note` | string | — | Why that score |
| `correctness` | int 1-5 | ✅ | How accurate/correct? |
| `correctness_note` | string | — | Why that score |
| `action` | enum | ✅ | `useful` \| `needs_update` \| `outdated` \| `incorrect` |

**Response 201:**
```json
{
  "id": "review_xyz789",
  "chunk_id": "cuid_abc123",
  "created_at": "2026-03-12T00:00:00Z"
}
```

---

## HTTP Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 201 | Created |
| 400 | Bad request / invalid body |
| 401 | Missing or invalid API key |
| 404 | Chunk not found |
| 503 | Database unavailable |
