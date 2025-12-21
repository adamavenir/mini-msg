import { Command } from 'commander';
import fs from 'fs';
import path from 'path';
import Database from 'better-sqlite3';
import { discoverProject } from '../core/project.js';
import { readGlobalConfig } from '../core/config.js';
import { initSchema } from '../db/schema.js';
import { readProjectConfig } from '../db/jsonl.js';
import { getAllAgents, getAllConfig, getClaimCountsByAgent } from '../db/queries.js';
import { formatRelative } from '../core/time.js';
import type { Agent, ConfigEntry } from '../types.js';

interface AgentInfo {
  guid: string;
  agent_id: string;
  goal: string | null;
  bio: string | null;
  registered_at: string;
  last_seen: string;
  left_at: string | null;
  message_count: number;
  claim_count: number;
}

interface ChannelInfo {
  initialized: boolean;
  channel_id?: string;
  channel_name?: string;
  path?: string;
  created_at?: string;
  last_activity?: string;
  message_count?: number;
  agent_count?: number;
  config?: Record<string, string>;
  agents?: AgentInfo[];
}

function getChannelInfo(dbPath: string, projectRoot: string): ChannelInfo | null {
  const mmDir = path.join(projectRoot, '.mm');
  if (!fs.existsSync(mmDir)) {
    return null;
  }

  const fullDbPath = path.join(mmDir, 'mm.db');
  if (!fs.existsSync(fullDbPath)) {
    return null;
  }

  try {
    const db = new Database(fullDbPath);
    initSchema(db);

    // Get config
    const configEntries = getAllConfig(db);
    const config: Record<string, string> = {};
    for (const entry of configEntries) {
      config[entry.key] = entry.value;
    }

    // Get agents with counts
    const agents = getAllAgents(db);
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

    // Get total message count
    const totalMessages = db.prepare('SELECT COUNT(*) as count FROM mm_messages').get() as { count: number };

    // Get last activity
    const lastMessage = db.prepare('SELECT MAX(ts) as ts FROM mm_messages').get() as { ts: number | null };

    // Get project config for created_at
    const projectConfig = readProjectConfig(fullDbPath);

    const agentInfos: AgentInfo[] = agents.map(agent => ({
      guid: agent.guid,
      agent_id: agent.agent_id,
      goal: agent.goal,
      bio: agent.bio,
      registered_at: new Date(agent.registered_at * 1000).toISOString(),
      last_seen: new Date(agent.last_seen * 1000).toISOString(),
      left_at: agent.left_at ? new Date(agent.left_at * 1000).toISOString() : null,
      message_count: messageCounts.get(agent.agent_id) || 0,
      claim_count: claimCounts.get(agent.agent_id) || 0,
    }));

    // Sort agents by last_seen descending
    agentInfos.sort((a, b) =>
      new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()
    );

    db.close();

    return {
      initialized: true,
      channel_id: config.channel_id,
      channel_name: config.channel_name,
      path: projectRoot,
      created_at: projectConfig?.created_at,
      last_activity: lastMessage.ts ? new Date(lastMessage.ts * 1000).toISOString() : undefined,
      message_count: totalMessages.count,
      agent_count: agents.length,
      config,
      agents: agentInfos,
    };
  } catch {
    return null;
  }
}

function formatChannelDisplay(info: ChannelInfo, showPath: boolean = false): void {
  if (!info.initialized) {
    console.log('Not initialized');
    console.log('Run: mm init');
    return;
  }

  console.log(`Channel: ${info.channel_name} (${info.channel_id})`);
  if (showPath && info.path) {
    console.log(`  path: ${info.path}`);
  }
  if (info.created_at) {
    const createdTs = Math.floor(new Date(info.created_at).getTime() / 1000);
    console.log(`  created: ${formatRelative(createdTs)}`);
  }
  if (info.last_activity) {
    const lastTs = Math.floor(new Date(info.last_activity).getTime() / 1000);
    console.log(`  last activity: ${formatRelative(lastTs)}`);
  }
  console.log(`  messages: ${info.message_count}`);
  console.log(`  agents: ${info.agent_count}`);

  // Show non-standard config
  if (info.config) {
    const standardKeys = new Set(['channel_id', 'channel_name']);
    const customConfig = Object.entries(info.config).filter(([k]) => !standardKeys.has(k));
    if (customConfig.length > 0) {
      console.log('  config:');
      for (const [key, value] of customConfig) {
        console.log(`    ${key}: ${value}`);
      }
    }
  }

  if (info.agents && info.agents.length > 0) {
    console.log(`  roster (${info.agents.length}):`);
    for (const agent of info.agents) {
      const status = agent.left_at ? ' (left)' : '';
      const lastSeenTs = Math.floor(new Date(agent.last_seen).getTime() / 1000);
      console.log(`    @${agent.agent_id}${status} - ${agent.message_count} msgs, last seen ${formatRelative(lastSeenTs)}`);
    }
  }
}

export function infoCommand(): Command {
  return new Command('info')
    .description('Show channel information and roster')
    .option('--global', 'show info for all registered channels')
    .action(async (options, cmd) => {
      const jsonMode = cmd.optsWithGlobals().json || false;
      const showGlobal = options.global || false;

      if (showGlobal) {
        const globalConfig = readGlobalConfig();

        if (!globalConfig || Object.keys(globalConfig.channels).length === 0) {
          if (jsonMode) {
            console.log(JSON.stringify({ channels: [] }, null, 2));
          } else {
            console.log('No channels registered');
          }
          return;
        }

        const channels: ChannelInfo[] = [];

        for (const [channelId, channel] of Object.entries(globalConfig.channels)) {
          const info = getChannelInfo(path.join(channel.path, '.mm', 'mm.db'), channel.path);
          if (info) {
            channels.push(info);
          }
        }

        if (jsonMode) {
          console.log(JSON.stringify({ channels }, null, 2));
        } else {
          console.log(`Channels (${channels.length}):\n`);
          for (const info of channels) {
            formatChannelDisplay(info, true);
            console.log('');
          }
        }
      } else {
        // Local channel info
        let project;
        try {
          project = discoverProject();
        } catch {
          if (jsonMode) {
            console.log(JSON.stringify({ initialized: false }, null, 2));
          } else {
            console.log('Not initialized');
            console.log('Run: mm init');
          }
          return;
        }

        const info = getChannelInfo(path.join(project.root, '.mm', 'mm.db'), project.root);

        if (!info) {
          if (jsonMode) {
            console.log(JSON.stringify({ initialized: false }, null, 2));
          } else {
            console.log('Not initialized');
            console.log('Run: mm init');
          }
          return;
        }

        if (jsonMode) {
          console.log(JSON.stringify(info, null, 2));
        } else {
          formatChannelDisplay(info);
        }
      }
    });
}
