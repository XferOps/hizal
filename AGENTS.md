# AGENTS.md — Hizal Dev Agent Operating Procedures

You are a dev agent working on the Hizal codebase (Go API + React/Vite frontend).
This file tells you how to work here. Read it fully before doing anything else.

Hizal is both the product you're building and the memory system you use to build it.

---

## Your First Three Steps (always, no exceptions)

1. **Start a Hizal session**
2. **Read the task spec from Forge MCP (Hizal project_id `cmmhg1y1f0001le01gkx2a3sk`)**
3. **Search Hizal for existing context on the task**

Only then start writing code.

---

## 1. Start a Hizal Session

Every dev session starts and ends with Hizal.

```
start_session(lifecycle_slug="dev")
```

This returns a `session_id`. Keep it visible — you'll need it for `register_focus` and `end_session`.

Then register what you're working on:

```
register_focus(
  session_id="<session-id>",
  task="HIZAL-XX: <ticket title>",
  project_id="<project-id>"
)
```

### Session Recovery

If you lose your `session_id` (context reset, compaction):

```
get_active_session()
```

- `status="active"` → use the returned `session_id`, call `resume_session` to extend TTL
- `status="none"` → call `start_session` to begin fresh

---

## 2. Read the Task Spec

**In our setup**, specs come from Forge via the forge MCP:

```
forge_get_task(taskId="<ticket-id>")
```

Hizal Forge tickets live in project `cmmhg1y1f0001le01gkx2a3sk` and use the `HIZAL-###` prefix.
If direct search by full ticket id fails, search within that project by number or title, or list tasks
for that project and locate the ticket there.

The ticket description is the spec. Read it fully before moving to step 3.

---

## 3. Search Hizal for Existing Context

Now that you know what you're building, search Hizal broadly first, then narrow if needed.

`search_context` can search across all accessible scopes by default:

- `AGENT` — your personal memory and prior investigations
- `PROJECT` — Back Office knowledge and conventions
- `ORG` — org-wide standards and principles

Start with 2-3 broad searches using different phrasings:

```
search_context(query="<key concept from the spec>")
search_context(query="<ticket id or feature name>")
search_context(query="<related subsystem or endpoint>")
```

Then narrow when you need a specific layer of context:

```
# Project-specific knowledge and conventions
search_context(
  query="<key concept from the spec>",
  project_id="d93a8d80-c6e6-43ea-b871-528e3399db3a",
  scope="PROJECT"
)

# Prior agent memory / investigation notes
search_context(
  query="<key concept from the spec>",
  scope="AGENT",
  chunk_type="MEMORY"
)

# Org-wide principles and standards
search_context(
  query="<key concept from the spec>",
  scope="ORG"
)
```

If you know the exact saved item you're looking for, search by `query_key`.

Examples:

```
search_context(query="<key concept from the spec>", project_id="d93a8d80-c6e6-43ea-b871-528e3399db3a")
search_context(query_key="<exact-query-key>", project_id="d93a8d80-c6e6-43ea-b871-528e3399db3a")
```

Run 2-3 searches with different phrasings. Read the returned chunks — they contain
architecture decisions, conventions, and prior work that must inform your implementation.

If an `AGENT` memory chunk turns out to be broadly useful for the team, promote it later by
writing it back as `write_knowledge` or `write_convention`.

Don't rediscover what the team already decided.

---

## Writing Code

### Branch first, always

Before writing a single line of code:

```bash
git fetch origin main
git checkout -b feat/<ticket-id-lowercase>-<short-description> main
# e.g. feat/hizal-146-password-strength-validation
```

This repo commonly uses **git worktrees**. `main` may already be checked out in another worktree,
so do not assume `git checkout main` will succeed. If your current worktree already points at the
same commit as `main`, branch from the current `HEAD`. Otherwise branch from fetched `main`
without trying to switch the other worktree.

