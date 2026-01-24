---
id: fray-9kh
status: closed
deps: []
links: []
created: 2025-12-21T10:58:15.528352-08:00
type: feature
priority: 2
---
# Prune: preserve thread integrity

When pruning, preserve parent messages for any replies in the keep window. Walk up the reply_to chain and add those parents to the keep set.

Example with --keep 3:
  msg-1: "Project idea"
  msg-2: "Sounds good" (reply to msg-1)
  msg-3: "Random update"
  msg-4: "Another thing"
  msg-5: "Question"
  msg-6: "Answer" (reply to msg-5)  ← in keep window
  msg-7: "Latest"                   ← in keep window
  msg-8: "Got it" (reply to msg-7)  ← in keep window

Result: Keep msg-5, msg-6, msg-7, msg-8 (msg-5 pulled in as parent of msg-6)
Prune: msg-1 through msg-4 (old thread has no replies in keep window)

Key: "preserve parents of active replies" not "preserve all threads".


