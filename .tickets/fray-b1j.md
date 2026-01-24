---
id: fray-b1j
status: closed
deps: []
links: []
created: 2025-12-23T04:36:54.766126-08:00
type: epic
priority: 1
---
# Design: Conversation primitives (threads, questions, answers, tasks, notes)

Design and implement structured conversation primitives for multi-agent coordination.

**Spec**: docs/THREADS-SPEC.md

Core insight: Messages are facts, threads are bookmarks.

Primitives:
- Threads (bookmarks on message chains, implicit vs explicit)
- Questions (with respondents + decider)
- Answers (with confidence scores, is_decision flag)
- Tasks (agent work queue, can link to beads)
- Notes (agent memory/scratchpad)

Inspired by And Bang (2012) - real-time team coordination.


