import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, updateAgent, createMessage } from '../db/queries.js';
import { extractMentions } from '../core/mentions.js';
import { writeClaudeEnv } from './hooks.js';
import { resolveAgentRef } from '../core/context.js';

export function backCommand(): Command {
  return new Command('back')
    .description('Rejoin as a previous agent')
    .argument('<agent>', 'agent name (e.g., @alice)')
    .argument('[message]', 'optional rejoin message to post')
    .action((agentArg: string, message: string | undefined, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);
        const agentId = resolveAgentRef(agentArg, projectConfig ?? null);

        // Check agent exists
        const agent = getAgent(db, agentId);
        if (!agent) {
          throw new Error(`Agent not found: @${agentId}`);
        }

        // Update last_seen and clear left_at
        const now = Math.floor(Date.now() / 1000);
        updateAgent(db, agentId, {
          last_seen: now,
          left_at: null,
        });

        // Post rejoin message if provided
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

        // Write to CLAUDE_ENV_FILE if in hooks context
        const wroteEnv = writeClaudeEnv(agentId);

        if (jsonMode) {
          console.log(JSON.stringify({
            agent_id: agentId,
            status: 'active',
            message_id: postedMessage?.id || null,
            claude_env: wroteEnv,
          }));
        } else {
          console.log(`Welcome back, @${agentId}!`);
          if (agent.goal) console.log(`  Goal: ${agent.goal}`);
          if (postedMessage) console.log(`  Posted: [${postedMessage.id}] ${message}`);
          if (wroteEnv) console.log(`  Registered with Claude hooks`);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
