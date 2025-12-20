import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  createClaim,
  getClaim,
  getClaimsByAgent,
  getClaimsByType,
  getAllClaims,
  deleteClaim,
  deleteClaimsByAgent,
  findConflictingFileClaims,
  pruneExpiredClaims,
  createAgent,
} from '../src/db/queries.ts';
import { initSchema } from '../src/db/schema.ts';
import Database from 'better-sqlite3';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('claim query functions', () => {
  let db: Database.Database;
  let tempDbPath: string;

  beforeEach(() => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-claims-test-'));
    tempDbPath = path.join(tempDir, 'test.db');
    db = new Database(tempDbPath);
    initSchema(db);

    // Create test agents
    const now = Math.floor(Date.now() / 1000);
    createAgent(db, {
      agent_id: 'alice',
      goal: null,
      bio: null,
      registered_at: now,
      last_seen: now,
    });
    createAgent(db, {
      agent_id: 'bob',
      goal: null,
      bio: null,
      registered_at: now,
      last_seen: now,
    });
  });

  afterEach(() => {
    db.close();
    const tempDir = path.dirname(tempDbPath);
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  describe('createClaim', () => {
    it('should create a file claim', () => {
      const claim = createClaim(db, {
        agent_id: 'alice',
        claim_type: 'file',
        pattern: 'src/auth.ts',
        reason: 'working on auth',
      });

      expect(claim.id).toBeDefined();
      expect(claim.agent_id).toBe('alice');
      expect(claim.claim_type).toBe('file');
      expect(claim.pattern).toBe('src/auth.ts');
      expect(claim.reason).toBe('working on auth');
      expect(claim.created_at).toBeGreaterThan(0);
      expect(claim.expires_at).toBeNull();
    });

    it('should create a bd claim', () => {
      const claim = createClaim(db, {
        agent_id: 'alice',
        claim_type: 'bd',
        pattern: 'xyz-123',
      });

      expect(claim.claim_type).toBe('bd');
      expect(claim.pattern).toBe('xyz-123');
    });

    it('should create an issue claim', () => {
      const claim = createClaim(db, {
        agent_id: 'alice',
        claim_type: 'issue',
        pattern: '456',
      });

      expect(claim.claim_type).toBe('issue');
      expect(claim.pattern).toBe('456');
    });

    it('should create a claim with TTL', () => {
      const expiresAt = Math.floor(Date.now() / 1000) + 7200; // 2 hours
      const claim = createClaim(db, {
        agent_id: 'alice',
        claim_type: 'file',
        pattern: 'src/temp.ts',
        expires_at: expiresAt,
      });

      expect(claim.expires_at).toBe(expiresAt);
    });

    it('should error on duplicate claim', () => {
      createClaim(db, {
        agent_id: 'alice',
        claim_type: 'file',
        pattern: 'src/auth.ts',
      });

      expect(() => {
        createClaim(db, {
          agent_id: 'bob',
          claim_type: 'file',
          pattern: 'src/auth.ts',
        });
      }).toThrow('Already claimed by @alice');
    });
  });

  describe('getClaim', () => {
    it('should get a claim by type and pattern', () => {
      createClaim(db, {
        agent_id: 'alice',
        claim_type: 'file',
        pattern: 'src/auth.ts',
      });

      const claim = getClaim(db, 'file', 'src/auth.ts');
      expect(claim).toBeDefined();
      expect(claim?.agent_id).toBe('alice');
    });

    it('should return undefined for non-existent claim', () => {
      const claim = getClaim(db, 'file', 'nonexistent.ts');
      expect(claim).toBeUndefined();
    });
  });

  describe('getClaimsByAgent', () => {
    it('should get all claims for an agent', () => {
      createClaim(db, { agent_id: 'alice', claim_type: 'file', pattern: 'src/a.ts' });
      createClaim(db, { agent_id: 'alice', claim_type: 'bd', pattern: 'xyz-1' });
      createClaim(db, { agent_id: 'bob', claim_type: 'file', pattern: 'src/b.ts' });

      const aliceClaims = getClaimsByAgent(db, 'alice');
      expect(aliceClaims.length).toBe(2);
      expect(aliceClaims.map(c => c.pattern)).toContain('src/a.ts');
      expect(aliceClaims.map(c => c.pattern)).toContain('xyz-1');
    });
  });

  describe('getClaimsByType', () => {
    it('should get all claims of a type', () => {
      createClaim(db, { agent_id: 'alice', claim_type: 'file', pattern: 'src/a.ts' });
      createClaim(db, { agent_id: 'bob', claim_type: 'file', pattern: 'src/b.ts' });
      createClaim(db, { agent_id: 'alice', claim_type: 'bd', pattern: 'xyz-1' });

      const fileClaims = getClaimsByType(db, 'file');
      expect(fileClaims.length).toBe(2);

      const bdClaims = getClaimsByType(db, 'bd');
      expect(bdClaims.length).toBe(1);
    });
  });

  describe('getAllClaims', () => {
    it('should get all active claims', () => {
      createClaim(db, { agent_id: 'alice', claim_type: 'file', pattern: 'src/a.ts' });
      createClaim(db, { agent_id: 'bob', claim_type: 'bd', pattern: 'xyz-1' });

      const claims = getAllClaims(db);
      expect(claims.length).toBe(2);
    });

    it('should prune expired claims', () => {
      const pastTime = Math.floor(Date.now() / 1000) - 3600; // 1 hour ago
      createClaim(db, {
        agent_id: 'alice',
        claim_type: 'file',
        pattern: 'src/expired.ts',
        expires_at: pastTime,
      });
      createClaim(db, { agent_id: 'bob', claim_type: 'file', pattern: 'src/active.ts' });

      const claims = getAllClaims(db);
      expect(claims.length).toBe(1);
      expect(claims[0].pattern).toBe('src/active.ts');
    });
  });

  describe('deleteClaim', () => {
    it('should delete a specific claim', () => {
      createClaim(db, { agent_id: 'alice', claim_type: 'file', pattern: 'src/a.ts' });

      const deleted = deleteClaim(db, 'file', 'src/a.ts');
      expect(deleted).toBe(true);

      const claim = getClaim(db, 'file', 'src/a.ts');
      expect(claim).toBeUndefined();
    });

    it('should return false for non-existent claim', () => {
      const deleted = deleteClaim(db, 'file', 'nonexistent.ts');
      expect(deleted).toBe(false);
    });
  });

  describe('deleteClaimsByAgent', () => {
    it('should delete all claims for an agent', () => {
      createClaim(db, { agent_id: 'alice', claim_type: 'file', pattern: 'src/a.ts' });
      createClaim(db, { agent_id: 'alice', claim_type: 'bd', pattern: 'xyz-1' });
      createClaim(db, { agent_id: 'bob', claim_type: 'file', pattern: 'src/b.ts' });

      const deleted = deleteClaimsByAgent(db, 'alice');
      expect(deleted).toBe(2);

      const aliceClaims = getClaimsByAgent(db, 'alice');
      expect(aliceClaims.length).toBe(0);

      const bobClaims = getClaimsByAgent(db, 'bob');
      expect(bobClaims.length).toBe(1);
    });
  });

  describe('findConflictingFileClaims', () => {
    beforeEach(() => {
      createClaim(db, { agent_id: 'alice', claim_type: 'file', pattern: 'src/auth.ts' });
      createClaim(db, { agent_id: 'bob', claim_type: 'file', pattern: 'src/api/*.ts' });
    });

    it('should find exact match conflicts', () => {
      const conflicts = findConflictingFileClaims(db, ['src/auth.ts']);
      expect(conflicts.length).toBe(1);
      expect(conflicts[0].agent_id).toBe('alice');
    });

    it('should find glob pattern conflicts', () => {
      const conflicts = findConflictingFileClaims(db, ['src/api/users.ts']);
      expect(conflicts.length).toBe(1);
      expect(conflicts[0].agent_id).toBe('bob');
    });

    it('should exclude agent from conflict check', () => {
      const conflicts = findConflictingFileClaims(db, ['src/auth.ts'], 'alice');
      expect(conflicts.length).toBe(0);
    });

    it('should return empty array when no conflicts', () => {
      const conflicts = findConflictingFileClaims(db, ['src/other.ts']);
      expect(conflicts.length).toBe(0);
    });

    it('should handle multiple files', () => {
      const conflicts = findConflictingFileClaims(db, [
        'src/auth.ts',
        'src/api/users.ts',
        'src/other.ts',
      ]);
      expect(conflicts.length).toBe(2);
    });
  });

  describe('pruneExpiredClaims', () => {
    it('should prune expired claims', () => {
      const pastTime = Math.floor(Date.now() / 1000) - 3600;
      createClaim(db, {
        agent_id: 'alice',
        claim_type: 'file',
        pattern: 'src/expired1.ts',
        expires_at: pastTime,
      });
      createClaim(db, {
        agent_id: 'alice',
        claim_type: 'file',
        pattern: 'src/expired2.ts',
        expires_at: pastTime - 100,
      });
      createClaim(db, { agent_id: 'bob', claim_type: 'file', pattern: 'src/active.ts' });

      const pruned = pruneExpiredClaims(db);
      expect(pruned).toBe(2);

      const remaining = getClaimsByType(db, 'file');
      expect(remaining.length).toBe(1);
      expect(remaining[0].pattern).toBe('src/active.ts');
    });
  });
});
