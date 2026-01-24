---
id: fray-6qkl
status: open
deps: []
links: []
created: 2025-12-31T23:53:30.914189-08:00
type: epic
priority: 2
---
# Split db/queries.go by entity domain

db/queries.go is 2432 lines with 102 functions. Split into: queries_agents.go (19), queries_messages.go (22), queries_threads.go (11), queries_questions.go (7), queries_claims.go (11), queries_config.go. Each domain gets its own file.


