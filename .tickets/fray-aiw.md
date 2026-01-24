---
id: fray-aiw
status: closed
deps: []
links: []
created: 2025-12-07T17:20:57.581699-08:00
type: feature
priority: 2
---
# Highlight user mentions in default color

User mentions like @adam should be visually distinct but use the terminal's default color.

## Current Behavior

When agents mention users like @adam, the mention appears as plain text because users don't have entries in the bdm_agents table. Only registered agent mentions get colorized.

Example:
- `@validation.1` → colored (registered agent)
- `@adam` → plain text (user, no visual distinction)

## Desired Behavior

User mentions should be highlighted in the terminal's default/reset color (white text on dark terminals, black on light terminals). This makes them visually distinct from plain text while differentiating them from agent mentions.

Example:
- `@validation.1` → colored with agent's color (e.g., green background)
- `@adam` → bold or underlined in default terminal color

## Implementation

Add a third category of mention highlighting:

1. **Agent mentions** (in agentBases): Use agent's color
2. **User mentions** (not in agentBases, but looks like a name): Use default/reset color with bold
3. **Everything else** (@payload, @foobar): Plain text

**Detection heuristic for users:**
- Not in agentBases set
- Length 3-15 characters
- No dots (distinguishes from agent IDs)
- Alphanumeric only

**Styling**: `${BOLD}${match}${RESET}${senderColor}` (bold in default color)

## Files to Modify

- `src/commands/format.ts` - Update `colorizeBody()` to handle user mentions
- `src/chat/display.ts` - Update `colorizeBody()` to handle user mentions

## Testing

```bash
bdm post --as test.1 "Hey @adam can you review this?"
bdm get --last 1  # @adam should be bold in default color
```


