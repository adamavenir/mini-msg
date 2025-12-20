import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, createClaim, createMessage, pruneExpiredClaims } from '../db/queries.js';
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

export function claimCommand(): Command {
  return new Command('claim')
    .description('Claim resources to prevent collision')
    .argument('<agent>', 'agent name (e.g., @alice)')
    .option('--file <path>', 'claim a single file')
    .option('--files <patterns>', 'claim multiple files (comma-separated globs)')
    .option('--bd <id>', 'claim a beads issue')
    .option('--issue <id>', 'claim a GitHub issue')
    .option('--ttl <duration>', 'expiration time (e.g., 2h, 30m, 1d)')
    .option('--reason <reason>', 'reason for claim')
    .action((agentArg: string, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);
        const agentId = resolveAgentRef(agentArg, projectConfig ?? null);

        // Verify agent exists
        const agent = getAgent(db, agentId);
        if (!agent) {
          throw new Error(`Agent not found: @${agentId}`);
        }

        // Prune expired claims first
        pruneExpiredClaims(db);

        // Calculate expiration if TTL provided
        let expiresAt: number | null = null;
        if (options.ttl) {
          const seconds = parseDuration(options.ttl);
          expiresAt = Math.floor(Date.now() / 1000) + seconds;
        }

        const claims: { type: ClaimType; pattern: string }[] = [];

        // Collect file claims
        if (options.file) {
          claims.push({ type: 'file', pattern: options.file });
        }
        if (options.files) {
          const patterns = options.files.split(',').map((p: string) => p.trim());
          for (const pattern of patterns) {
            claims.push({ type: 'file', pattern });
          }
        }

        // Collect bd claims (strip # prefix)
        if (options.bd) {
          claims.push({ type: 'bd', pattern: stripHash(options.bd) });
        }

        // Collect issue claims (strip # prefix)
        if (options.issue) {
          claims.push({ type: 'issue', pattern: stripHash(options.issue) });
        }

        if (claims.length === 0) {
          throw new Error('No claims specified. Use --file, --files, --bd, or --issue');
        }

        // Create claims
        const created: { type: ClaimType; pattern: string }[] = [];
        for (const claim of claims) {
          createClaim(db, {
            agent_id: agentId,
            claim_type: claim.type,
            pattern: claim.pattern,
            reason: options.reason ?? null,
            expires_at: expiresAt,
          });
          created.push(claim);
        }

        // Post message about claims
        const claimList = created.map(c => {
          if (c.type === 'file') return c.pattern;
          return `${c.type}:${c.pattern}`;
        }).join(', ');

        createMessage(db, {
          from_agent: agentId,
          body: `claimed: ${claimList}`,
          mentions: [],
        });

        if (jsonMode) {
          console.log(JSON.stringify({
            agent_id: agentId,
            claims: created,
            expires_at: expiresAt,
          }));
        } else {
          console.log(`@${agentId} claimed:`);
          for (const claim of created) {
            const typePrefix = claim.type === 'file' ? '' : `${claim.type}:`;
            console.log(`  ${typePrefix}${claim.pattern}`);
          }
          if (expiresAt) {
            const ttlMinutes = Math.round((expiresAt - Math.floor(Date.now() / 1000)) / 60);
            console.log(`  Expires in ${ttlMinutes} minutes`);
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
