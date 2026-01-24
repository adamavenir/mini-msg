---
id: fray-3hj
status: closed
deps: []
links: []
created: 2025-12-22T12:01:32.047673-08:00
type: bug
priority: 1
---
# Bug: @all not expanded to individual agent mentions

When user posts with @all, it's stored literally as 'all' in mentions array instead of expanding to all agent names.

Current behavior:
  mentions: ["all"]

Expected behavior:
  mentions: ["alice", "bob", "charlie", ...]

This breaks mention history - agents don't see @all messages when checking their @mentions because their name isn't in the array.

Fix: when extracting mentions and 'all' is found, expand to list of all registered agent IDs.


