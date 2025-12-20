import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { initSchema, schemaExists } from '../src/db/schema.ts';
import Database from 'better-sqlite3';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('schema', () => {
  let db: Database.Database;
  let tempDbPath: string;

  beforeEach(() => {
    // Create temporary database
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-schema-test-'));
    tempDbPath = path.join(tempDir, 'test.db');
    db = new Database(tempDbPath);
  });

  afterEach(() => {
    db.close();
    const tempDir = path.dirname(tempDbPath);
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  it('should initialize schema successfully', () => {
    initSchema(db);

    expect(schemaExists(db)).toBe(true);
  });

  it('should create all required tables', () => {
    initSchema(db);

    const tables = db.prepare(`
      SELECT name FROM sqlite_master
      WHERE type='table' AND name LIKE 'mm_%'
      ORDER BY name
    `).all() as { name: string }[];

    const tableNames = tables.map(t => t.name);

    expect(tableNames).toContain('mm_agents');
    expect(tableNames).toContain('mm_messages');
    expect(tableNames).toContain('mm_linked_projects');
    expect(tableNames).toContain('mm_config');
  });

  it('should create indexes on mm_messages', () => {
    initSchema(db);

    const indexes = db.prepare(`
      SELECT name FROM sqlite_master
      WHERE type='index' AND tbl_name='mm_messages'
    `).all() as { name: string }[];

    const indexNames = indexes.map(i => i.name);

    expect(indexNames).toContain('idx_mm_messages_ts');
    expect(indexNames).toContain('idx_mm_messages_from');
  });

  it('should insert default config', () => {
    initSchema(db);

    const config = db.prepare(`
      SELECT * FROM mm_config WHERE key = 'stale_hours'
    `).get() as { key: string; value: string } | undefined;

    expect(config).toBeDefined();
    expect(config?.value).toBe('4');
  });

  it('should be idempotent - safe to run multiple times', () => {
    initSchema(db);
    initSchema(db);
    initSchema(db);

    expect(schemaExists(db)).toBe(true);

    // Verify only one default config entry
    const configs = db.prepare(`
      SELECT COUNT(*) as count FROM mm_config WHERE key = 'stale_hours'
    `).get() as { count: number };

    expect(configs.count).toBe(1);
  });

  it('should have correct column structure for mm_agents', () => {
    initSchema(db);

    const columns = db.prepare(`
      PRAGMA table_info(mm_agents)
    `).all() as { name: string; type: string; notnull: number; pk: number }[];

    const columnNames = columns.map(c => c.name);
    const guidCol = columns.find(c => c.name === 'guid');

    expect(columnNames).toContain('guid');
    expect(columnNames).toContain('agent_id');
    expect(columnNames).toContain('goal');
    expect(columnNames).toContain('bio');
    expect(columnNames).toContain('registered_at');
    expect(columnNames).toContain('last_seen');
    expect(columnNames).toContain('left_at');
    expect(columnNames).not.toContain('id');
    expect(columnNames).not.toContain('short_id');
    expect(columnNames).not.toContain('display_id');
    expect(guidCol?.pk).toBe(1);
  });

  it('should have correct column structure for mm_messages', () => {
    initSchema(db);

    const columns = db.prepare(`
      PRAGMA table_info(mm_messages)
    `).all() as { name: string; type: string; notnull: number; pk: number }[];

    const columnNames = columns.map(c => c.name);
    const guidCol = columns.find(c => c.name === 'guid');

    expect(columnNames).toContain('guid');
    expect(columnNames).toContain('ts');
    expect(columnNames).toContain('channel_id');
    expect(columnNames).toContain('from_agent');
    expect(columnNames).toContain('body');
    expect(columnNames).toContain('mentions');
    expect(columnNames).not.toContain('id');
    expect(columnNames).not.toContain('short_id');
    expect(columnNames).not.toContain('display_id');
    expect(guidCol?.pk).toBe(1);

    // Verify default value for mentions
    const mentionsCol = columns.find(c => c.name === 'mentions');
    expect(mentionsCol).toBeDefined();
  });
});
