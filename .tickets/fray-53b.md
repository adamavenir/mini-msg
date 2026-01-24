---
id: fray-53b
status: closed
deps: []
links: []
created: 2025-12-21T09:05:34.730701-08:00
type: feature
priority: 2
---
# Suggest correct agent name when delimiter differs

When agent runs 'mm new partydev' but 'party-dev' exists (or vice versa), show 'did you mean @party-dev?' and require --force to proceed. Only trigger on delimiter differences (hyphen, dot, missing delimiter). Applies to new, back, post, get, etc.


