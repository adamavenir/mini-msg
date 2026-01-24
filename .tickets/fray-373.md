---
id: fray-373
status: closed
deps: [fray-vfv]
links: []
created: 2025-12-30T16:46:47.429502-08:00
type: feature
priority: 2
---
# Improve agent read state tracking

SUPERSEDED: See fray-vfv for new watermark-based design.

Original requirements still apply but implementation is now via positional read_to watermarks instead of per-message receipts.

Key insight: per-message receipts are semantically wrong because session identity ≠ agent identity. Watermarks track position, not individual items.


