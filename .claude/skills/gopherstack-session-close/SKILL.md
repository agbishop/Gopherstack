---
name: gopherstack-session-close
description: Execute gopherstack's mandatory end-of-session close protocol completely and in order — file follow-up issues, run gates, close/update bd issues, then git pull --rebase, bd dolt push, git push, verify git status is clean. Use whenever a work session in this repo is ending, whenever asked to "wrap up", "close out", "finish this session", or before declaring work done. The single failure mode this exists to prevent is work that is committed but never pushed, or completed but never recorded in bd.
---

# gopherstack session close

Work is **not** complete until `git push` succeeds and `git status` shows up
to date with origin. Committed-but-unpushed work is stranded and the exact
failure this protocol exists to prevent. Never end a session on "ready to
push when you are" — push it yourself.

## Pre-flight checklist

1. Anything left to do? File a `bd` issue for it — don't leave it in your head or a comment.
2. Did code change? Run the gate subset for what changed (see `gopherstack-gates` skill for the decision table; short version below).
3. Any `bd` issues you worked finished, or in progress? Update their status.
4. Run the push sequence below, in order, without skipping steps.
5. Confirm `git status` literally says up to date with origin before reporting done.

## Ordered close sequence

```bash
# 1. File issues for anything unfinished (repeat per item)
bd create --title="<title>" --description="<desc>" --type=task --priority=<0-4>

# 2. Gates, only if code changed — see gate-subset table below

# 3. Update tracker state
bd update <id> --claim        # if you're taking something on
bd close <id> [--reason="<why>"]

# 4. MANDATORY push sequence — do not stop partway
git pull --rebase
bd dolt push
git push
git status                    # MUST show "up to date with origin"
```

If `git push` fails, resolve and retry until it succeeds. Do not leave the
session with unpushed commits, even if the failure looks like someone
else's problem (rebase conflict, stale branch, etc.) — resolve it.

## Gate subset by what changed

| what changed | run before closing |
|---|---|
| Nothing (pure investigation/planning) | none — but still push any bd issue changes |
| One service's Go code | `go test -race ./services/<svc>/...` |
| Shared `pkgs/` code | `go build ./...` plus tests for every consumer you touched |
| Anything you intend to open a PR from | full gate: `make lint && make test && make build-linux && make integration-test` |

See the `gopherstack-gates` skill for the full decision table and linter
gotchas — don't re-derive it here.

## bd command reference

| command | use |
|---|---|
| `bd ready` | find available work |
| `bd list --status=open\|in_progress` | survey tracker state |
| `bd show <id>` | view issue details |
| `bd update <id> --claim` | claim work |
| `bd close <id...> [--reason=]` | complete work |
| `bd create --title= --description= --type=task\|bug\|feature --priority=0-4` | file new work |
| `bd dep add <issue> <depends-on>` | record a dependency |
| `bd stats` | tracker-wide summary |
| `bd doctor` / `bd doctor --check=conventions` | health check |
| `bd stale` / `bd orphans` | find neglected/unlinked issues |
| `bd preflight` | pre-PR checks |
| `bd remember "insight"` / `bd memories <keyword>` / `bd forget <key>` | persistent knowledge — use instead of MEMORY.md files |
| `bd human <id>` | flag an issue for human decision |
| `bd prime` | reload full bd context after compaction |
| `bd dolt push` | push the Dolt-backed tracker DB to remote |

Use `bd` for ALL task tracking in this repo — never TodoWrite, TaskCreate, or
markdown TODO lists. Use `bd remember` instead of a MEMORY.md file.

**Landmine: never run `bd edit`.** It opens `$EDITOR` and blocks the agent
indefinitely with no way to escape non-interactively.

## Commit convention

Conventional Commits with a scope — the scope is the service package name
or subsystem (`bd`, `deps`, `parity`, `lint`, `sdkcheck`, `release`, or a
service name). Real examples from `git log`:

```
feat(directconnect): <summary>
fix(dynamodb): <summary>
test(sdkcheck): <summary>
build(deps): <summary>
chore(bd): <summary>
docs(parity): <summary>
refactor: <summary>
ci: <summary>
```

Merged PRs carry a trailing `(#NNNN)`. `.git/hooks/` has only stock
`.sample` files — no active pre-commit/pre-push hooks — so nothing local
blocks a bad commit; CI's required status checks are the real gate.

## Clean up before handoff

- Clear any stashes you created this session.
- Prune merged/stale remote-tracking branches if you created any.
- Leave a short handoff note (what shipped, what's still open in `bd`,
  anything a human should decide — use `bd human <id>` for the latter
  rather than burying it in prose).
