import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('channel context', () => {
  let tempDir: string;
  let homeDir: string;
  let projectA: string;
  let projectB: string;

  let configMod: typeof import('../src/core/config.ts');
  let contextMod: typeof import('../src/core/context.ts');
  let projectMod: typeof import('../src/core/project.ts');
  let schemaMod: typeof import('../src/db/schema.ts');
  let jsonlMod: typeof import('../src/db/jsonl.ts');

  beforeEach(async () => {
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-channels-test-'));
    homeDir = path.join(tempDir, 'home');
    fs.mkdirSync(homeDir, { recursive: true });
    process.env.HOME = homeDir;

    vi.resetModules();
    configMod = await import('../src/core/config.ts');
    contextMod = await import('../src/core/context.ts');
    projectMod = await import('../src/core/project.ts');
    schemaMod = await import('../src/db/schema.ts');
    jsonlMod = await import('../src/db/jsonl.ts');

    projectA = path.join(tempDir, 'alpha');
    projectB = path.join(tempDir, 'beta');
    fs.mkdirSync(projectA, { recursive: true });
    fs.mkdirSync(projectB, { recursive: true });

    const projA = projectMod.initProject(projectA);
    const dbA = projectMod.openDatabase(projA);
    schemaMod.initSchema(dbA);
    dbA.close();
    jsonlMod.updateProjectConfig(projA.dbPath, {
      channel_id: 'ch-alpha111',
      channel_name: 'alpha',
    });
    configMod.registerChannel('ch-alpha111', 'alpha', projectA);

    const projB = projectMod.initProject(projectB);
    const dbB = projectMod.openDatabase(projB);
    schemaMod.initSchema(dbB);
    dbB.close();
    jsonlMod.updateProjectConfig(projB.dbPath, {
      channel_id: 'ch-beta222',
      channel_name: 'beta',
    });
    configMod.registerChannel('ch-beta222', 'beta', projectB);
  });

  afterEach(() => {
    delete process.env.HOME;
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  it('registers channels in global config', () => {
    const config = configMod.readGlobalConfig();
    expect(config?.channels['ch-alpha111']?.name).toBe('alpha');
    expect(config?.channels['ch-beta222']?.name).toBe('beta');
  });

  it('resolves explicit channel context by name', () => {
    const ctx = contextMod.resolveChannelContext({ channel: 'beta' });
    expect(ctx.channelId).toBe('ch-beta222');
    expect(ctx.channelName).toBe('beta');
    expect(ctx.project.root).toBe(projectB);
  });

  it('uses local project by default (no global current_channel)', () => {
    const configPath = path.join(homeDir, '.config', 'mm', 'mm-config.json');
    if (fs.existsSync(configPath)) {
      fs.unlinkSync(configPath);
    }
    const ctx = contextMod.resolveChannelContext({ cwd: projectA });
    expect(ctx.channelId).toBe('ch-alpha111');
    expect(ctx.project.root).toBe(projectA);
  });
});
