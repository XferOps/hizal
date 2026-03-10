# SKILL: winnow-review

## Description
Review workflow for Winnow context chunks. Assesses the quality, accuracy, and usefulness of stored chunks, rates them, and updates or removes chunks that are stale, incorrect, or low-value. Keeps the knowledge base healthy and trustworthy.

## Setup
Same as `winnow-research` — Winnow MCP server must be configured with a valid API key and project ID.

## Usage
Invoke this skill when:
- Running a periodic knowledge base health check
- A chunk was referenced and found to be inaccurate
- After a major refactor or decision change
- Before a milestone to ensure context reflects current reality

**Trigger phrases:**
- "Review and rate the Winnow chunks for X"
- "Audit the context for X"
- "Clean up stale context about X"
- "Rate the quality of chunks tagged X"

## Workflow

### Step 1 — Identify chunks to review
Search for chunks in the target area:
```
search_context(query="<topic or area>", projectId="<project_id>", limit=10)
```
Or use a tag-based query to find all chunks needing review.

### Step 2 — Assess each chunk
For each chunk, read the full content:
```
read_context(id="<chunk_id>")
```

Evaluate against these criteria:
- **Accuracy** — Is the information correct as of today?
- **Relevance** — Is it still applicable to the current project state?
- **Clarity** — Is it well-written and understandable?
- **Completeness** — Does it cover the topic sufficiently?
- **Age** — When was it last updated? (check `get_context_versions`)

### Step 3 — Rate the chunk
Submit a review with a usefulness/correctness rating:
```
review_context(
  id="<chunk_id>",
  projectId="<project_id>",
  rating=<1-5>,
  comment="<assessment notes>"
)
```
Rating scale:
- **5** — Accurate, clear, highly useful
- **4** — Good but could use minor updates
- **3** — Partially useful, needs revision
- **2** — Mostly outdated or unclear
- **1** — Incorrect or irrelevant — should be updated or deleted

### Step 4 — Update stale chunks
For chunks rated 2-3, update with corrected content:
```
update_context(
  id="<chunk_id>",
  content="<corrected content>",
  projectId="<project_id>"
)
```

### Step 5 — Remove or supersede low-quality chunks
For chunks rated 1:
- If replaceable: write a new chunk, then delete the old one
- If not replaceable but wrong: update with a clear correction note

```
delete_context(id="<chunk_id>", projectId="<project_id>")
```

### Step 6 — Report
Summarize the review:
- Total chunks reviewed
- Rating distribution (5/4/3/2/1)
- Chunks updated / deleted
- Recommendations for follow-up

## Notes
- Never delete a chunk without reading it fully first
- When in doubt, update rather than delete — preserve history
- Schedule periodic reviews (monthly or per sprint)
- Chunks with rating 1-2 from multiple reviewers should be prioritized for cleanup
- Use `get_context_versions` to understand how a chunk evolved before rating it
