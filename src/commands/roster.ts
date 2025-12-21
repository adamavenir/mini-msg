import { Command } from 'commander';
import path from 'path';
import Database from 'better-sqlite3';
import { getContext, handleError } from './shared.js';
import { getAllAgents, getClaimCountsByAgent } from '../db/queries.js';
import { initSchema } from '../db/schema.js';
import { readGlobalConfig } from '../core/config.js';
import { formatRelative } from '../core/time.js';
import type { Agent } from '../types.js';

interface RosterAgent {
  guid: string;
  agent_id: string;
  status: string | null;
  purpose: string | null;
  registered_at: string;
  last_seen: string;
  left_at: string | null;
  message_count: number;
  claim_count: number;
  channel_id?: string;
  channel_name?: string;
}

function getMessageCounts(db: Database.Database): Map<string, number> {
  const messageCounts = new Map<string, number>();
  const rows = db.prepare(`
    SELECT from_agent as agent_id, COUNT(*) as count
    FROM mm_messages
    GROUP BY from_agent
  `).all() as { agent_id: string; count: number }[];
  for (const row of rows) {
    messageCounts.set(row.agent_id, row.count);
  }
  return messageCounts;
}

function buildRosterAgent(
  agent: Agent,
  messageCounts: Map<string, number>,
  claimCounts: Map<string, number>,
  channelId?: string,
  channelName?: string
): RosterAgent {
  return {
    guid: agent.guid,
    agent_id: agent.agent_id,
    status: agent.status,
    purpose: agent.purpose,
    registered_at: new Date(agent.registered_at * 1000).toISOString(),
    last_seen: new Date(agent.last_seen * 1000).toISOString(),
    left_at: agent.left_at ? new Date(agent.left_at * 1000).toISOString() : null,
    message_count: messageCounts.get(agent.agent_id) || 0,
    claim_count: claimCounts.get(agent.agent_id) || 0,
    ...(channelId && { channel_id: channelId }),
    ...(channelName && { channel_name: channelName }),
  };
}

function formatAgentDisplay(agent: RosterAgent, showChannel: boolean): void {
  const leftStatus = agent.left_at ? '(left)' : '';
  const claimInfo = agent.claim_count > 0
    ? ` (${agent.claim_count} claim${agent.claim_count !== 1 ? 's' : ''})`
    : '';

  console.log(`  @${agent.agent_id}${leftStatus}${claimInfo}`);

  if (showChannel && agent.channel_name) {
    console.log(`    channel: ${agent.channel_name}`);
  }
  if (agent.status) {
    console.log(`    status: ${agent.status}`);
  }
  if (agent.purpose) {
    console.log(`    purpose: ${agent.purpose}`);
  }

  // Parse ISO strings back to unix timestamps for formatRelative
  const lastSeenTs = Math.floor(new Date(agent.last_seen).getTime() / 1000);
  const registeredTs = Math.floor(new Date(agent.registered_at).getTime() / 1000);

  console.log(`    last seen: ${formatRelative(lastSeenTs)}`);
  console.log(`    registered: ${formatRelative(registeredTs)}`);
  console.log(`    messages: ${agent.message_count}`);
}

export function rosterCommand(): Command {
  return new Command('roster')
    .description('List registered agents')
    .option('--local', 'show agents from local channel only (default)')
    .option('--global', 'show agents from all registered channels')
    .action(async (options, cmd) => {
      try {
        const jsonMode = cmd.optsWithGlobals().json || false;
        const showGlobal = options.global || false;

        const allAgents: RosterAgent[] = [];

        if (showGlobal) {
          // Get agents from all registered channels
          const config = readGlobalConfig();

          if (!config || Object.keys(config.channels).length === 0) {
            if (jsonMode) {
              console.log(JSON.stringify({ agents: [], total: 0 }, null, 2));
            } else {
              console.log('No channels registered');
            }
            return;
          }

          for (const [channelId, channel] of Object.entries(config.channels)) {
            const dbPath = path.join(channel.path, '.mm', 'mm.db');
            try {
              const db = new Database(dbPath);
              initSchema(db);

              const agents = getAllAgents(db);
              const messageCounts = getMessageCounts(db);
              const claimCounts = getClaimCountsByAgent(db);

              for (const agent of agents) {
                allAgents.push(buildRosterAgent(
                  agent,
                  messageCounts,
                  claimCounts,
                  channelId,
                  channel.name
                ));
              }

              db.close();
            } catch {
              // Skip channels that can't be opened
            }
          }
        } else {
          // Get agents from local channel only
          const { db, channelId } = getContext(cmd);

          const agents = getAllAgents(db);
          const messageCounts = getMessageCounts(db);
          const claimCounts = getClaimCountsByAgent(db);

          for (const agent of agents) {
            allAgents.push(buildRosterAgent(agent, messageCounts, claimCounts));
          }

          db.close();
        }

        // Sort by last_seen descending (most recent first)
        allAgents.sort((a, b) =>
          new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()
        );

        if (jsonMode) {
          console.log(JSON.stringify({
            agents: allAgents,
            total: allAgents.length
          }, null, 2));
        } else {
          if (allAgents.length === 0) {
            console.log('No agents registered');
          } else {
            const label = showGlobal ? 'REGISTERED AGENTS (global)' : 'REGISTERED AGENTS';
            console.log(`${label} (${allAgents.length}):`);
            for (const agent of allAgents) {
              formatAgentDisplay(agent, showGlobal);
            }
          }
        }
      } catch (error) {
        handleError(error);
      }
    });
}
