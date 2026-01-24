---
id: fray-co5
status: closed
deps: [fray-2vy, fray-oa5]
links: []
created: 2025-12-04T10:10:22.288019-08:00
type: task
priority: 1
parent: fray-yh4
---
# Implement config command

Implement the config command for viewing and setting configuration.

## Usage

```bash
bdm config                       # show all config
bdm config stale-hours           # show specific key
bdm config stale-hours 8         # set value
```

## Available Config Keys

| Key | Default | Description |
|-----|---------|-------------|
| stale_hours | 4 | Hours of inactivity before agent is stale |

## Implementation

### Show all config
```bash
bdm config
```

Output:
```
stale_hours = 4
```

### Show specific key
```bash
bdm config stale-hours
```

Output:
```
stale_hours = 4
```

### Set value
```bash
bdm config stale-hours 8
```

Output:
```
stale_hours = 8 (was: 4)
```

## Key Name Normalization
- Accept both `stale-hours` and `stale_hours`
- Store with underscores internally
- Display with underscores

## Validation
- stale_hours: must be positive integer
- Unknown key: "Unknown config key: foo"

## JSON Output

```json
{
  "stale_hours": "4"
}
```

## Files
- src/commands/config.ts

## Acceptance Criteria
- Show all config works
- Show specific key works
- Set value works with validation
- Key name normalization (hyphens to underscores)
- Error on unknown keys


