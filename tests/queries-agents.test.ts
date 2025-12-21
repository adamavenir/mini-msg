import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  getAgent,
  getAgentsByPrefix,
  createAgent,
  updateAgent,
  getActiveAgents,
  getAllAgents,
  getMaxVersion,
} from '../src/db/queries.ts';
import { initSchema } from '../src/db/schema.ts';
import type { Agent } from '../src/types.ts';
import Database from 'better-sqlite3';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('agent query functions', () => {
  let db: Database.Database;
  let tempDbPath: string;

  beforeEach(() => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-queries-test-'));
    tempDbPath = path.join(tempDir, 'test.db');
    db = new Database(tempDbPath);
    initSchema(db);
  });

  afterEach(() => {
    db.close();
    const tempDir = path.dirname(tempDbPath);
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  describe('createAgent', () => {
    it('should create an agent', () => {
      const now = Math.floor(Date.now() / 1000);
      createAgent(db, {
        agent_id: 'alice.1',
        status: 'testing',
        purpose: 'test agent',
        registered_at: now,
        last_seen: now,
      });

      const agent = getAgent(db, 'alice.1');
      expect(agent).toBeDefined();
      expect(agent?.agent_id).toBe('alice.1');
      expect(agent?.status).toBe('testing');
      expect(agent?.purpose).toBe('test agent');
      expect(agent?.left_at).toBeNull();
    });

    it('should throw on duplicate agent_id', () => {
      const now = Math.floor(Date.now() / 1000);
      createAgent(db, {
        agent_id: 'alice.1',
        status: 'testing',
        purpose: 'test agent',
        registered_at: now,
        last_seen: now,
      });

      expect(() => {
        createAgent(db, {
          agent_id: 'alice.1',
          status: 'duplicate',
          purpose: 'duplicate',
          registered_at: now,
          last_seen: now,
        });
      }).toThrow();
    });
  });

  describe('getAgent', () => {
    it('should return undefined for non-existent agent', () => {
      const agent = getAgent(db, 'nonexistent.1');
      expect(agent).toBeUndefined();
    });

    it('should return agent by exact ID', () => {
      const now = Math.floor(Date.now() / 1000);
      createAgent(db, {
        agent_id: 'alice.1',
        status: 'testing',
        purpose: 'test agent',
        registered_at: now,
        last_seen: now,
      });

      const agent = getAgent(db, 'alice.1');
      expect(agent?.agent_id).toBe('alice.1');
    });
  });

  describe('getAgentsByPrefix', () => {
    beforeEach(() => {
      const now = Math.floor(Date.now() / 1000);
      // Create test agents
      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });
      createAgent(db, {
        agent_id: 'alice.419',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });
      createAgent(db, {
        agent_id: 'alice.frontend.3',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });
      createAgent(db, {
        agent_id: 'bob.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });
    });

    it('should match all versions of a base name', () => {
      const agents = getAgentsByPrefix(db, 'alice');
      expect(agents.length).toBe(3);
      expect(agents.map(a => a.agent_id)).toContain('alice.1');
      expect(agents.map(a => a.agent_id)).toContain('alice.419');
      expect(agents.map(a => a.agent_id)).toContain('alice.frontend.3');
    });

    it('should match qualified prefixes', () => {
      const agents = getAgentsByPrefix(db, 'alice.frontend');
      expect(agents.length).toBe(1);
      expect(agents[0].agent_id).toBe('alice.frontend.3');
    });

    it('should return empty array for no matches', () => {
      const agents = getAgentsByPrefix(db, 'charlie');
      expect(agents.length).toBe(0);
    });

    it('should match exact ID as well', () => {
      const agents = getAgentsByPrefix(db, 'alice.1');
      expect(agents.length).toBe(1);
      expect(agents[0].agent_id).toBe('alice.1');
    });
  });

  describe('updateAgent', () => {
    beforeEach(() => {
      const now = Math.floor(Date.now() / 1000);
      createAgent(db, {
        agent_id: 'alice.1',
        status: 'original status',
        purpose: 'original purpose',
        registered_at: now,
        last_seen: now,
      });
    });

    it('should update status', () => {
      updateAgent(db, 'alice.1', { status: 'new status' });
      const agent = getAgent(db, 'alice.1');
      expect(agent?.status).toBe('new status');
      expect(agent?.purpose).toBe('original purpose');
    });

    it('should update multiple fields', () => {
      const newTime = Math.floor(Date.now() / 1000) + 100;
      updateAgent(db, 'alice.1', {
        status: 'new status',
        purpose: 'new purpose',
        last_seen: newTime,
      });

      const agent = getAgent(db, 'alice.1');
      expect(agent?.status).toBe('new status');
      expect(agent?.purpose).toBe('new purpose');
      expect(agent?.last_seen).toBe(newTime);
    });

    it('should set left_at', () => {
      const leftTime = Math.floor(Date.now() / 1000);
      updateAgent(db, 'alice.1', { left_at: leftTime });

      const agent = getAgent(db, 'alice.1');
      expect(agent?.left_at).toBe(leftTime);
    });

    it('should do nothing if no updates provided', () => {
      const originalAgent = getAgent(db, 'alice.1');
      updateAgent(db, 'alice.1', {});
      const updatedAgent = getAgent(db, 'alice.1');

      expect(updatedAgent).toEqual(originalAgent);
    });
  });

  describe('getActiveAgents', () => {
    it('should return only active agents', () => {
      const now = Math.floor(Date.now() / 1000);
      const oneHourAgo = now - 3600;
      const fiveHoursAgo = now - 5 * 3600;

      // Active agent (seen 1 hour ago)
      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: oneHourAgo,
      });

      // Stale agent (seen 5 hours ago)
      createAgent(db, {
        agent_id: 'bob.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: fiveHoursAgo,
      });

      // Left agent (recent but left)
      createAgent(db, {
        agent_id: 'charlie.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });
      updateAgent(db, 'charlie.1', { left_at: now });

      const activeAgents = getActiveAgents(db, 4); // 4 hour threshold
      expect(activeAgents.length).toBe(1);
      expect(activeAgents[0].agent_id).toBe('alice.1');
    });

    it('should return empty array if no active agents', () => {
      const now = Math.floor(Date.now() / 1000);
      const longAgo = now - 10 * 3600;

      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: longAgo,
      });

      const activeAgents = getActiveAgents(db, 4);
      expect(activeAgents.length).toBe(0);
    });
  });

  describe('getAllAgents', () => {
    it('should return all agents regardless of status', () => {
      const now = Math.floor(Date.now() / 1000);
      const longAgo = now - 10 * 3600;

      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      createAgent(db, {
        agent_id: 'bob.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: longAgo,
      });

      createAgent(db, {
        agent_id: 'charlie.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });
      updateAgent(db, 'charlie.1', { left_at: now });

      const allAgents = getAllAgents(db);
      expect(allAgents.length).toBe(3);
    });
  });

  describe('getMaxVersion', () => {
    it('should return 0 if no agents with base exist', () => {
      const maxVersion = getMaxVersion(db, 'alice');
      expect(maxVersion).toBe(0);
    });

    it('should return highest version number', () => {
      const now = Math.floor(Date.now() / 1000);

      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      createAgent(db, {
        agent_id: 'alice.419',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      createAgent(db, {
        agent_id: 'alice.5',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      const maxVersion = getMaxVersion(db, 'alice');
      expect(maxVersion).toBe(419);
    });

    it('should handle qualified base names', () => {
      const now = Math.floor(Date.now() / 1000);

      createAgent(db, {
        agent_id: 'alice.frontend.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      createAgent(db, {
        agent_id: 'alice.frontend.10',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      const maxVersion = getMaxVersion(db, 'alice.frontend');
      expect(maxVersion).toBe(10);
    });

    it('should not match different bases', () => {
      const now = Math.floor(Date.now() / 1000);

      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      createAgent(db, {
        agent_id: 'alice.frontend.5',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      // Should only count alice.1, not alice.frontend.5
      const maxVersion = getMaxVersion(db, 'alice');
      expect(maxVersion).toBe(1);
    });
  });
});
