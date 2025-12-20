import { Command } from 'commander';
import fs from 'fs';
import path from 'path';
import readline from 'readline';
import Database from 'better-sqlite3';
import { discoverProject } from '../core/project.js';
import { generateGuid } from '../core/guid.js';
import { registerChannel } from '../core/config.js';
import {
  rebuildDatabaseFromJsonl,
  updateProjectConfig,
  type AgentJsonlRecord,
  type MessageJsonlRecord,
  type ProjectConfig,
} from '../db/jsonl.js';
import { handleError } from './shared.js';

type TableColumn = { name: string; type: string; notnull: number; pk: number };

type AgentRow = {
  guid?: string;
  agent_id: string;
  goal: string | null;
  bio: string | null;
  registered_at: number;
  last_seen: number;
  left_at: number | null;
};

type MessageRow = {
  guid?: string;
  id?: number;
  ts: number;
  from_agent: string;
  body: string;
  mentions: string | null;
  type: string | null;
  reply_to: number | string | null;
  edited_at: number | null;
  archived_at: number | null;
};

function promptForChannelName(defaultName: string): Promise<string> {
  if (!process.stdin.isTTY) {
    return Promise.resolve(defaultName);
  }

  return new Promise((resolve) => {
    const rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
    });
    rl.question(`Channel name for this project? [${defaultName}]: `, (answer) => {
      rl.close();
      const trimmed = answer.trim();
      resolve(trimmed === '' ? defaultName : trimmed);
    });
  });
}

function tableExists(db: Database.Database, table: string): boolean {
  const result = db.prepare(`
    SELECT name FROM sqlite_master
    WHERE type='table' AND name=?
  `).get(table) as { name: string } | undefined;

  return result !== undefined;
}

function getColumns(db: Database.Database, table: string): TableColumn[] {
  return db.prepare(`PRAGMA table_info(${table})`).all() as TableColumn[];
}

function ensureDir(dirPath: string): void {
  if (!fs.existsSync(dirPath)) {
    fs.mkdirSync(dirPath, { recursive: true });
  }
}

function writeJsonlFile(filePath: string, records: unknown[]): void {
  ensureDir(path.dirname(filePath));
  if (records.length === 0) {
    fs.writeFileSync(filePath, '', 'utf8');
    return;
  }

  const contents = records.map(record => JSON.stringify(record)).join('\n') + '\n';
  fs.writeFileSync(filePath, contents, 'utf8');
}

function parseMentions(raw: string | null): string[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (Array.isArray(parsed)) {
      return parsed.filter((entry): entry is string => typeof entry === 'string');
    }
  } catch {
    return [];
  }
  return [];
}

function generateUniqueGuid(prefix: string, used: Set<string>): string {
  let guid = generateGuid(prefix);
  while (used.has(guid)) {
    guid = generateGuid(prefix);
  }
  used.add(guid);
  return guid;
}

