import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAllClaims, getClaimsByAgent } from '../db/queries.js';
import { resolveAgentRef } from '../core/context.js';
import { formatRelative } from '../core/time.js';
import type { ClaimType } from '../types.js';

export function claimsCommand(): Command {
  return new Command('claims')
    .description('List active claims')
    .argument('[agent]', 'filter by agent name (e.g., @alice)')
    .option('--type <type>', 'filter by claim type (file, bd, issue)')
    .action((agentArg: string | undefined, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);

        let claims;
        if (agentArg) {
          const agentId = resolveAgentRef(agentArg, projectConfig ?? null);
          claims = getClaimsByAgent(db, agentId);
        } else {
          claims = getAllClaims(db);
        }

        // Filter by type if specified
        if (options.type) {
          const claimType = options.type as ClaimType;
          claims = claims.filter(c => c.claim_type === claimType);
        }

        if (jsonMode) {
          console.log(JSON.stringify(claims));
        } else {
          if (claims.length === 0) {
            if (agentArg) {
              const agentId = resolveAgentRef(agentArg, projectConfig ?? null);
              console.log(`No claims for @${agentId}`);
            } else {
              console.log('No active claims');
            }
          } else {
            console.log(`CLAIMS (${claims.length}):`);

            // Group by agent
            const byAgent = new Map<string, typeof claims>();
            for (const claim of claims) {
              const existing = byAgent.get(claim.agent_id) || [];
              existing.push(claim);
              byAgent.set(claim.agent_id, existing);
            }

            for (const [agentId, agentClaims] of byAgent) {
              console.log(`\n  @${agentId}:`);
              for (const claim of agentClaims) {
                const typePrefix = claim.claim_type === 'file' ? '' : `${claim.claim_type}:`;
                const age = formatRelative(claim.created_at);
                let expiry = '';
                if (claim.expires_at) {
                  const now = Math.floor(Date.now() / 1000);
                  if (claim.expires_at > now) {
                    const remaining = Math.round((claim.expires_at - now) / 60);
                    expiry = ` (${remaining}m left)`;
                  } else {
                    expiry = ' (expired)';
                  }
                }
                const reason = claim.reason ? ` - ${claim.reason}` : '';
                console.log(`    ${typePrefix}${claim.pattern} (${age})${expiry}${reason}`);
              }
            }
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
