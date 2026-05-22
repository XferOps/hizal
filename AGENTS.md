# AGENTS.md — Hizal Dev Agent Operating Procedures

You are a dev agent working on the Hizal codebase (Go API + React/Vite frontend).
This file tells you how to work here. Read it fully before doing anything else.

Hizal is both the product you're building and the memory system you use to build it.

---

## Hizal Context System

Every dev session starts and ends with Hizal.

Use the Hizal skills (in `skills/`):

1. **Start** → `hizal-start` skill — begin session, register focus
2. **Read the spec** → see "Task Specs" below
3. **Search** → `hizal-search` skill — find existing context before building
4. **Write** → `hizal-write` skill — persist decisions as you build
5. **End** → `hizal-end` skill — close session, review surfaced chunks

### Project-Specific Context

- **Lifecycle slug:** `dev`
- **Hizal project ID:** `d93a8d80-c6e6-43ea-b871-528e3399db3a`
- **Forge project ID:** `cmmhg1y1f0001le01gkx2a3sk`

Pass these to the Hizal skills when starting sessions and writing chunks.

---

## Task Specs

**In our setup**, specs come from Forge via the forge MCP:

```
forge_get_task(taskId="<ticket-id>")
```

Hizal Forge tickets live in project `cmmhg1y1f0001le01gkx2a3sk` and use the `HIZAL-###` prefix.
If direct search by full ticket id fails, search within that project by number or title, or list tasks
for that project and locate the ticket there.

The ticket description is the spec. Read it fully before writing code.

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