export function migrateCommand(): Command {
  return new Command('migrate')
    .description('Migrate mm project from v0.1.0 to v0.2.0 format')
    .action(async () => {
      try {
        const project = discoverProject();
        const mmDir = path.dirname(project.dbPath);
        const configPath = path.join(mmDir, 'mm-config.json');

        if (fs.existsSync(configPath)) {
          throw new Error('mm-config.json already exists. Nothing to migrate.');
        }

        const backupDir = path.join(project.root, '.mm.bak');
        if (fs.existsSync(backupDir)) {
          throw new Error('Backup already exists at .mm.bak/. Move it aside before migrating.');
        }

        const defaultName = path.basename(project.root);
        const channelName = await promptForChannelName(defaultName);
        const channelId = generateGuid('ch');

        const sourceDb = new Database(project.dbPath, { readonly: true });
        if (!tableExists(sourceDb, 'mm_agents') || !tableExists(sourceDb, 'mm_messages')) {
          sourceDb.close();
          throw new Error('Missing mm tables in database. Nothing to migrate.');
        }

        const agentColumns = getColumns(sourceDb, 'mm_agents').map(col => col.name);
        const hasAgentGuid = agentColumns.includes('guid');
        const agentRows = hasAgentGuid
          ? sourceDb.prepare(`
              SELECT guid, agent_id, goal, bio, registered_at, last_seen, left_at
              FROM mm_agents
            `).all() as AgentRow[]
          : sourceDb.prepare(`
              SELECT agent_id, goal, bio, registered_at, last_seen, left_at
              FROM mm_agents
            `).all() as AgentRow[];

        const messageColumns = getColumns(sourceDb, 'mm_messages').map(col => col.name);
        const hasMessageGuid = messageColumns.includes('guid');
        const hasMessageId = messageColumns.includes('id');

        const messageSelectFields = [
          hasMessageGuid ? 'guid' : null,
          hasMessageId ? 'id' : null,
          'ts',
          'from_agent',
          'body',
          'mentions',
          'type',
          'reply_to',
          'edited_at',
          'archived_at',
        ].filter(Boolean).join(', ');

        const messageOrder = hasMessageId ? 'ts ASC, id ASC' : 'ts ASC';
        const messageRows = sourceDb.prepare(`
          SELECT ${messageSelectFields}
          FROM mm_messages
          ORDER BY ${messageOrder}
        `).all() as MessageRow[];

        const readReceiptColumns = tableExists(sourceDb, 'mm_read_receipts')
          ? getColumns(sourceDb, 'mm_read_receipts').map(col => col.name)
          : [];
        const hasReceiptGuid = readReceiptColumns.includes('message_guid');
        const readReceipts = tableExists(sourceDb, 'mm_read_receipts')
          ? sourceDb.prepare(`
              SELECT ${hasReceiptGuid ? 'message_guid' : 'message_id'}, agent_prefix, read_at
              FROM mm_read_receipts
            `).all() as { message_guid?: string; message_id?: number; agent_prefix: string; read_at: number }[]
          : [];

        sourceDb.close();

        const usedAgentGuids = new Set<string>();
        const knownAgents: NonNullable<ProjectConfig['known_agents']> = {};
        const agentsJsonl: AgentJsonlRecord[] = [];

        for (const agent of agentRows) {
          let guid = agent.guid;
          if (!guid || usedAgentGuids.has(guid)) {
            guid = generateUniqueGuid('usr', usedAgentGuids);
          } else {
            usedAgentGuids.add(guid);
          }

          const createdAt = new Date(agent.registered_at * 1000).toISOString();
          const status = agent.left_at ? 'inactive' : 'active';
          const globalName = channelName ? `${channelName}-${agent.agent_id}` : agent.agent_id;

          knownAgents[guid] = {
            name: agent.agent_id,
            global_name: globalName,
            home_channel: channelId,
            created_at: createdAt,
            status,
          };

          agentsJsonl.push({
            type: 'agent',
            id: guid,
            name: agent.agent_id,
            global_name: globalName,
            home_channel: channelId,
            created_at: createdAt,
            status,
            agent_id: agent.agent_id,
            goal: agent.goal ?? null,
            bio: agent.bio ?? null,
            registered_at: agent.registered_at,
            last_seen: agent.last_seen,
            left_at: agent.left_at ?? null,
          });
        }

        const usedMessageGuids = new Set<string>();
        const idToGuid = new Map<number, string>();
        const messageGuids: string[] = [];
        const messagesJsonl: MessageJsonlRecord[] = [];

        if (!hasMessageGuid && !hasMessageId) {
          throw new Error('Could not locate message IDs for migration.');
        }

        for (let i = 0; i < messageRows.length; i += 1) {
          const message = messageRows[i];
          let guid = message.guid;
          if (!guid || usedMessageGuids.has(guid)) {
            guid = generateUniqueGuid('msg', usedMessageGuids);
          } else {
            usedMessageGuids.add(guid);
          }

          messageGuids[i] = guid;
          if (message.id !== undefined) {
            idToGuid.set(message.id, guid);
          }
        }

        for (let i = 0; i < messageRows.length; i += 1) {
          const message = messageRows[i];
          const guid = messageGuids[i];

          let replyTo: string | null = null;
          if (message.reply_to !== null && message.reply_to !== undefined) {
            if (typeof message.reply_to === 'number') {
              replyTo = idToGuid.get(message.reply_to) ?? null;
            } else if (!hasMessageGuid) {
              const parsed = Number(message.reply_to);
              replyTo = Number.isNaN(parsed) ? null : (idToGuid.get(parsed) ?? null);
            } else {
              replyTo = String(message.reply_to);
            }
          }

          const messageType = message.type === 'user' ? 'user' : 'agent';
          messagesJsonl.push({
            type: 'message',
            id: guid,
            channel_id: channelId,
            from_agent: message.from_agent,
            body: message.body,
            mentions: parseMentions(message.mentions),
            message_type: messageType,
            reply_to: replyTo,
            ts: message.ts,
            edited_at: message.edited_at ?? null,
            archived_at: message.archived_at ?? null,
          });
        }

        fs.cpSync(mmDir, backupDir, { recursive: true });

        updateProjectConfig(project.dbPath, {
          version: 1,
          channel_id: channelId,
          channel_name: channelName,
          created_at: new Date().toISOString(),
          known_agents: knownAgents,
        });

        writeJsonlFile(path.join(mmDir, 'agents.jsonl'), agentsJsonl);
        writeJsonlFile(path.join(mmDir, 'messages.jsonl'), messagesJsonl);

        const db = new Database(project.dbPath);
        db.pragma('foreign_keys = ON');
        db.pragma('journal_mode = WAL');
        db.pragma('busy_timeout = 5000');
        rebuildDatabaseFromJsonl(db, project.dbPath);

        const migrateExtras = db.transaction(() => {
          if (readReceipts.length > 0) {
            const insertReceipt = db.prepare(`
              INSERT OR IGNORE INTO mm_read_receipts (message_guid, agent_prefix, read_at)
              VALUES (?, ?, ?)
            `);
            for (const row of readReceipts) {
              let messageGuid = row.message_guid;
              if (!messageGuid && row.message_id !== undefined) {
                messageGuid = idToGuid.get(row.message_id);
              }
              if (!messageGuid) continue;
              insertReceipt.run(messageGuid, row.agent_prefix, row.read_at);
            }
          }
        });

        migrateExtras();
        db.close();

        registerChannel(channelId, channelName, project.root);
        console.log(`✓ Registered channel ${channelId} as '${channelName}'`);
        console.log(`Migration complete. Backup at .mm.bak/`);
        console.log(`Migrated ${agentsJsonl.length} agents and ${messagesJsonl.length} messages.`);
      } catch (error) {
        handleError(error);
      }
    });
}
