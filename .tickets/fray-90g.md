---
id: fray-90g
status: open
deps: []
links: []
created: 2025-12-30T16:47:25.980687-08:00
type: feature
priority: 3
---
# Add 'export' command

Export thread/channel to file.

```bash
fray export thrd-xyz > discussion.txt
fray export --channel > full-channel.md
fray export --format markdown thrd-xyz
fray export --format json thrd-xyz
```

Output formats:
- Plain text (default)
- Markdown with metadata
- JSON for programmatic use

Use cases: documentation, archiving, sharing outside fray.


