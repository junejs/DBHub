---
name: multica-usage
description: Sync the role agents under the repo's `docs/agents/` directory (DBHUB Project Manager, Solution Design, Implementation, Test, CodeReview) into the current Multica workspace — creating or updating Multica agents so they can be assigned issues and run tasks. Use this whenever the user wants to upload, publish, push, register, deploy, or sync the `agent_team` agents (or any `*_AGENT.md` file) into Multica, recreate the DBHUB agent team in a fresh workspace, or make the agents "show up / become available" in Multica. Also use it when the user asks to keep the workspace's agents in sync after editing an agent markdown file.
---

# Multica agent upload

Upload (create-or-update) every agent in `docs/agents/` into the **current Multica workspace** as a Multica agent, so each one can be assigned issues and invoked like a teammate.

## What "upload" means here

Each `*_AGENT.md` file in `docs/agents/` is a set of agent instructions. The equivalent Multica resource is an **agent** whose `instructions` field is the file's contents. "Uploading" = making sure a Multica agent exists for each file, with its `instructions` matching the file, plus a stable `name` and `description`.

This is **idempotent**: re-running it only updates `instructions`/`description` for agents that already exist, and creates the rest. It never deletes or archives anything.

## Prerequisites

- The `multica` CLI is on PATH and authenticated (run `multica auth whoami` / `multica workspace list` to confirm). It targets the "current workspace" — the one selected by the active profile or `MULTICA_WORKSPACE_ID`.
- `jq` is installed (`brew install jq` if not).
- Run from the **repository root**, so `./docs/agents` resolves. (The repo is `dbhub-requirement`.)

## How to upload

Just run the bundled script from the **repository root**. A dry-run first is good hygiene — it prints the exact `multica` commands without touching the workspace:

```bash
bash .agents/skills/multica-usage/scripts/upload_agents.sh --dry-run
```

If those look right, run it for real:

```bash
bash .agents/skills/multica-usage/scripts/upload_agents.sh
```

Output shows `+ create <name>` / `↻ update <name> (<id>)` per agent, then a summary like `done: 3 created, 2 updated, 0 failed`.

## What gets uploaded (the manifest)

The mapping `file → agent name → description` lives in [`agents.tsv`](agents.tsv) (TAB-separated), next to this SKILL.md. It is the single source of truth for which files are uploaded:

| file | multica agent name | description |
|---|---|---|
| `PM_AGENT.md` | DBHUB Project Manager | 方案与执行之间的调度层：拆 issue 树、分配、跟踪推进闭环 |
| `SOLUTION_DESIGN_AGENT.md` | DBHUB Solution Design | 把需求转化为可执行方案设计（数据结构 / API / 技术方案） |
| `IMPLEMENTATION_AGENT.md` | DBHUB Implementation | 全栈实现：契约→Go 后端 + React/TS 前端 + 单测，一个 PR |
| `TEST_AGENT.md` | DBHUB Test | 功能与集成测试：端到端链路 / 越权 / 边界 / 验收 |
| `CODEREVIEW_AGENT.md` | DBHUB CodeReview | 架构守门人：拦截 CI 全绿却暗藏的安全洞与技术债 |

Matching is by **name**. If an agent with that name already exists in the workspace, it's updated; otherwise it's created.

## Runtime & visibility (the defaults)

The script resolves sensible defaults so you usually don't need to think about them:

- **Runtime**: defaults to the first **online, local** runtime (`multica runtime list`). Override with `--runtime-id <ID>` or the `MULTICA_RUNTIME_ID` env var. It is only set when **creating** an agent — once an agent exists, the script leaves its runtime alone (you may have intentionally pointed it elsewhere).
- **Visibility**: `workspace`, so other teammates and agents (e.g. the Project Manager dispatching issues) can invoke them. Override with `--visibility private` if you want owner-only agents.

Other flags: `--source-dir DIR` (default `./docs/agents`), `--only NAME` (upload a single agent), `--dry-run`, `--help`.

## Adding or changing an agent

1. Edit the `*_AGENT.md` file under `docs/agents/` (or add a new one).
2. If it's new, add a row to `agents.tsv`: `file<TAB>Name<TAB>short description`.
3. Re-run the script. Existing agents get their `instructions` updated; new ones get created.

## Notes / gotchas

- The instructions are passed through a shell variable, so backticks and `$VAR`-like tokens inside the markdown are uploaded **literally** — they are never executed by the shell. Don't switch to an inline `--instructions "$(cat file)"` form without keeping this property.
- The script writes JSON to stdout on success (suppressed) and lets stderr through on failure. A non-zero exit means at least one agent failed; the summary line tells you how many.
- This only ever creates/updates. To remove an agent that's no longer in the manifest, archive it manually: `multica agent archive <id>`.
