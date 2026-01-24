---
id: fray-qlw
status: closed
deps: []
links: []
created: 2025-12-04T14:03:48.56224-08:00
type: bug
priority: 1
---
# Skip integration tests gracefully when bd CLI not installed

## Problem
Integration tests hard-fail when `bd` isn't installed (tests/integration.test.ts:15-22).

```typescript
beforeEach(() => {
  try {
    execSync('bd init --prefix test', { cwd: testDir, stdio: 'pipe' });
  } catch (error) {
    console.log('Skipping integration tests: bd CLI not found');
    throw new Error('bd CLI not installed - skipping integration tests');
  }
});
```

Throwing in `beforeEach` causes the entire test suite to **fail**, not skip. This breaks `npm test` on machines without `bd`.

## Fix
1. Check for `bd` availability before the describe block
2. Use `describe.skipIf()` or conditional `describe.skip`
3. Or wrap in try/catch at describe level

Example pattern:
```typescript
const bdAvailable = (() => {
  try {
    execSync('bd --version', { stdio: 'pipe' });
    return true;
  } catch { return false; }
})();

describe.skipIf(!bdAvailable)('CLI integration tests', () => { ... });
```

## Files
- tests/integration.test.ts:15-22

## Acceptance Criteria
- `npm test` passes on machines without bd installed
- Integration tests are skipped (not failed) when bd unavailable
- Clear skip message indicates why tests were skipped


