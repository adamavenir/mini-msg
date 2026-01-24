---
id: fray-qnd
status: closed
deps: []
links: []
created: 2025-12-31T14:42:11.807705-08:00
type: feature
priority: 1
---
# Parse '# Questions for @x' sections from posts

Auto-extract questions from markdown posts.

## Convention
- REQUIRE `# Questions for @x` header (or just `# Questions` = for user)
- Allow `#` or `##` headers, ignore `###`+
- Skip fenced code blocks when parsing
- Multiple targets: `# Questions for @x @y @z`
- Each numbered item (1. 2. 3.) becomes a question
- Options via a. b. c. with nested - Pro/Con
- `# Wondering` sections tracked but not displayed

## Storage Model
- Keep ORIGINAL body in JSONL (don't strip)
- Create linked question records on post
- Hide question content on RENDER only

## Edit Handling
- Editing body does NOT re-extract questions
- Warn user that # Questions/# Wondering hidden on render
- To edit questions, edit question object directly

## Backward Compat
- `fray ask` = single focused question (complementary)
- `# Questions` = batch in context

See msg-ln1pevbd for original design, msg-mv35bxrh for implementation decisions.


