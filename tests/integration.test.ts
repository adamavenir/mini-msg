import { describe, it, expect, beforeAll, beforeEach, afterEach } from 'vitest';
import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('CLI integration tests', () => {
  let testDir: string;
  let homeDir: string;
  const mmPath = path.resolve(process.cwd(), 'dist/bin/mm.js');

  beforeAll(() => {
    execSync('npm run build', { cwd: process.cwd(), stdio: 'pipe' });
  });

  beforeEach(() => {
    // Create temp directory
    testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-integration-test-'));
    homeDir = path.join(testDir, 'home');
    fs.mkdirSync(homeDir, { recursive: true });

    // Initialize mm project
    execSync(`node ${mmPath} init`, {
      cwd: testDir,
      stdio: 'pipe',
      env: { ...process.env, HOME: homeDir },
    });
  });

  afterEach(() => {
    if (testDir && fs.existsSync(testDir)) {
      fs.rmSync(testDir, { recursive: true, force: true });
    }
  });

  function run(cmd: string, cwd = testDir): string {
    const fullCmd = `node ${mmPath} ${cmd}`;
    const output = execSync(fullCmd, {
      cwd,
      encoding: 'utf-8',
      env: { ...process.env, HOME: homeDir },
    });
    // Strip ANSI color codes for easier testing
    return output.replace(/\x1b\[[0-9;]*m/g, '');
  }

  function runExpectingError(cmd: string, cwd = testDir): { code: number; output: string } {
    const fullCmd = `node ${mmPath} ${cmd}`;
    try {
      const output = execSync(fullCmd, {
        cwd,
        encoding: 'utf-8',
        env: { ...process.env, HOME: homeDir },
      });
      return { code: 0, output };
    } catch (error: any) {
      return { code: error.status || 1, output: error.stderr || error.stdout || '' };
    }
  }

  function sleep(ms: number): void {
    const shared = new SharedArrayBuffer(4);
    const view = new Int32Array(shared);
    Atomics.wait(view, 0, 0, ms);
  }

  describe('agent lifecycle', () => {
    it('should create agent with simple name', () => {
      const output = run('new alice --goal "testing"');
      expect(output).toContain('@alice');
      expect(output).toContain('testing');
    });

    it('should error when creating agent with same active name', () => {
      run('new alice');
      const result = runExpectingError('new alice');
      expect(result.code).toBe(1);
      expect(result.output).toContain('currently active');
    });

    it('should allow reclaiming stale/left agent name', () => {
      run('new alice');
      run('bye alice');
      const output = run('new alice');
      expect(output).toContain('Rejoined');
      expect(output).toContain('@alice');
    });

    it('should generate random name when none provided', () => {
      const output = run('new');
      // Random names are like "eager-beaver"
      expect(output).toMatch(/@[a-z]+-[a-z]+/);
    });

    it('should resume agent session with back', () => {
      run('new alice');
      run('bye alice');
      const output = run('back alice');
      expect(output).toContain('Welcome back');
    });

    it('should mark agent as left with bye', () => {
      run('new alice');
      run('bye alice');
      const hereOutput = run('here');
      expect(hereOutput).not.toContain('@alice');
    });

    it('should clear claims when agent leaves with bye', () => {
      run('new alice');
      run('claim alice --file src/auth.ts');
      const beforeBye = run('claims');
      expect(beforeBye).toContain('src/auth.ts');
      run('bye alice');
      const afterBye = run('claims');
      expect(afterBye).toContain('No active claims');
    });

    it('should show agent details with who', () => {
      run('new alice --goal "testing" --bio "test agent"');
      const output = run('who alice');
      expect(output).toContain('alice');
      expect(output).toContain('testing');
      expect(output).toContain('test agent');
      expect(output).toContain('active');
    });

    it('should rename an agent', () => {
      run('new alice');
      const output = run('rename alice bob');
      expect(output).toContain('Renamed @alice to @bob');
      const hereOutput = run('here');
      expect(hereOutput).toContain('@bob');
      expect(hereOutput).not.toContain('@alice');
    });
  });

  describe('messaging', () => {
    beforeEach(() => {
      run('new alice --goal "sender"');
      run('new bob --goal "receiver"');
    });

    it('should post and display messages', () => {
      run('post --as alice "hello world"');
      const output = run('');
      expect(output).toContain('hello world');
      expect(output).toContain('alice');
    });

    it('should extract and route @mentions', () => {
      run('post --as alice "@bob hey there"');
      const bobMentions = run('@bob');
      expect(bobMentions).toContain('@bob hey there');
    });

    it('should support prefix matching for mentions', () => {
      run('post --as alice "@bob check this out"');
      // Querying @bob should find messages mentioning @bob
      const bobMentions = run('@bob');
      expect(bobMentions).toContain('@bob check this out');
    });

    it('should handle @all broadcasts', () => {
      run('post --as alice "@all standup time"');
      const aliceMentions = run('@alice');
      const bobMentions = run('@bob');
      expect(aliceMentions).toContain('@all standup time');
      expect(bobMentions).toContain('@all standup time');
    });

    it('should limit message display', () => {
      sleep(1100);
      run('post --as alice "msg1"');
      sleep(1100);
      run('post --as alice "msg2"');
      sleep(1100);
      run('post --as alice "msg3"');
      const output = run('get --last 2');
      expect(output).toContain('msg2');
      expect(output).toContain('msg3');
    });
  });

  describe('presence tracking', () => {
    it('should list active agents', () => {
      run('new alice --goal "active agent"');
      run('new bob --goal "also active"');
      const output = run('here');
      expect(output).toContain('@alice');
      expect(output).toContain('@bob');
      expect(output).toContain('active agent');
      expect(output).toContain('also active');
    });

    it('should not show left agents in here', () => {
      run('new alice');
      run('new bob');
      run('bye alice');
      const output = run('here');
      expect(output).not.toContain('@alice');
      expect(output).toContain('@bob');
    });

    it('should show stale agents with here --all', () => {
      // This test is challenging because we'd need to artificially age an agent
      // For now, just verify that --all doesn't crash and shows active agents
      run('new alice');
      const output = run('here --all');
      expect(output).toContain('@alice');
    });
  });

  describe('channel context', () => {
    it('should support --in and mm use across channels', () => {
      const otherDir = path.join(testDir, 'other');
      fs.mkdirSync(otherDir, { recursive: true });
      execSync(`node ${mmPath} init`, {
        cwd: otherDir,
        stdio: 'pipe',
        env: { ...process.env, HOME: homeDir },
      });

      const primaryName = path.basename(testDir);
      const otherName = path.basename(otherDir);
      run(`use ${primaryName}`);

      run('new alice');
      run('post --as alice "primary"');

      run(`--in ${otherName} new bob`);
      run(`--in ${otherName} post --as bob "secondary"`);

      const otherHere = run(`--in ${otherName} here`);
      expect(otherHere).toContain('@bob');
      expect(otherHere).not.toContain('@alice');

      run(`use ${otherName}`);
      const switchedHere = run('here');
      expect(switchedHere).toContain('@bob');
      expect(switchedHere).not.toContain('@alice');
    });
  });

  describe('configuration', () => {
    it('should set and get config values', () => {
      run('config stale-hours 8');
      const output = run('config stale-hours');
      expect(output).toContain('8');
    });

    it('should list all config', () => {
      run('config stale-hours 6');
      const output = run('config');
      expect(output).toContain('stale_hours');
      expect(output).toContain('6');
    });
  });

  describe('error handling', () => {
    it('should error for unknown agent in post', () => {
      const result = runExpectingError('post --as unknown "test"');
      expect(result.code).toBe(1);
      expect(result.output).toContain('Agent not found');
    });

    it('should error for unknown agent in bye', () => {
      const result = runExpectingError('bye unknown');
      expect(result.code).toBe(1);
      expect(result.output).toContain('Agent not found');
    });

    it('should error for unknown agent in back', () => {
      const result = runExpectingError('back unknown');
      expect(result.code).toBe(1);
      expect(result.output).toContain('Agent not found');
    });
  });

  describe('prune guardrails', () => {
    it('should refuse to prune with uncommitted .mm changes', () => {
      execSync('git init', {
        cwd: testDir,
        stdio: 'pipe',
        env: { ...process.env, HOME: homeDir },
      });

      run('new alice');
      execSync('git add .mm', {
        cwd: testDir,
        stdio: 'pipe',
        env: { ...process.env, HOME: homeDir },
      });
      execSync('git -c user.name=Test -c user.email=test@example.com commit -m "init"', {
        cwd: testDir,
        stdio: 'pipe',
        env: { ...process.env, HOME: homeDir },
      });

      run('new bob');
      const result = runExpectingError('prune');
      expect(result.code).toBe(1);
      expect(result.output).toContain('Uncommitted changes in .mm/. Commit first.');
    });
  });

  describe('claims', () => {
    beforeEach(() => {
      run('new alice');
      run('new bob');
    });

    it('should claim files', () => {
      const output = run('claim alice --file src/auth.ts');
      expect(output).toContain('@alice claimed');
      expect(output).toContain('src/auth.ts');
    });

    it('should claim with globs', () => {
      const output = run('claim alice --files "src/*.ts,lib/*.ts"');
      expect(output).toContain('src/*.ts');
      expect(output).toContain('lib/*.ts');
    });

    it('should claim bd issues', () => {
      const output = run('claim alice --bd xyz-123');
      expect(output).toContain('bd:xyz-123');
    });

    it('should strip # from bd and issue', () => {
      run('claim alice --bd "#abc-1" --issue "#456"');
      const output = run('claims alice');
      expect(output).toContain('bd:abc-1');
      expect(output).toContain('issue:456');
    });

    it('should list claims', () => {
      run('claim alice --file src/auth.ts --reason "working on auth"');
      const output = run('claims');
      expect(output).toContain('@alice');
      expect(output).toContain('src/auth.ts');
      expect(output).toContain('working on auth');
    });

    it('should error on duplicate claim', () => {
      run('claim alice --file src/auth.ts');
      const result = runExpectingError('claim bob --file src/auth.ts');
      expect(result.code).toBe(1);
      expect(result.output).toContain('Already claimed by @alice');
    });

    it('should clear specific claims', () => {
      run('claim alice --file src/auth.ts --bd xyz-123');
      run('clear alice --file src/auth.ts');
      const output = run('claims alice');
      expect(output).not.toContain('src/auth.ts');
      expect(output).toContain('bd:xyz-123');
    });

    it('should clear all claims', () => {
      run('claim alice --file src/auth.ts --bd xyz-123');
      run('clear alice');
      const output = run('claims alice');
      expect(output).toContain('No claims');
    });
  });

  describe('status', () => {
    beforeEach(() => {
      run('new alice');
    });

    it('should update goal with status message', () => {
      run('status alice "working on auth"');
      const whoOutput = run('who alice');
      expect(whoOutput).toContain('working on auth');
    });

    it('should create claims with status', () => {
      run('status alice "fixing login" --file src/auth.ts --bd xyz-123');
      const claimsOutput = run('claims alice');
      expect(claimsOutput).toContain('src/auth.ts');
      expect(claimsOutput).toContain('bd:xyz-123');
    });

    it('should post status message to room', () => {
      run('status alice "refactoring db" --file src/db.ts');
      const output = run('');
      expect(output).toContain('refactoring db');
      expect(output).toContain('claimed');
      expect(output).toContain('src/db.ts');
    });

    it('should clear status with --clear', () => {
      run('status alice "testing" --file src/test.ts');
      run('status alice --clear');
      const claimsOutput = run('claims alice');
      expect(claimsOutput).toContain('No claims');
      const whoOutput = run('who alice');
      expect(whoOutput).not.toContain('testing');
    });

    it('should show claim counts in here', () => {
      run('status alice "working" --file src/a.ts --file src/b.ts');
      const hereOutput = run('here');
      expect(hereOutput).toContain('@alice');
      expect(hereOutput).toContain('claim');
    });
  });

  describe('JSON output mode', () => {
    it('should output JSON with --json flag', () => {
      run('new alice --goal "test"');
      const output = run('--json here');
      const parsed = JSON.parse(output);
      expect(Array.isArray(parsed.agents)).toBe(true);
      expect(parsed.agents[0].display_name).toBe('alice');
      expect(parsed.agents[0].agent_id).toMatch(/^usr-/);
      expect(parsed.agents[0].goal).toBe('test');
      expect(parsed.total).toBe(1);
    });

    it('should output JSON with @mention shorthand and --json flag', () => {
      run('new alice --goal "sender"');
      run('new bob --goal "receiver"');
      run('post --as alice "@bob hey there"');
      const output = run('@bob --json');
      const parsed = JSON.parse(output);
      expect(Array.isArray(parsed)).toBe(true);
      expect(parsed.length).toBeGreaterThan(0);
      expect(parsed[0].body).toContain('@bob hey there');
      expect(parsed[0].from_agent).toBe('alice');
    });
  });
});
