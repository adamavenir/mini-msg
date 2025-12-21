import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import fs from 'fs';
import path from 'path';
import os from 'os';
import Database from 'better-sqlite3';
import { initProject, openDatabase } from '../src/core/project.ts';
import { initSchema } from '../src/db/schema.ts';
import { rebuildDatabaseFromJsonl } from '../src/db/jsonl.ts';
import type { AgentJsonlRecord, MessageJsonlRecord } from '../src/db/jsonl.ts';

describe('sqlite rebuild from JSONL', () => {
  let tempDir: string;
  let projectRoot: string;

  beforeEach(() => {
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-rebuild-test-'));
    projectRoot = path.join(tempDir, 'project');
    fs.mkdirSync(projectRoot, { recursive: true });
  });

  afterEach(() => {
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  it('should rebuild when JSONL is newer than the database', () => {
    const project = initProject(projectRoot);
    const db = openDatabase(project);
    initSchema(db);
    db.close();

    const mmDir = path.join(projectRoot, '.mm');
    const message: MessageJsonlRecord = {
      type: 'message',
      id: 'msg-keep123',
      channel_id: 'ch-chan123',
      from_agent: 'alice.1',
      body: 'hello',
      mentions: [],
      message_type: 'agent',
      reply_to: null,
      ts: 10,
      edited_at: null,
      archived_at: null,
    };
    const agent: AgentJsonlRecord = {
      type: 'agent',
      id: 'usr-keep123',
      name: 'alice.1',
      agent_id: 'alice.1',
      status: null,
      purpose: null,
      registered_at: 1,
      last_seen: 1,
      left_at: null,
    };

    fs.writeFileSync(path.join(mmDir, 'messages.jsonl'), JSON.stringify(message) + '\n', 'utf8');
    fs.writeFileSync(path.join(mmDir, 'agents.jsonl'), JSON.stringify(agent) + '\n', 'utf8');

    const old = new Date(Date.now() - 10_000);
    fs.utimesSync(project.dbPath, old, old);

    const rebuilt = openDatabase(project);
    const row = rebuilt.prepare('SELECT guid FROM mm_messages WHERE guid = ?').get(message.id) as { guid: string } | undefined;
    expect(row?.guid).toBe('msg-keep123');
    rebuilt.close();
  });

  it('should preserve GUIDs across rebuilds', () => {
    const project = initProject(projectRoot);
    const db = openDatabase(project);
    initSchema(db);
    db.close();

    const mmDir = path.join(projectRoot, '.mm');
    const message: MessageJsonlRecord = {
      type: 'message',
      id: 'msg-fixed99',
      channel_id: null,
      from_agent: 'bob.1',
      body: 'persisted',
      mentions: [],
      message_type: 'agent',
      reply_to: null,
      ts: 20,
      edited_at: null,
      archived_at: null,
    };

    fs.writeFileSync(path.join(mmDir, 'messages.jsonl'), JSON.stringify(message) + '\n', 'utf8');

    const old = new Date(Date.now() - 10_000);
    fs.utimesSync(project.dbPath, old, old);

    const rebuilt = openDatabase(project);
    const row = rebuilt.prepare('SELECT guid FROM mm_messages WHERE guid = ?').get(message.id) as { guid: string } | undefined;
    expect(row?.guid).toBe('msg-fixed99');
    rebuilt.close();
  });

  it('should handle missing JSONL files during rebuild', () => {
    const project = initProject(projectRoot);
    const db = new Database(project.dbPath);
    rebuildDatabaseFromJsonl(db, project.dbPath);

    const row = db.prepare('SELECT COUNT(*) as count FROM mm_messages').get() as { count: number };
    expect(row.count).toBe(0);
    db.close();
  });
});
