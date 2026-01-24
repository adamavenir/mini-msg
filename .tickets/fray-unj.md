---
id: fray-unj
status: open
deps: []
links: []
created: 2025-12-30T16:47:26.108512-08:00
type: feature
priority: 3
---
# Add 'import' command

Import file as messages.

```bash
fray import doc.md --as alice              # single message
fray import doc.md --as alice --split      # split on ## headers
fray import doc.md --as alice --thread research
```

Splitting logic:
- `#` or `##` markdown headers indicate message boundaries
- Each section becomes its own message
- Preserves threading/attribution

Use cases: bringing external docs into fray, seeding conversations.


