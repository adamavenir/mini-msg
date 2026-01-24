---
id: fray-cbt
status: open
deps: []
links: []
created: 2025-12-30T16:47:26.234814-08:00
type: feature
priority: 3
---
# In-chat slash commands

Bring CLI capabilities into the chat interface.

```
/ask @bob what's blocking?       # ask question
/answer #qstn-xyz the thing      # answer question  
/thread new research             # create thread
/pin #abc123                     # pin message
/alias #abc123 context           # create alias
/neo                             # onboard with context dump
```

Slash commands parsed in chat input. Reduces context switching between chat and CLI.

**Depends on**: Core features (alias, pin, neo) should exist first.


