# Daemon Test Scenarios

Controlled test environments for daemon testing. Each scenario is a git-restorable state.

## Usage

```bash
# Reset to baseline
git checkout -- test/daemon/

# Or reset a specific scenario
git checkout -- test/daemon/fresh-room/
```

## Scenarios

### fresh-room
Empty room with one human and multiple managed agents.
- Human: `adam`
- Managed agents: `alpha` (claude), `beta` (claude), `gamma` (codex)
- No messages except initial greeting
- No threads, no questions
- Tests: spawn, first-time activation

### active-thread
Ongoing discussion with ownership model.
- Human: `adam`
- Managed agents: `owner` (idle), `helper` (offline)
- Thread `design-discussion` owned by `owner`
- Thread `code-review` owned by `adam`
- Messages in both room and threads
- Tests: ownership-based triggering, thread context

Key messages:
- `msg-thread003`: Human mentions helper in owner's thread (should NOT trigger helper)
- `msg-thread004`: Agent mentions helper in owner's thread (should NOT trigger)
- `msg-thread005`: Human mentions owner in room (should trigger)

### mention-patterns
Various @mention types to test filtering.
- Human: `adam`
- Managed agents: `target` (offline), `other` (offline)
- One thread owned by `adam`

Message types for `@target`:
- `msg-direct01`: Direct address "@target hey..." (SHOULD trigger)
- `msg-midsen01`: Mid-sentence "thinking @target might..." (should NOT trigger)
- `msg-cctype01`: CC pattern "cc @target..." (should NOT trigger)
- `msg-fyi00001`: FYI pattern "FYI @target..." (should NOT trigger)
- `msg-thrddir1`: Direct in thread "@target thoughts..." (SHOULD trigger - adam owns)
- `msg-thrdmid1`: Mid-sentence in thread (should NOT trigger)
- `msg-agentm01`: Agent mentions in room (should NOT trigger)
- `msg-agentm02`: Agent mentions in thread (should NOT trigger)
- `msg-broadc01`: Broadcast "@all" (triggers all offline managed agents)

### stale-session
Agent with old session to test resume.
- Human: `adam`
- Managed agents: `stale` (offline, has session history)
- One handoff note in stale's thread
- Session records in agents.jsonl

History:
- Old session started at ts 1767300100, ended at 1767300220
- Stale left a handoff note before leaving
- New mention at ts 1767400000 should trigger resume

Tests: session resume, watermark respect, handoff context

### neo-simulation
Rich scenario for testing `neo` onboarding and `get` commands. Simulates a realistic
agent returning after being away with lots of activity to catch up on.

**Agents:**
- Human: `adam`
- Test subject: `neo` (offline, completed previous session)
- Others: `architect` (idle), `reviewer` (offline), `dev` (offline)

**Threads (5):**
- `meta` - project-wide shared context (neo subscribed)
- `neo-notes` - neo's personal handoff notes (neo owns)
- `design-discussion` - JWT auth design (neo subscribed, architect owns)
- `code-review` - PR review (neo NOT subscribed - tests filtering)
- `deep-dive` - error handling with reply chain (neo subscribed, adam owns)

**Messages (18 total):**
- 10 from neo's previous session (read before leaving)
- 8 new while neo was away (unread)
- Includes: direct mentions, mid-sentence mentions, cc/fyi mentions
- Reply chains in deep-dive thread
- Reactions on neo's messages

**Questions:**
- 1 closed (neo answered in previous session)
- 2 open directed to neo (PASETO question, error handling question)
- 1 unasked wonder from architect

**Reactions on neo's messages:**
- `msg-neo-read1`: "good idea" from architect, 👍 from adam
- `msg-neo-read3`: "LGTM" from architect and adam
- `msg-neo-note1`: "helpful" from architect

**Session history:**
- Previous session: 1767300100-1767310000 (~2.7 hours)
- Neo left handoff note before leaving
- New activity: 1767400000-1767404000

**What neo should see on return (`fray get neo`):**
- Room activity since leaving
- @mentions (direct should be prominent, cc/fyi less so)
- Thread activity in subscribed threads
- Should NOT see code-review thread activity (not subscribed)
- Open questions assigned to neo
- Reactions on their messages

**Tests:** neo onboarding, get command, mention filtering, thread subscriptions,
question visibility, reaction display, read/unread distinction

## JSONL Structure Reference

All scenarios follow the same structure:
- `fray-config.json`: Channel config with known_agents
- `agents.jsonl`: Agent records + session events
- `messages.jsonl`: Message records
- `threads.jsonl`: Thread definitions
- `questions.jsonl`: Question records (empty in all scenarios)
- `.gitignore`: Ignores *.db files

## Adding New Scenarios

1. Create directory: `test/daemon/{scenario}/.fray/`
2. Copy `.gitignore` from existing scenario
3. Create JSONL files with appropriate test data
4. Use deterministic IDs (e.g., `msg-{scenario}001`)
5. Document expected daemon behavior in this README
6. Commit to establish baseline
