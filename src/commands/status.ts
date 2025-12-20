import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, updateAgent, createClaim, createMessage, deleteClaimsByAgent, getClaimsByAgent, pruneExpiredClaims } from '../db/queries.js';
import { resolveAgentRef } from '../core/context.js';
import type { ClaimType } from '../types.js';

/**
 * Parse duration string to seconds.
 * Supports: 30m, 2h, 1d
 */
function parseDuration(duration: string): number {
  const match = duration.match(/^(\d+)(m|h|d)$/);
  if (!match) {
    throw new Error(`Invalid duration format: ${duration}. Use 30m, 2h, or 1d`);
  }

  const value = parseInt(match[1], 10);
  const unit = match[2];

  switch (unit) {
    case 'm':
      return value * 60;
    case 'h':
      return value * 3600;
    case 'd':
      return value * 86400;
    default:
      throw new Error(`Unknown time unit: ${unit}`);
  }
}

/**
 * Strip # prefix from bd/issue values.
 */
function stripHash(value: string): string {
  return value.startsWith('#') ? value.substring(1) : value;
}

export function statusCommand(): Command {
  return new Command('status')
    .description('Update status with optional claims')
    .argument('<agent>', 'agent name (e.g., @alice)')
    .argument('[message]', 'status message (becomes goal)')
    .option('--file <path>', 'claim a single file')
    .option('--files <patterns>', 'claim multiple files (comma-separated globs)')
    .option('--bd <id>', 'claim a beads issue')
    .option('--issue <id>', 'claim a GitHub issue')
    .option('--ttl <duration>', 'expiration time for claims (e.g., 2h, 30m, 1d)')
    .option('--clear', 'clear all claims and reset goal')
    .action((agentArg: string, message: string | undefined, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);
        const agentId = resolveAgentRef(agentArg, projectConfig ?? null);

        // Verify agent exists
        const agent = getAgent(db, agentId);
        if (!agent) {
          throw new Error(`Agent not found: @${agentId}`);
        }

        // Handle --clear
        if (options.clear) {
          const existingClaims = getClaimsByAgent(db, agentId);
          const clearedCount = deleteClaimsByAgent(db, agentId);
          updateAgent(db, agentId, { goal: null, last_seen: Math.floor(Date.now() / 1000) });

          // Post message about clearing status
          createMessage(db, {
            from_agent: agentId,
            body: clearedCount > 0
              ? `status cleared (released ${clearedCount} claim${clearedCount !== 1 ? 's' : ''})`
              : 'status cleared',
            mentions: [],
          });

          if (jsonMode) {
            console.log(JSON.stringify({
              agent_id: agentId,
              action: 'cleared',
              claims_released: clearedCount,
            }));
          } else {
            console.log(`@${agentId} status cleared`);
            if (clearedCount > 0) {
              console.log(`  Released ${clearedCount} claim${clearedCount !== 1 ? 's' : ''}`);
            }
          }

          db.close();
          return;
        }

        // Prune expired claims first
        pruneExpiredClaims(db);

        // Calculate expiration if TTL provided
        let expiresAt: number | null = null;
        if (options.ttl) {
          const seconds = parseDuration(options.ttl);
          expiresAt = Math.floor(Date.now() / 1000) + seconds;
        }

        // Collect claims
        const claims: { type: ClaimType; pattern: string }[] = [];

        if (options.file) {
          claims.push({ type: 'file', pattern: options.file });
        }
        if (options.files) {
          const patterns = options.files.split(',').map((p: string) => p.trim());
          for (const pattern of patterns) {
            claims.push({ type: 'file', pattern });
          }
        }
        if (options.bd) {
          claims.push({ type: 'bd', pattern: stripHash(options.bd) });
        }
        if (options.issue) {
          claims.push({ type: 'issue', pattern: stripHash(options.issue) });
        }

        // Create claims
        const created: { type: ClaimType; pattern: string }[] = [];
        for (const claim of claims) {
          createClaim(db, {
            agent_id: agentId,
            claim_type: claim.type,
            pattern: claim.pattern,
            reason: message ?? null,
            expires_at: expiresAt,
          });
          created.push(claim);
        }

        // Update goal if message provided
        if (message) {
          updateAgent(db, agentId, { goal: message, last_seen: Math.floor(Date.now() / 1000) });
        } else {
          updateAgent(db, agentId, { last_seen: Math.floor(Date.now() / 1000) });
        }

        // Build status message
        let body = message || 'status update';
        if (created.length > 0) {
          const claimList = created.map(c => {
            if (c.type === 'file') return c.pattern;
            return `${c.type}:${c.pattern}`;
          }).join(', ');
          body = message
            ? `${message} [claimed: ${claimList}]`
            : `claimed: ${claimList}`;
        }

        // Post status message
        createMessage(db, {
          from_agent: agentId,
          body,
          mentions: [],
        });

        if (jsonMode) {
          console.log(JSON.stringify({
            agent_id: agentId,
            goal: message ?? null,
            claims: created,
            expires_at: expiresAt,
          }));
        } else {
          if (message) {
            console.log(`@${agentId}: ${message}`);
          } else {
            console.log(`@${agentId} status updated`);
          }
          if (created.length > 0) {
            console.log('  Claimed:');
            for (const claim of created) {
              const typePrefix = claim.type === 'file' ? '' : `${claim.type}:`;
              console.log(`    ${typePrefix}${claim.pattern}`);
            }
            if (expiresAt) {
              const ttlMinutes = Math.round((expiresAt - Math.floor(Date.now() / 1000)) / 60);
              console.log(`  Expires in ${ttlMinutes} minutes`);
            }
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
