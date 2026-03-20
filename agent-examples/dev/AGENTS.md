# AGENTS.md — [Your Project] Dev Agent

You are a dev agent working on [Your Project].
This file tells you how to work here. Read it fully before doing anything else.

Hizal is your memory system. Every convention, architectural decision, and lesson your team has learned lives there. Before writing a line of code, you search it. As you work, you write back to it.

---

## Setup (fill this in before deploying agents)

| Key | Value |
|-----|-------|
| **Hizal project ID** | `YOUR_PROJECT_ID` |
| **Hizal API** | `https://your-hizal-instance/mcp` (or `https://winnow-api.xferops.dev`) |
| **Lifecycle** | `dev` |

### MCP Configuration

Add Hizal to your agent's MCP config:

```json
{
  "mcpServers": {
    "hizal": {
      "url": "https://your-hizal-instance/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_API_KEY"
      }
    }
  }
}
```

---

## Your First Three Steps (always, no exceptions)

1. **Start a Hizal session**
2. **Read your assigned task**
3. **Search Hizal for existing context on the task**

Only then start writing code.

---

## 1. Start a Hizal Session

```
start_session(lifecycle_slug="dev")
```

This returns a `session_id`. Keep it — you'll need it for `register_focus` and `end_session`.

Then register what you're working on:

```
register_focus(
  session_id="<session-id>",
  task="<ticket ID>: <task title>",
  project_id="YOUR_PROJECT_ID"
)
```

When `start_session` returns, your identity, project conventions, and org principles are
already in context — injected automatically. You don't need to go find them.

### Session Recovery

If you lose your `session_id` (context reset, compaction):

```
get_active_session()
```

- `status="active"` → use the returned `session_id`, call `resume_session` to extend TTL
- `status="none"` → call `start_session` to begin fresh

---

## 2. Read Your Assigned Task

Your team's task source (replace with your actual setup):

```bash
# If using Forge:
forge_get_task(taskId="<ticket-id>")

# If using GitHub Issues:
gh issue view <issue-number>

# If specs live in Hizal:
search_context(query="spec status TODO", project_id="YOUR_PROJECT_ID")
read_context(query_key="spec-<ticket-id>-<short-name>", project_id="YOUR_PROJECT_ID")
```

Read the task description fully. Extract:
- What needs to be built or fixed
- Acceptance criteria
- Dependencies or blockers
- Any explicit constraints

The task description is your contract. If something is unclear, write a `write_memory` note
and flag it — don't silently assume.

---

## 3. Search Hizal for Existing Context

Now that you know what you're building, search for prior decisions:

```
search_context(query="<key concept from your task>", project_id="YOUR_PROJECT_ID")
```

Run 2–3 searches with different phrasings. Examples:
- `search_context(query="auth middleware how it works")`
- `search_context(query="database migration conventions")`
- `search_context(query="error handling patterns")`

Read the returned chunks. They contain:
- Architecture decisions and the reasoning behind them
- Conventions you must follow
- Gotchas and lessons from previous work
- Existing patterns to reuse

**Don't rediscover what the team already decided.** If there's a knowledge chunk covering your area, start from it — don't start from scratch.

---

## Writing Code

### Branch first, always

Before writing a single line:

```bash
git checkout main && git pull
git checkout -b feat/<ticket-id>-<short-description>
# e.g. feat/iss-42-user-auth
```

**Never commit directly to main.** If you've committed to main accidentally, stop immediately — create a branch from HEAD and reset main before pushing.

### Build check (adapt to your stack)

```bash
# Your project's equivalent:
make test       # or: go test ./... / npm test / pytest / etc.
make build      # or: go build / npm run build / etc.
```

Both must be green before opening a PR. No exceptions.

---

## Write to Hizal As You Build

This is not optional. Write chunks as you make decisions — not just at the end.

| What you're writing | Tool | Scope |
|---------------------|------|-------|
| Architecture or design decision | `write_knowledge` | PROJECT |
| Convention this codebase follows | `write_convention` | PROJECT (always injected) |
| Something personal you learned | `write_memory` | AGENT |
| Org-wide pattern or standard | `write_org_knowledge` | ORG |

**One chunk per meaningful decision.** Don't batch everything into one chunk at the end —
decisions made at the start of a task are often the most important ones and get lost.

### Examples

```
write_knowledge(
  query_key="auth-jwt-verification",
  title="JWT verification in middleware",
  content="Auth middleware validates JWTs at the gateway layer before requests reach handlers. Tokens are RS256-signed. The public key is loaded from JWKS_URI on startup and cached. Never validate tokens in individual handlers.",
  project_id="YOUR_PROJECT_ID"
)

write_memory(
  query_key="lesson-silent-401-on-missing-tenant",
  title="Lesson: /v1/context silently 401s without tenant resolver",
  content="Spent 30min debugging 401s on write_context. The tenant resolver middleware is required — if you bypass it (e.g. in test setup), the handler returns 401 with no explanation. Always use the full middleware chain in integration tests.",
  project_id="YOUR_PROJECT_ID"
)
```

---

## Open the PR

**Your task is not done until a PR exists.** Build passing and code written is not done.
Done means: branch pushed, PR open, reviewer requested.

```bash
gh pr create \
  --title "feat(<ticket-id>): <description>" \
  --body "## Summary

<what you built and why>

## Testing

<what you ran and results>

## Notes

<anything reviewers should know>

---
**Ticket:** <link to your task>"

gh pr edit --add-reviewer <your-reviewer>
```

After addressing review feedback, re-request review — don't assume the reviewer will notice:

```bash
gh api repos/<org>/<repo>/pulls/<PR#>/requested_reviewers \
  -X POST -f 'reviewers[]=<reviewer>'
```

---

## Update Your Task

After opening the PR, update the task status and link the PR so the team knows where things stand.

```bash
# Forge:
forge_move_task(taskId="<ticket-id>", columnId="<code-review-column-id>")
forge_create_comment(taskId="<ticket-id>", content="PR opened: <url>")

# GitHub Issues:
gh issue comment <issue-number> --body "PR: <url>"
```

If you're using Hizal-based specs, update the spec chunk:

```
update_context(
  query_key="spec-<ticket-id>-<short-name>",
  project_id="YOUR_PROJECT_ID",
  content="<spec content with Status: CODE_REVIEW and PR: <url>>",
  change_note="PR opened: <url>"
)
```

---

## End Your Session

After the PR is open and the task is updated:

```
end_session(session_id="<session-id>")
```

Review the returned chunks. For each one, decide:

- **Keep** — useful personal observation, leave it as AGENT memory
- **Promote** — valuable for the whole team, call `write_knowledge` with the content
- **Discard** — noise, ignore it

This is how knowledge compounds. Each session end is a small act of institutional memory-building.
Do it deliberately.

---

## Creating New Tasks

When you discover work that needs doing (bugs, improvements, missing features), create a ticket
so it doesn't get lost:

```bash
# Forge:
forge_create_task(
  projectId="<forge-project-id>",
  columnId="<backlog-column-id>",
  title="<descriptive title>",
  description="<what needs doing, why, acceptance criteria>",
  priority=MEDIUM,
  type=TASK
)

# GitHub Issues:
gh issue create --title "<title>" --body "<description>"
```

Don't just write a TODO comment in code and move on.

---

## The Principle

The prompt that kicked off your session is just a door opener.

Everything else — the task spec, the conventions, the architectural decisions, the lessons from
previous work — lives in Hizal. Search it first. Code second.

Your work isn't done when tests pass. It's done when:
1. The PR is open
2. The task is updated
3. What you learned is written back to Hizal

That's the loop. Close it every session.
