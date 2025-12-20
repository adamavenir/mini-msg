import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, updateAgent, createMessage, deleteClaimsByAgent } from '../db/queries.js';
import { extractMentions } from '../core/mentions.js';
import { resolveAgentRef } from '../core/context.js';

export function byeCommand(): Command {
  return new Command('bye')
    .description('Leave agent session')
    .argument('<agent>', 'agent name (e.g., @alice)')
    .argument('[message]', 'optional goodbye message to post')
    .action((agentArg: string, message: string | undefined, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);
        const agentId = resolveAgentRef(agentArg, projectConfig ?? null);

        // Check agent exists
        const agent = getAgent(db, agentId);
        if (!agent) {
          throw new Error(`Agent not found: @${agentId}`);
        }

        const now = Math.floor(Date.now() / 1000);

        // Clear any claims before leaving
        const clearedClaims = deleteClaimsByAgent(db, agentId);

        // Post goodbye message if provided (before marking as left)
        let postedMessage = null;
        if (message) {
          const mentions = extractMentions(message);
          postedMessage = createMessage(db, {
            ts: now,
            from_agent: agentId,
            body: message,
            mentions,
          });
        }

        // Set left_at timestamp
        updateAgent(db, agentId, {
          left_at: now,
        });

        if (jsonMode) {
          console.log(JSON.stringify({
            agent_id: agentId,
            status: 'left',
            message_id: postedMessage?.id || null,
            claims_cleared: clearedClaims,
          }));
        } else {
          console.log(`Goodbye, @${agentId}!`);
          if (postedMessage) console.log(`  Posted: [${postedMessage.id}] ${message}`);
          if (clearedClaims > 0) console.log(`  Released ${clearedClaims} claim${clearedClaims !== 1 ? 's' : ''}`);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
