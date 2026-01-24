---
id: fray-wkx
status: closed
deps: [fray-yug, fray-8xb]
links: []
created: 2025-12-04T10:17:46.99771-08:00
type: task
priority: 1
parent: fray-po1
---
# Write integration tests for key flows

Create integration tests covering the main user workflows.

## Prerequisites
- Tests require the `bd` CLI to be installed (beads issue tracker)
- Tests create temporary beads projects using `bd init`

## Test Flows

### 1. Full Agent Lifecycle
```typescript
test('agent lifecycle: new → post → bye', async () => {
  // Create agent
  await run('bdm new alice --goal "testing"');
  // Verify agent exists
  expect(await run('bdm who alice')).toContain('alice.1');
  // Post message
  await run('bdm post --as alice.1 "hello world"');
  // Verify message
  expect(await run('bdm')).toContain('hello world');
  // Leave
  await run('bdm bye alice.1');
  // Verify not in here
  expect(await run('bdm here')).not.toContain('alice.1');
});
```

### 2. Mention Routing
```typescript
test('mention routing', async () => {
  await run('bdm new alice');
  await run('bdm new bob');
  await run('bdm post --as alice.1 "@bob.1 hey there"');
  await run('bdm post --as bob.1 "@alice check this"');
  
  // Bob should see alice's message
  expect(await run('bdm @bob')).toContain('@bob.1 hey there');
  // Alice should see bob's message (prefix match)
  expect(await run('bdm @alice')).toContain('@alice check this');
});
```

### 3. @all Broadcast
```typescript
test('@all broadcast', async () => {
  await run('bdm new pm');
  await run('bdm new dev');
  await run('bdm post --as pm.1 "@all standup"');
  
  // Both should see it
  expect(await run('bdm @pm')).toContain('@all standup');
  expect(await run('bdm @dev')).toContain('@all standup');
});
```

### 4. Cross-Project Messaging
```typescript
test('cross-project messaging', async () => {
  // Setup: create two beads projects using bd init
  await runIn('/tmp/proj-a', 'bd init --prefix a');
  await runIn('/tmp/proj-b', 'bd init --prefix b');
  
  // Link from A to B
  await runIn('/tmp/proj-a', 'bdm link other /tmp/proj-b');
  
  // Create agent in A, post to B
  await runIn('/tmp/proj-a', 'bdm new frontend');
  await runIn('/tmp/proj-a', 'bdm --project other post --as frontend.1 "hey B"');
  
  // Verify message appears in B
  expect(await runIn('/tmp/proj-b', 'bdm')).toContain('hey B');
});
```

### 5. Stale Agent Filtering
```typescript
test('stale agents filtered from here', async () => {
  await run('bdm new alice');
  // Artificially age the agent by updating DB directly
  // Note: need direct DB access for this test
  await db.run('UPDATE bdm_agents SET last_seen = ? WHERE agent_id = ?', 
    [Date.now()/1000 - 5*3600, 'alice.1']);
  
  // Default here should not show stale
  expect(await run('bdm here')).not.toContain('alice.1');
  // --all should show
  expect(await run('bdm here --all')).toContain('alice.1');
});
```

## Test Setup

```typescript
// tests/setup.ts
import { execaCommand } from 'execa';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

let testDir: string;

export async function setupTestProject(): Promise<string> {
  // Create temp directory
  testDir = await fs.mkdtemp(path.join(os.tmpdir(), 'bdm-test-'));
  // Initialize beads project
  await execaCommand('bd init --prefix test', { cwd: testDir });
  return testDir;
}

export async function cleanup() {
  if (testDir) {
    await fs.rm(testDir, { recursive: true, force: true });
  }
}

export function run(cmd: string, cwd: string = testDir) {
  return execaCommand(cmd, { cwd });
}
```

## Files
- tests/integration/lifecycle.test.ts
- tests/integration/mentions.test.ts
- tests/integration/cross-project.test.ts
- tests/setup.ts

## Acceptance Criteria
- All key workflows have integration tests
- Tests run in isolated temp directories
- Tests clean up after themselves
- All tests pass