**Never commit directly to main.** If you realize you've committed to main, stop —
create a branch from your current HEAD and reset main before pushing.

### Stack

- **Go 1.23+** — API server (`internal/`)
- **PostgreSQL 16** with pgvector — embeddings on `context_chunks`
- **Migrations** in `internal/db/migrations/` (sequential: `NNN_name.up.sql` / `NNN_name.down.sql`)

### Conventions

- API handlers in `internal/api/`
- Models in `internal/models/models.go` (canonical package for DB types)
- MCP tools in `internal/mcp/`
- New routes wired in `internal/api/router.go` under the appropriate auth group
- Write at least one test for every new handler or MCP tool
- `go build ./...` and `go test ./...` must be green before opening a PR

### Build check

```bash
go build ./...
go vet ./...
go test ./... -race -timeout 60s
```

---

## Write to Hizal As You Build

This is not optional. Write chunks as you make decisions — not just at the end.

| What you're writing | Tool | Scope |
|---------------------|------|-------|
| Architecture or design decision | `write_knowledge` | PROJECT |
| Convention this codebase follows | `write_convention` | PROJECT (always_inject) |
| Something personal you learned | `write_memory` | AGENT |

**Do not use `write_context`** — it's deprecated. Use the purpose-built tools above.

Write one chunk per meaningful decision. Don't batch everything into one chunk at the end.

---

## Open the PR

**Your session is not complete until a PR exists.** Tests passing and code written is not done.
Done means: branch pushed, PR open, reviewers requested.

```bash
gh pr create \
  --repo parkerscobey/hizal \
  --title "feat(HIZAL-XX): <description>" \
  --body "## Summary\n\n<what you built>\n\n## Testing\n\n<what you ran>\n\n---\n**Forge ticket:** [HIZAL-XX](https://forge.xferops.dev/projects/cmmhg1y1f0001le01gkx2a3sk) — <ticket title>"

gh pr edit --repo parkerscobey/hizal --add-reviewer parkerscobey
```

Always request review from `parkerscobey`.

After pushing fixes to address review feedback, **re-request review**:

```bash
gh api repos/parkerscobey/hizal/pulls/<PR#>/requested_reviewers \
  -X POST -f 'reviewers[]=parkerscobey'
```

---

## End Your Session

After the PR is open and the Forge spec is updated:

```
end_session(session_id="<session-id>")
```

Review the returned MEMORY chunks. For each one, decide:
- **Keep** — useful personal observation, leave as AGENT memory
- **Promote** — valuable for the team, call `write_knowledge` with the content
- **Discard** — noise, ignore it

This is how knowledge compounds across agents and sessions.

---

## Creating New Specs

When you discover work that needs doing (bugs, improvements, missing features):

Specs live in **Forge**, not in Hizal chunks. Create a new Forge task in the Hizal project backlog.

Hizal Forge project:
- Project ID: `cmmhg1y1f0001le01gkx2a3sk`
- Backlog column ID: `cmmhg1y1f0002le01a4uwj2hs`

```
forge_create_task(
  projectId="cmmhg1y1f0001le01gkx2a3sk",
  columnId="cmmhg1y1f0002le01a4uwj2hs",
  title="Short spec title",
  description="<full spec / problem / fix / files>",
  type="TASK",   # or BUG / STORY
  priority="MEDIUM"
)
```

Search existing specs to find the next available number:

```
forge_search_tasks(query="HIZAL", projectId="cmmhg1y1f0001le01gkx2a3sk")
```

If search is incomplete, list all tasks for the project and inspect the highest existing `HIZAL-###`
ticket number before creating a new one.

The Forge task description is the spec. Write the full problem statement, proposed fix, and any
relevant files or constraints there.

---

## The Principle

The prompt that kicked off your session is just a door opener.
Everything else — the spec in Forge, and the conventions and prior decisions in Hizal — should shape the work.
Read those first. Code second.
