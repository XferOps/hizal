# Hizal

**A semantic memory layer that doesn't just store what agents know — it modulates how they think.**

[![CI](https://github.com/XferOps/hizal/actions/workflows/ci.yml/badge.svg)](https://github.com/XferOps/hizal/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

---

🧪 Try it: [hizal.ai](https://hizal.ai) • 🚧 TUI (coming soon): [hizal-tui](https://github.com/parkerscobey/hizal-tui)

> You remember the work you do and learn from your mistakes. Why should your AI Agents be any different?

## Introduction

The Hizal (mycorrhizal) Go API exposes both REST and MCP (HTTP+SSE) that gives any agent the ability to write what they learn. Future agent sessions reuse the stored knowledge. Context is deterministically injected instead of evaporating.

### Context Scopes

Hizal has three scopes — _ORG_, _PROJECT_, and _AGENT_ — they're not visibility filters. They're an ownership model.

The scope determines who writes it, who benefits from it, and how it injects. An org principle loads for every agent. A project convention loads for agents on that project. An agent memory stays private.

---

### Chunks and Chunk Types

Hizal stores structured **_context chunks_** -- semantically-embedded text with versioning.

**_Chunks_** are not stored as undifferentiated blobs, but as **_chunk types_** that are customizable and extendable.

**Built-in chunk types:**
| Chunk Type     | Default Scope | Injection Settings                | Description                                                                                                  |
| -------------- | ------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| IDENTITY       | Agent         | All sessions for the owning agent | Agent's personality, traits, and core identity. Automatically included whenever this agent starts a session. |
| CONVENTION     | Project       | All sessions on the project       | Coding standards and patterns. Any agent working on the project gets these as ambient context.               |
| PRINCIPLE      | Org           | All sessions in the org           | Org-wide non-negotiable truths. Every agent in the org receives these in every session.                      |
| CONSTRAINT     | Project       | All sessions on the project       | Hard limits and non-negotiable requirements. Force-fed alongside conventions to every agent on the project.  |
| MEMORY         | Agent         | None (search only)                | Episodic observations, conversation history. Private to the agent, surfaces only when searched for.          |
| KNOWLEDGE      | Project       | None (search only)                | Facts, architecture, and established patterns. Agents pull what's relevant to their current task.            |
| DECISION       | Project       | None (search only)                | Past decisions with reasoning. Found when agents need context on a topic they're about to make a call on.    |
| RESEARCH       | Project       | None (search only)                | Investigation findings and exploration notes.                                                                |
| PLAN           | Project       | None (search only)                | Task breakdowns, approaches, and planned work.                                                               |
| SPEC           | Project       | None (search only)                | Requirements and specification documents.                                                                    |
| IMPLEMENTATION | Project       | None (search only)                | Code-level notes and implementation documentation.                                                           |
| LESSON         | Project       | None (search only)                | Post-mortems and learned lessons.                                                                            |

---

### Deterministic Context Injection Settings

Hizal brings disjunctive normal form to your context window. You define _who_ gets a chunk and _when_.

Chunk types define and individual chunks inherit or override the `inject_audience` — a targeting spec that determines whether the chunk is automatically injected into a session's context window at startup. No search required, no retrieval step. If the rule matches, the chunk is there.

**The rules follow DNF (disjunctive normal form):** an array of rules where any single rule matching is sufficient (OR), and within each rule, all conditions must be met (AND).

```json
{
  "inject_audience": {
    "rules": [
      { "agent_types": ["dev"], "project_ids": ["proj-abc"] },
      { "agent_ids": ["agent-xyz"] }
    ]
  }
}
```
This reads: inject this chunk into sessions where the agent is a **dev type AND working on project proj-abc**, OR where the agent is **specifically agent-xyz**. Two rules, OR'd together. Conditions within each rule are AND'd.

**Available predicates:**
| Predicate         | Matches on                                       |
| ----------------- | ------------------------------------------------ |
| `all`             | Every session. The broadest possible audience.   |
| `agent_ids`       | Specific agents by ID                            |
| `agent_types`     | Agent type (e.g. `dev`, `qa`, `orchestrator`)    |
| `project_ids`     | Specific projects by ID                          |
| `org_ids`         | Specific orgs by ID                              |
| `agent_tags`      | Tags assigned to the agent                       |
| `focus_tags`      | Tags registered for the current session focus    |
| `lifecycle_types` | Session lifecycle type                           |

Injection is deterministic. Retrieval is optional. Chunks with `inject_audience` are pushed into matching sessions automatically — agents don't need to search for them and can't miss them. Chunks without `inject_audience` are still available via `search_context`, but they require the agent to ask.

---

### Custom Agent Types

Agent types are a role primitive. A _dev_ agent gets coding tools and project conventions. A _research_ agent gets search tools and research chunks. An _orchestrator_ gets project management tools and org-wide context. Each type ships with a curated tool surface, injection defaults, and session lifecycle behavior — and **orgs can define their own types** that inherit from these or start fresh.

It's RBAC for AI agents — with the behavioral defaults baked in.

**Built-in agent types:**
| Agent Type   | Injected Chunk Types                                 | Injected Scopes     | Search Scopes       | Exclusive Tools                                                                               | Description                                                                                                                                                                                  |
| ------------ | ---------------------------------------------------- | ------------------- | ------------------- | --------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Dev          | Knowledge, Convention, Identity, Principle, Decision | Agent, Project, Org | Agent, Project, Org | —                                                                                                                       | Coding agents (Claude Code, Cursor, etc.). Gets the full picture — project conventions, decisions, and identity are all in the context window at session start.                              |
| Orchestrator | Knowledge, Convention, Identity, Principle, Decision | Agent, Project, Org | Agent, Project, Org | `store_principle`, `create_project`, `list_agents`, `add_agent_to_project`, `remove_agent_from_project` | Long-running coordination agents (OpenClaw). Same context as dev, plus project management and principle-setting tools that other types can't access. The human interface into the dev cycle. |
| Admin        | Knowledge, Identity, Principle                       | Agent, Org          | Agent, Org          | `store_principle`                                                                                                       | Business and ops agents. No project-scoped injection — they don't see code conventions or project decisions. Org-level context only.                                                        |
| Research     | Identity                                             | Agent               | Agent, Project, Org | —                                                                                                                       | Investigation agents. Minimal injection — just their identity. Full search access so they can pull what they need, but they start lean to avoid polluting exploratory work with assumptions. |

---

### Sessions

Sessions are first-class objects, not an afterthought. They track the full arc of an agent's work — what was injected, what was searched, what was written — and gate the transition from ephemeral observations to durable knowledge.

```
start_session(lifecycle_slug="dev")
│
├─ Identity, conventions, and principles injected automatically
├─ register_focus(task="HIZAL-42: billing webhooks", project_id="...")
│
├─ During work:
│   ├─ search_context → find existing knowledge
│   ├─ write_knowledge → share project facts
│   └─ write_memory → record personal observations
│
└─ end_session(session_id="...")
    └─ Returns MEMORY/RESEARCH/PLAN chunks for review and promotion
```
`start_session` does the heavy lifting: it resolves the agent's type, selects the lifecycle preset, and deterministically injects all matching inject_audience chunks before the agent writes a single line of code. `end_session` surfaces ephemeral chunks so they can be curated into durable knowledge by the agent.

One active session per agent, enforced at the database level. Sessions track metrics — chunks read, chunks written, resume count — so you can see how agents actually use context over time.

### Session Lifecycles

Lifecycles are governance presets that shape how sessions behave. TTL, required steps, consolidation thresholds, injection scopes — all configurable per lifecycle, per org.

| Lifecycle    | TTL | Required Steps | Consolidation Threshold | Inject Scopes       | Description                                                                                      |
| ------------ | --- | -------------- | ----------------------- | ------------------- | ------------------------------------------------------------------------------------------------ |
| Default      | 8h  | —              | 5 chunks                | Agent, Project, Org | General-purpose. No required steps, full injection. Use when no other preset fits.               |
| Dev          | 8h  | `register_focus` | 3 chunks                | Agent, Project, Org | Coding sessions. Requires explicit task declaration before writing.                              |
| Admin        | 4h  | `register_focus` | 2 chunks                | Agent, Org          | Ops and business sessions. Shorter TTL, no project-scoped injection.                             |
| Orchestrator | 24h | —              | 10 chunks               | Agent, Project, Org | Long-running coordination. Extended TTL for agents that steer subagents across a full dev cycle. |

`required_steps` enforces workflow discipline — a dev session won't accept writes until the agent declares what it's working on via `register_focus`. `consolidation_threshold` controls how many ephemeral chunks (memory, research, plans) trigger the end-of-session consolidation prompt. **Orgs can define their own lifecycles** that inherit from these or start fresh.

---

## Quickstart

### Self-host

See [`SELF_HOSTING.md`](./SELF_HOSTING.md) for Docker and bare-metal setup.

### Hosted

[api.hizal.ai](https://api.hizal.ai) — sign up for an API key at [hizal.ai](https://hizal.ai)

### Connect your agent

Add to your MCP config (Claude Code, Cursor, OpenCode, or any MCP client):

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

```python
# Start a session — identity, conventions, principles load automatically
start_session(lifecycle_slug="dev")

# Search existing knowledge
search_context(query="how does auth work", project_id="...")

# Write what you learned
write_knowledge(
  query_key="auth-flow",
  title="JWT verification in middleware",
  content="The auth middleware validates JWTs by...",
  project_id="..."
)

# End session — MEMORY chunks returned for review
end_session(session_id="...")
```

Next session — knowledge is still there. Identity loads automatically. Conventions are always in context.

---

## Architecture

```
┌────────────┐      ┌────────────┐      ┌──────────────────┐
│ Your Agent │─MCP─▶│  Hizal API │─────▶│   PostgreSQL     │
│            │      │    (Go)    │      │   + pgvector     │
└────────────┘      └────────────┘      └──────────────────┘
                          │
                    ┌─────▼─────┐
                    │  OpenAI   │
                    │ Embeddings│
                    └───────────┘
```

- **Go API** with MCP server (HTTP+SSE transport)
- **PostgreSQL 16** with pgvector for semantic search
- **text-embedding-3-small** for embeddings ($0.02/1M tokens)
- **No server-side LLM** — agents do all reasoning client-side

Every feature is a composable primitive first. MCP tools like `search_context` and `write_knowledge` are the building blocks. Agent skills (`hizal-research`, `hizal-plan`, etc.) are guided workflows composed from those primitives. The UI is a dashboard for humans over the same data. Power users access primitives directly. Nobody outgrows the product.

---

## Agent Skills

**Pre-built workflows composed from MCP primitives:**
| Skill          | Purpose                                               |
| -------------- | ----------------------------------------------------- |
| hizal-seed     | Populate a new project with foundational context      |
| hizal-research | Investigate a topic, fill knowledge gaps              |
| hizal-plan     | Create implementation plans validated against context |
| hizal-compact  | Merge overlapping chunks into cleaner summaries       |
| hizal-review   | Rate and improve context quality                      |
| hizal-onboard  | Get up to speed on a project fast                     |

Skills live in `skills/` — each has a `SKILL.md` with full workflow instructions.

---

## Documentation

| Doc | Contents |
|-----|---------|
| [`SELF_HOSTING.md`](./SELF_HOSTING.md) | Self-hosting guide |
| [`docs/01-problem-sources.md`](./docs/01-problem-sources.md) | Problem statement and research |
| [`docs/02-architecture.md`](./docs/02-architecture.md) | System design and data model |
| [`docs/03-mcp-tools.md`](./docs/03-mcp-tools.md) | MCP tool reference |
| [`docs/04-skills.md`](./docs/04-skills.md) | Agent skill specifications |
| [`docs/05-workflows.md`](./docs/05-workflows.md) | Session lifecycle and workflows |
| [`docs/06-agent-onboarding.md`](./docs/06-agent-onboarding.md) | Agent provisioning guide |
| [`docs/api-reference.md`](./docs/api-reference.md) | REST API reference |
| [`CONTRIBUTING.md`](./CONTRIBUTING.md) | How to contribute |
| [`AGENTS.md`](./AGENTS.md) | Complete dev agent session walkthrough |

---

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0 — see [`LICENSE`](./LICENSE).

---

Built by [XferOps](https://xferops.com). We run a team of AI agents building software. Hizal is how they remember.

---

## The Name

Hizal is derived from **mycorrhizal** — the symbiotic fungal networks that thread through forest soil, connecting the root systems of trees across entire ecosystems.

These networks aren't passive. Trees actively use them: sharing carbon with saplings that can't reach the light, sending chemical warnings when pests attack, routing nutrients to struggling neighbors. The forest thinks through the network. Individual trees survive because the network remembers.

The parallel to AI agents is deliberate. Each agent session is a tree — capable on its own, but isolated. Hizal is the network underneath: persistent, shared, searchable knowledge that lets agents build on each other's work instead of starting from scratch every time.

The "-hizal" suffix is pulled directly from mycorrhizal. It felt right. Memory that connects things should sound like it.
