import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { Command } from 'commander';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('agent discovery', () => {
  let tempDir: string;
  let homeDir: string;
  let projectRoot: string;
  let channelId: string;
  let channelName: string;
  let priorHome: string | undefined;

  let projectMod: typeof import('../src/core/project.ts');
  let schemaMod: typeof import('../src/db/schema.ts');
  let jsonlMod: typeof import('../src/db/jsonl.ts');
  let configMod: typeof import('../src/core/config.ts');
  let queriesMod: typeof import('../src/db/queries.ts');

  beforeEach(async () => {
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-agent-discovery-'));
    homeDir = path.join(tempDir, 'home');
    fs.mkdirSync(homeDir, { recursive: true });
    priorHome = process.env.HOME;
    process.env.HOME = homeDir;

    channelId = 'ch-alpha111';
    channelName = 'alpha';
    projectRoot = path.join(tempDir, 'alpha');
    fs.mkdirSync(projectRoot, { recursive: true });

    vi.resetModules();
    projectMod = await import('../src/core/project.ts');
    schemaMod = await import('../src/db/schema.ts');
    jsonlMod = await import('../src/db/jsonl.ts');
    configMod = await import('../src/core/config.ts');
    queriesMod = await import('../src/db/queries.ts');

    const project = projectMod.initProject(projectRoot);
    const db = projectMod.openDatabase(project);
    schemaMod.initSchema(db);
    db.close();

    jsonlMod.updateProjectConfig(project.dbPath, {
      channel_id: channelId,
      channel_name: channelName,
    });
    configMod.registerChannel(channelId, channelName, projectRoot);
  });

  afterEach(() => {
    if (priorHome !== undefined) {
      process.env.HOME = priorHome;
    } else {
      delete process.env.HOME;
    }
    vi.restoreAllMocks();
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  function buildProgram(newCommand: () => Command): Command {
    const program = new Command();
    program.option('--in <channel>');
    program.option('--json');
    program.addCommand(newCommand());
    return program;
  }

  it('reuses known agent GUID when name matches', async () => {
    const knownGuid = 'usr-known1';
    jsonlMod.updateProjectConfig(projectRoot, {
      known_agents: {
        [knownGuid]: {
          name: 'alice',
          global_name: `${channelName}-alice`,
          home_channel: channelId,
        },
      },
    });

    const { newCommand } = await import('../src/commands/new.ts');
    const program = buildProgram(newCommand);
    await program.parseAsync(['node', 'mm', '--in', channelName, 'new', 'alice']);

    const db = projectMod.openDatabase({
      root: projectRoot,
      dbPath: path.join(projectRoot, '.mm', 'mm.db'),
    });
    schemaMod.initSchema(db);
    const agent = queriesMod.getAgent(db, 'alice');
    db.close();

    expect(agent?.guid).toBe(knownGuid);

    const config = jsonlMod.readProjectConfig(projectRoot);
    expect(config?.known_agents?.[knownGuid]?.home_channel).toBe(channelId);
  });

  it('avoids GUID collisions with existing known agents', async () => {
    const collisionGuid = 'usr-collision';
    jsonlMod.updateProjectConfig(projectRoot, {
      known_agents: {
        [collisionGuid]: {
          name: 'eve',
          global_name: `${channelName}-eve`,
          home_channel: channelId,
        },
      },
    });

    const sequence = [collisionGuid, 'usr-fresh1'];
    const guidMod = await import('../src/core/guid.ts');
    const actualGenerateGuid = guidMod.generateGuid;
    const guidSpy = vi.spyOn(guidMod, 'generateGuid')
      .mockImplementation((prefix: string) => {
        if (prefix.startsWith('usr')) {
          return sequence.shift() ?? 'usr-fallback';
        }
        return actualGenerateGuid(prefix);
      });

    const { newCommand } = await import('../src/commands/new.ts');
    const program = buildProgram(newCommand);
    await program.parseAsync(['node', 'mm', '--in', channelName, 'new', 'alice']);

    const db = projectMod.openDatabase({
      root: projectRoot,
      dbPath: path.join(projectRoot, '.mm', 'mm.db'),
    });
    schemaMod.initSchema(db);
    const agent = queriesMod.getAgent(db, 'alice');
    db.close();

    expect(agent?.guid).toBe('usr-fresh1');
    guidSpy.mockRestore();
  });

  it('records global name and home channel for new agents', async () => {
    const { newCommand } = await import('../src/commands/new.ts');
    const program = buildProgram(newCommand);
    await program.parseAsync(['node', 'mm', '--in', channelName, 'new', 'devrel']);

    const db = projectMod.openDatabase({
      root: projectRoot,
      dbPath: path.join(projectRoot, '.mm', 'mm.db'),
    });
    schemaMod.initSchema(db);
    const agent = queriesMod.getAgent(db, 'devrel');
    db.close();

    expect(agent).toBeTruthy();
    const config = jsonlMod.readProjectConfig(projectRoot);
    const entry = config?.known_agents?.[agent?.guid ?? ''];
    expect(entry?.global_name).toBe(`${channelName}-devrel`);
    expect(entry?.home_channel).toBe(channelId);
  });
});
