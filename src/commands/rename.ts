import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, getConfig, isAgentActive, renameAgent, createMessage } from '../db/queries.js';
import { isValidAgentId, normalizeAgentRef } from '../core/agents.js';
import { resolveAgentRef } from '../core/context.js';

export function renameCommand(): Command {
  return new Command('rename')
    .description('Rename an agent')
    .argument('<old>', 'current agent name (e.g., @alice)')
    .argument('<new>', 'new agent name (e.g., @bob)')
    .action((oldName: string, newName: string, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);

        // Normalize names (strip @ prefix)
        const oldId = resolveAgentRef(oldName, projectConfig ?? null);
        const newId = normalizeAgentRef(newName);

        // Validate new name
        if (!isValidAgentId(newId)) {
          throw new Error(
            `Invalid agent name: ${newId}\n` +
            `Names must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens.\n` +
            `Examples: alice, pm, eager-beaver, frontend-dev`
          );
        }

        // Check old agent exists
        const oldAgent = getAgent(db, oldId);
        if (!oldAgent) {
          throw new Error(`Agent not found: @${oldId}`);
        }

        // Check new name not taken by an active agent
        const staleHours = parseInt(getConfig(db, 'stale_hours') || '4', 10);
        const existingAgent = getAgent(db, newId);
        if (existingAgent && isAgentActive(db, newId, staleHours)) {
          throw new Error(`Agent @${newId} already exists and is active.`);
        }

        // If new name is taken by inactive agent, we need to handle this
        if (existingAgent) {
          throw new Error(
            `Agent @${newId} already exists (inactive).\n` +
            `Choose a different name or wait for the inactive agent to be cleaned up.`
          );
        }

        // Perform the rename
        renameAgent(db, oldId, newId);

        // Post system message about the rename
        const now = Math.floor(Date.now() / 1000);
        createMessage(db, {
          ts: now,
          from_agent: 'system',
          body: `@${oldId} renamed to @${newId}`,
          mentions: [oldId, newId],
          type: 'agent',
        });

        if (jsonMode) {
          console.log(JSON.stringify({
            old_id: oldId,
            new_id: newId,
            success: true,
          }));
        } else {
          console.log(`Renamed @${oldId} to @${newId}`);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
