import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getActiveAgents, getAllAgents, getConfig, getClaimCountsByAgent } from '../db/queries.js';
import { formatRelative } from '../core/time.js';

export function hereCommand(): Command {
  return new Command('here')
    .description('List active agents')
    .option('--all', 'include stale agents')
    .action(async (options, cmd) => {
      try {
        const { db, jsonMode } = getContext(cmd);

        let agents;
        if (options.all) {
          // Show all agents that haven't explicitly left (including stale ones)
          agents = getAllAgents(db).filter(a => a.left_at === null);
        } else {
          // Show only active (non-stale, non-left) agents
          const staleHours = parseInt(getConfig(db, 'stale_hours') || '4', 10);
          agents = getActiveAgents(db, staleHours);
        }

        // Get claim counts for all agents
        const claimCounts = getClaimCountsByAgent(db);
        const messageCounts = new Map<string, number>();
        const messageCountRows = db.prepare(`
          SELECT from_agent as agent_id, COUNT(*) as count
          FROM mm_messages
          GROUP BY from_agent
        `).all() as { agent_id: string; count: number }[];
        for (const row of messageCountRows) {
          messageCounts.set(row.agent_id, row.count);
        }

        if (jsonMode) {
          const payload = {
            agents: agents.map(agent => ({
              agent_id: agent.guid,
              display_name: agent.agent_id,
              goal: agent.goal,
              last_active: new Date(agent.last_seen * 1000).toISOString(),
              message_count: messageCounts.get(agent.agent_id) || 0,
              claim_count: claimCounts.get(agent.agent_id) || 0,
            })),
            total: agents.length,
          };
          console.log(JSON.stringify(payload, null, 2));
        } else {
          if (agents.length === 0) {
            console.log('No active agents');
          } else {
            console.log(`ACTIVE AGENTS (${agents.length}):`);
            for (const agent of agents) {
              const lastSeen = formatRelative(agent.last_seen);
              const claimCount = claimCounts.get(agent.agent_id) || 0;
              const claimInfo = claimCount > 0 ? ` (${claimCount} claim${claimCount !== 1 ? 's' : ''})` : '';
              const goal = agent.goal ? ` - ${agent.goal}` : '';
              console.log(`  @${agent.agent_id}${claimInfo}${goal}`);
              console.log(`    last seen: ${lastSeen}`);
            }
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
