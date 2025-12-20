import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, deleteClaimsByAgent, deleteClaim, createMessage, getClaimsByAgent } from '../db/queries.js';
import { resolveAgentRef } from '../core/context.js';

/**
 * Strip # prefix from bd/issue values.
 */
function stripHash(value: string): string {
  return value.startsWith('#') ? value.substring(1) : value;
}

export function clearCommand(): Command {
  return new Command('clear')
    .description('Clear claims for an agent')
    .argument('<agent>', 'agent name (e.g., @alice)')
    .option('--file <path>', 'clear a specific file claim')
    .option('--bd <id>', 'clear a specific beads issue claim')
    .option('--issue <id>', 'clear a specific GitHub issue claim')
    .action((agentArg: string, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);
        const agentId = resolveAgentRef(agentArg, projectConfig ?? null);

        // Verify agent exists
        const agent = getAgent(db, agentId);
        if (!agent) {
          throw new Error(`Agent not found: @${agentId}`);
        }

        let cleared = 0;
        const clearedItems: string[] = [];

        // Clear specific claims if flags provided
        if (options.file) {
          if (deleteClaim(db, 'file', options.file)) {
            cleared++;
            clearedItems.push(options.file);
          }
        }

        if (options.bd) {
          const pattern = stripHash(options.bd);
          if (deleteClaim(db, 'bd', pattern)) {
            cleared++;
            clearedItems.push(`bd:${pattern}`);
          }
        }

        if (options.issue) {
          const pattern = stripHash(options.issue);
          if (deleteClaim(db, 'issue', pattern)) {
            cleared++;
            clearedItems.push(`issue:${pattern}`);
          }
        }

        // If no specific flags, clear all claims for the agent
        if (!options.file && !options.bd && !options.issue) {
          const existingClaims = getClaimsByAgent(db, agentId);
          cleared = deleteClaimsByAgent(db, agentId);
          for (const claim of existingClaims) {
            if (claim.claim_type === 'file') {
              clearedItems.push(claim.pattern);
            } else {
              clearedItems.push(`${claim.claim_type}:${claim.pattern}`);
            }
          }
        }

        // Post message if anything was cleared
        if (cleared > 0) {
          createMessage(db, {
            from_agent: agentId,
            body: `cleared claims: ${clearedItems.join(', ')}`,
            mentions: [],
          });
        }

        if (jsonMode) {
          console.log(JSON.stringify({
            agent_id: agentId,
            cleared,
            items: clearedItems,
          }));
        } else {
          if (cleared === 0) {
            console.log(`No claims to clear for @${agentId}`);
          } else {
            console.log(`@${agentId} cleared ${cleared} claim${cleared !== 1 ? 's' : ''}:`);
            for (const item of clearedItems) {
              console.log(`  ${item}`);
            }
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
