---
id: fray-zei
status: closed
deps: [fray-qnd]
links: []
created: 2025-12-31T14:42:26.672635-08:00
type: feature
priority: 1
---
# Add 'fray answer' command for question review workflow

Dedicated interface for users to review and answer questions one at a time.

## Interface
- Modal/blocking (like Claude Code's question UI)
- One question at a time
- Options with pros/cons (a. b. c. with nested - Pro/Con)
- User can select option OR write custom answer

## Accept/Close Flow
- **Agent → User:** Requires explicit ACCEPT before truly closed
  - Answer posted → 'has-answer'
  - User reviews → 'accepted' or 'needs-revision'
- **Agent → Agent:** Auto-close on READ by recipient
  - Answer posted → 'has-answer'
  - Recipient reads → auto-closed (acknowledgment)

## Skip Flow
- Skip = soft ignore for now
- At END of answer session: 'Review X previously skipped questions?'
- Does NOT show questions skipped THIS session

## Order
- Newest question set first
- Questions shown in order listed in set
- Then backward through older sets

## Invocation
- `fray answer` standalone
- `/answer` in chat interface
- `fray` leads into answer if questions pending (return = answer, any other key = exit)

See msg-ln1pevbd for original design, msg-qgromx1z for accept/close flow.


