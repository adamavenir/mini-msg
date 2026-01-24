---
id: fray-7mm
status: closed
deps: [fray-3vp]
links: []
created: 2025-12-19T11:57:26.505276-08:00
type: task
priority: 1
---
# P1.5: mm prune with cold storage

Implement mm prune with guardrails and cold storage.

Prune flow:
1. Check uncommitted changes: git status --porcelain .mm/
   - If dirty: Error "Uncommitted changes in .mm/. Commit first."
2. Check remote sync (if remote exists):
   - git rev-list --count @{u}..HEAD (ahead)
   - git rev-list --count HEAD..@{u} (behind)
   - If ahead/behind: Error "Branch not synced. Push/pull first."
   - If no remote: Skip this check (offline OK)
3. Archive messages:
   - Append all messages.jsonl → history.jsonl
   - Keep last N messages in messages.jsonl (default 100)
4. Rebuild SQLite from messages.jsonl
5. Success: "Pruned to last N messages. Archived to history.jsonl"

Flags:
- mm prune (default: keep 100)
- mm prune --keep 50
- mm prune --all (delete history.jsonl)

No counter reset needed (GUID-only architecture).

Implementation:
- src/commands/prune.ts: Update with guardrails
- Check git status and remote sync
- Append to history.jsonl
- Rewrite messages.jsonl with last N

References: PLAN.md section 7
Critical files: src/commands/prune.ts


