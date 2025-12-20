import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, getActiveAgents, getAllAgents, getConfig } from '../db/queries.js';
import { formatRelative, isStale } from '../core/time.js';
import { matchesPrefix } from '../core/agents.js';
import { resolveAgentRef } from '../core/context.js';

export function whoCommand(): Command {
  return new Command('who')
    .description('Show agent details')
    .argument('<agent>', 'agent ID or "here" for all active')
    .action(async (agentRef: string, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);

        if (agentRef === 'here') {
          // Show all active agents with details
          const staleHours = parseInt(getConfig(db, 'stale_hours') || '4', 10);
          const agents = getActiveAgents(db, staleHours);

          if (jsonMode) {
            console.log(JSON.stringify(agents));
          } else {
            if (agents.length === 0) {
              console.log('No active agents');
            } else {
              for (const agent of agents) {
                displayAgent(agent, staleHours);
                console.log();
              }
            }
          }
        } else {
          // Find agent by exact match or prefix
          const resolvedRef = resolveAgentRef(agentRef, projectConfig ?? null);
          let agent = getAgent(db, resolvedRef);

          if (!agent) {
            // Try prefix matching
            const allAgents = getAllAgents(db);
            const matches = allAgents.filter(a => matchesPrefix(a.agent_id, resolvedRef));

            if (matches.length === 0) {
              throw new Error(`Agent not found: ${agentRef}`);
            } else if (matches.length === 1) {
              agent = matches[0];
            } else {
              throw new Error(
                `Ambiguous prefix '${agentRef}' matches: ${matches.map(a => a.agent_id).join(', ')}`
              );
            }
          }

          if (jsonMode) {
            console.log(JSON.stringify(agent));
          } else {
            const staleHours = parseInt(getConfig(db, 'stale_hours') || '4', 10);
            displayAgent(agent, staleHours);
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}

function displayAgent(agent: any, staleHours: number): void {
  const registeredAt = formatRelative(agent.registered_at);
  const lastSeen = formatRelative(agent.last_seen);
  const status = agent.left_at
    ? 'left'
    : isStale(agent.last_seen, staleHours)
    ? 'stale'
    : 'active';

  console.log(agent.agent_id);
  if (agent.goal) console.log(`  Goal: ${agent.goal}`);
  if (agent.bio) console.log(`  Bio: ${agent.bio}`);
  console.log(`  Registered: ${registeredAt}`);
  console.log(`  Last seen: ${lastSeen}`);
  console.log(`  Status: ${status}`);
}
