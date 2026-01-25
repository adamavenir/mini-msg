---
id: fray-eg25
status: closed
deps: []
links: []
created: 2025-12-31T23:53:30.767432-08:00
type: epic
priority: 2
---
# Decompose chat/model.go into focused modules

chat/model.go is 2966 lines handling 10+ UI subsystems. Split into: model.go (core), model_update.go, model_view.go, messages.go, suggestions.go, panels.go, questions.go, viewport.go, colors.go, layout.go, input.go. Each file <300 lines with single responsibility.


