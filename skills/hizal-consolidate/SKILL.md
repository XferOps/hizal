---
name: hizal-consolidate
description: End-of-session consolidation — classify episodic write_memory chunks from the current session as KEEP (agent memory), PROMOTE (project knowledge), or DISCARD. Run this before ending any significant work session.
---

# Hizal Consolidate

Run this skill at the end of every significant work session to convert ephemeral notes into durable memory.

Use it for requests like:
- "Consolidate this session"
- "End-of-session cleanup"
- "Promote discoveries to project knowledge"
- When you see an `unconsolidated: true` flag at session start

## What This Is For

During a session, `write_memory` captures episodic notes — observations, discoveries, gotchas. These live in AGENT scope and fade into noise if left unreviewed.

Consolidation makes two decisions per chunk:

| Destination | When | Tool |
|---|---|---|
| **KEEP** as agent memory | Personal, interpretive, style-adjacent — applies to how *you* work | Leave as-is or refine with `write_memory` |
| **PROMOTE** to project knowledge | Factual, reusable, any agent would benefit | `write_knowledge` (PROJECT scope), then delete original |
| **DISCARD** | Redundant, already captured elsewhere, or no lasting value | Delete the chunk |

**Rule of thumb:** Would another agent benefit from knowing this? If yes → PROMOTE. If it's about how *you* think → KEEP. If it's a working note with no shelf life → DISCARD.

## Examples

**KEEP as agent memory:**
- "I tend to over-engineer auth — check with Parker before adding complexity"
- "Parker prefers flat file structures over nested directories"
- "I learn faster when I read the test file before the implementation"

**PROMOTE to project knowledge:**
- "The billing webhook handler must be idempotent — Stripe retries up to 3× on timeout"
- "ECS task def must be updated manually after Go module path changes"
- "The `chunk_type=SPEC` filter in Forge requires project_id, not team_id"

**DISCARD:**
- "Ran `git status` — clean"
- "Checked the PR — waiting on review"
- Notes already captured in an existing knowledge chunk

## Session Lifecycle

Start a Hizal session at the top of any consolidation task — see `hizal-onboard`. End it with `end_session` when done.

## Workflow

### 1. FETCH

Retrieve write_memory chunks written during this session:

```
search_context(
  query="session episodic memory",
  scope="AGENT",
  project_id="<project_id>",
  limit=50
)
```

Filter results to those with `created_at >= session_start` and `always_inject=false`. Identity and convention chunks are not consolidation candidates.

### 2. SEARCH (deduplication check)

For each candidate chunk, check if the content is already captured:

```
search_context(query="<chunk topic>", project_id="<project_id>", limit=5)
search_context(query="<chunk topic>", scope="AGENT", limit=5)
```

If a close match exists and the new chunk adds nothing → DISCARD.

### 3. DECIDE

For each chunk, assign one of: **KEEP** | **PROMOTE** | **DISCARD**

Apply the decision criteria above. When uncertain between KEEP and PROMOTE, ask: would another agent benefit? If yes → PROMOTE.

### 4. WRITE

Execute decisions in this order:

**PROMOTE** chunks:
```
write_knowledge(
  project_id="<project_id>",
  query_key="<descriptive-slug>",
  title="<what this teaches>",
  content="<the knowledge>"
)
```
Then delete the original write_memory chunk.

**KEEP** chunks:
Optionally refine the content if it was a rough note:
```
update_context(query_key="<key>", content="<refined version>")
```

**DISCARD** chunks:
Delete without writing anything.

### 5. COMPACT (optional)

If agent memory has grown noisy across sessions, run a secondary pass to merge related KEEP chunks into distilled long-term memories. See `hizal-compact` for guidance.

Only compact within AGENT scope during consolidation. Do not compact PROJECT scope here.

### 6. CLEAR FOCUS

If `register_focus` was called during the session, clear it:

```
clear_focus()
```

## Nudges You May See

**Start-of-session flag:** If activated context includes `unconsolidated: true`, you have un-reviewed write_memory chunks from a prior session. Run consolidation before starting new work.

**Mid-session nudge (SSE):** If you receive a `consolidation_nudge` event (typically at 10 chunks), consider a partial consolidation pass now rather than a large dump at end of session.

## Notes

- Never delete a chunk you haven't read.
- Err toward KEEP over DISCARD — lost context is worse than noisy context.
- A write_memory chunk that becomes write_knowledge is a success: it crossed the boundary from personal to shared.
- Consolidation complements `hizal-compact` — consolidate first (classify + promote), then compact (deduplicate + merge) if needed.
- Do not use `write_convention` during consolidation. Conventions require deliberate intent, not promotion from episodic notes.
