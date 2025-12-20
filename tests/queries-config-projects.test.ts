import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  getConfig,
  setConfig,
  getAllConfig,
  getLinkedProject,
  getLinkedProjects,
  linkProject,
  unlinkProject,
} from '../src/db/queries.ts';
import { initSchema } from '../src/db/schema.ts';
import Database from 'better-sqlite3';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('config and linked project query functions', () => {
  let db: Database.Database;
  let tempDbPath: string;

  beforeEach(() => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-config-test-'));
    tempDbPath = path.join(tempDir, 'test.db');
    db = new Database(tempDbPath);
    initSchema(db);
  });

  afterEach(() => {
    db.close();
    const tempDir = path.dirname(tempDbPath);
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  describe('config functions', () => {
    it('should get default config value', () => {
      const staleHours = getConfig(db, 'stale_hours');
      expect(staleHours).toBe('4');
    });

    it('should return undefined for non-existent key', () => {
      const value = getConfig(db, 'nonexistent');
      expect(value).toBeUndefined();
    });

    it('should set new config value', () => {
      setConfig(db, 'new_key', 'new_value');
      const value = getConfig(db, 'new_key');
      expect(value).toBe('new_value');
    });

    it('should update existing config value', () => {
      setConfig(db, 'stale_hours', '8');
      const value = getConfig(db, 'stale_hours');
      expect(value).toBe('8');
    });

    it('should get all config entries', () => {
      setConfig(db, 'key1', 'value1');
      setConfig(db, 'key2', 'value2');

      const allConfig = getAllConfig(db);

      expect(allConfig.length).toBeGreaterThanOrEqual(3); // default + 2 new
      expect(allConfig.map(c => c.key)).toContain('stale_hours');
      expect(allConfig.map(c => c.key)).toContain('key1');
      expect(allConfig.map(c => c.key)).toContain('key2');
    });

    it('should return config in sorted order', () => {
      setConfig(db, 'zebra', 'last');
      setConfig(db, 'alpha', 'first');

      const allConfig = getAllConfig(db);
      const keys = allConfig.map(c => c.key);

      // Check that keys are sorted
      const sortedKeys = [...keys].sort();
      expect(keys).toEqual(sortedKeys);
    });
  });

  describe('linked project functions', () => {
    it('should link a project', () => {
      linkProject(db, 'api', '/path/to/api');

      const project = getLinkedProject(db, 'api');
      expect(project).toBeDefined();
      expect(project?.alias).toBe('api');
      expect(project?.path).toBe('/path/to/api');
    });

    it('should return undefined for non-existent project', () => {
      const project = getLinkedProject(db, 'nonexistent');
      expect(project).toBeUndefined();
    });

    it('should update existing linked project', () => {
      linkProject(db, 'api', '/old/path');
      linkProject(db, 'api', '/new/path');

      const project = getLinkedProject(db, 'api');
      expect(project?.path).toBe('/new/path');
    });

    it('should get all linked projects', () => {
      linkProject(db, 'api', '/path/to/api');
      linkProject(db, 'web', '/path/to/web');
      linkProject(db, 'cli', '/path/to/cli');

      const projects = getLinkedProjects(db);

      expect(projects.length).toBe(3);
      expect(projects.map(p => p.alias)).toContain('api');
      expect(projects.map(p => p.alias)).toContain('web');
      expect(projects.map(p => p.alias)).toContain('cli');
    });

    it('should return projects in sorted order', () => {
      linkProject(db, 'zebra', '/path/z');
      linkProject(db, 'alpha', '/path/a');
      linkProject(db, 'beta', '/path/b');

      const projects = getLinkedProjects(db);
      const aliases = projects.map(p => p.alias);

      expect(aliases).toEqual(['alpha', 'beta', 'zebra']);
    });

    it('should unlink a project', () => {
      linkProject(db, 'api', '/path/to/api');

      const removed = unlinkProject(db, 'api');
      expect(removed).toBe(true);

      const project = getLinkedProject(db, 'api');
      expect(project).toBeUndefined();
    });

    it('should return false when unlinking non-existent project', () => {
      const removed = unlinkProject(db, 'nonexistent');
      expect(removed).toBe(false);
    });

    it('should handle empty project list', () => {
      const projects = getLinkedProjects(db);
      expect(projects).toEqual([]);
    });
  });
});
