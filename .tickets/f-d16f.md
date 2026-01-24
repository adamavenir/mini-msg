---
id: f-d16f
status: open
deps: []
links: []
created: 2026-01-24T04:25:57Z
type: feature
priority: 2
assignee: Adam Avenir
tags: [daemon, chat-ui, threading]
---
# Reply-to threading behavior: mentions override trigger recipients

When a reply-to message starts with @mention(s), those mentioned agents should become the trigger recipients instead of the original message author. This makes threading more intentional - you can add context to a thread without accidentally triggering the original author.

Implementation:
1. Daemon change: If reply_to is set AND body starts with @mention, use those mentions as trigger recipients instead of IsReplyToAgent logic
2. Chat UI change: When replying, show 'Replying to: ...' in grey. If user types @ at start, change to 'Threading @mentioned into: ...' in bright blue
3. Message metadata: Add 'threaded_from' field to message so agents see 'Threaded from message <xyz>' in their context

Related: Session ID cross-contamination bug (separate issue from investigation)

## Acceptance Criteria

1. Typing '@dev hello' while replying to @pm's message only triggers @dev, not @pm
2. Chat UI shows blue 'Threading' indicator when @ is typed at start of reply
3. Agents receive 'Threaded from' context in their wake prompt

