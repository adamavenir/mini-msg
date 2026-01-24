---
id: fray-4fp2
status: closed
deps: []
links: []
created: 2025-12-31T17:47:13.540206-08:00
type: bug
priority: 2
---
# Answer input: shift-enter for newlines, alt-delete for word delete

In fray answer interactive mode, shift-enter doesn't work to add newlines and alt-delete doesn't work to delete words.

## Root Cause
The answer command uses simple bufio.Reader.ReadString which doesn't support special key combinations. Chat uses Bubble Tea's textarea component which handles these.

## Solution
Refactor answer input to use Bubble Tea textarea like the chat does. This would give:
- Shift-enter for newlines
- Alt-delete for word delete  
- Consistent behavior with chat input

## Scope
Medium refactor - need to convert answer command from bufio to Bubble Tea TUI.


