import fs from 'fs';
import path from 'path';
import type Database from 'better-sqlite3';
import type { Agent, Message, MessageType } from '../types.js';
import { initSchema } from './schema.js';

const MESSAGES_FILE = 'messages.jsonl';
const AGENTS_FILE = 'agents.jsonl';
const PROJECT_CONFIG_FILE = 'mm-config.json';

export interface MessageJsonlRecord {
  type: 'message';
  id: string;
  channel_id: string | null;
  from_agent: string;
  body: string;
  mentions: string[];
  message_type: MessageType;
  reply_to: string | null;
  ts: number;
  edited_at: number | null;
  archived_at: number | null;
}

export interface MessageUpdateJsonlRecord {
  type: 'message_update';
  id: string;
  body?: string;
  edited_at?: number | null;
  archived_at?: number | null;
}

export interface AgentJsonlRecord {
  type: 'agent';
  id: string;
  name: string;
  global_name?: string;
  home_channel?: string | null;
  created_at?: string;
  status?: 'active' | 'inactive';
  agent_id: string;
  goal: string | null;
  bio: string | null;
  registered_at: number;
  last_seen: number;
  left_at: number | null;
}

export interface ProjectConfig {
  version: number;
  channel_id?: string;
  channel_name?: string;
  created_at?: string;
  known_agents?: Record<string, {
    name: string;
    global_name?: string;
    home_channel?: string | null;
    created_at?: string;
    first_seen?: string;
    status?: 'active' | 'inactive';
    nicks?: string[];
  }>;
  [key: string]: unknown;
}

function resolveMmDir(projectPath: string): string {
  if (projectPath.endsWith('.db')) {
    return path.dirname(projectPath);
  }
  if (path.basename(projectPath) === '.mm') {
    return projectPath;
  }
  return path.join(projectPath, '.mm');
}

function ensureDir(dirPath: string): void {
  if (!fs.existsSync(dirPath)) {
    fs.mkdirSync(dirPath, { recursive: true });
  }
}

function appendJsonLine(filePath: string, record: unknown): void {
  ensureDir(path.dirname(filePath));
  fs.appendFileSync(filePath, JSON.stringify(record) + '\n', 'utf8');
}

function touchDatabaseFile(projectPath: string): void {
  const dbPath = projectPath.endsWith('.db')
    ? projectPath
    : path.join(resolveMmDir(projectPath), 'mm.db');
  if (!fs.existsSync(dbPath)) return;
  const now = new Date();
  fs.utimesSync(dbPath, now, now);
}

function readJsonlFile<T>(filePath: string): T[] {
  if (!fs.existsSync(filePath)) {
    return [];
  }

  const contents = fs.readFileSync(filePath, 'utf8');
  const lines = contents.split('\n');
  const records: T[] = [];

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    try {
      records.push(JSON.parse(trimmed) as T);
    } catch {
      continue;
    }
  }

  return records;
}

export function appendMessage(projectPath: string, message: Message): void {
  const mmDir = resolveMmDir(projectPath);
  const record: MessageJsonlRecord = {
    type: 'message',
    id: message.id,
    channel_id: message.channel_id ?? null,
    from_agent: message.from_agent,
    body: message.body,
    mentions: message.mentions,
    message_type: message.type,
    reply_to: message.reply_to ?? null,
    ts: message.ts,
    edited_at: message.edited_at ?? null,
    archived_at: message.archived_at ?? null,
  };

  appendJsonLine(path.join(mmDir, MESSAGES_FILE), record);
  touchDatabaseFile(projectPath);
}

export function appendMessageUpdate(projectPath: string, update: {
  id: string;
  body?: string;
  edited_at?: number | null;
  archived_at?: number | null;
}): void {
  const mmDir = resolveMmDir(projectPath);
  const record: MessageUpdateJsonlRecord = {
    type: 'message_update',
    id: update.id,
    ...update,
  };

  appendJsonLine(path.join(mmDir, MESSAGES_FILE), record);
  touchDatabaseFile(projectPath);
}

export function appendAgent(projectPath: string, agent: Agent): void {
  const mmDir = resolveMmDir(projectPath);
  const projectConfig = readProjectConfig(projectPath);
  const channelName = projectConfig?.channel_name;
  const channelId = projectConfig?.channel_id ?? null;
  const name = agent.agent_id;
  const globalName = channelName ? `${channelName}-${name}` : name;
  const createdAt = new Date(agent.registered_at * 1000).toISOString();
  const status = agent.left_at ? 'inactive' : 'active';
  const record: AgentJsonlRecord = {
    type: 'agent',
    id: agent.guid,
    name,
    global_name: globalName,
    home_channel: channelId,
    created_at: createdAt,
    status,
    agent_id: agent.agent_id,
    goal: agent.goal,
    bio: agent.bio,
    registered_at: agent.registered_at,
    last_seen: agent.last_seen,
    left_at: agent.left_at,
  };

  appendJsonLine(path.join(mmDir, AGENTS_FILE), record);
  touchDatabaseFile(projectPath);
}

export function updateProjectConfig(projectPath: string, updates: Partial<ProjectConfig>): ProjectConfig {
  const mmDir = resolveMmDir(projectPath);
  ensureDir(mmDir);

  const existing = readProjectConfig(projectPath) ?? { version: 1, known_agents: {} };
  const existingKnown = existing.known_agents ?? {};
  const updateKnown = updates.known_agents ?? {};
  const mergedKnown: ProjectConfig['known_agents'] = { ...existingKnown };

  for (const [guid, agent] of Object.entries(updateKnown)) {
    const prior = mergedKnown[guid] ?? {};
    mergedKnown[guid] = { ...prior, ...agent };
  }

  const merged: ProjectConfig = {
    ...existing,
    ...updates,
    version: updates.version ?? existing.version ?? 1,
    known_agents: mergedKnown,
  };

  fs.writeFileSync(
    path.join(mmDir, PROJECT_CONFIG_FILE),
    JSON.stringify(merged, null, 2) + '\n',
    'utf8'
  );

  return merged;
}

export function readMessages(projectPath: string): MessageJsonlRecord[] {
  const mmDir = resolveMmDir(projectPath);
  const records = readJsonlFile<MessageJsonlRecord | MessageUpdateJsonlRecord>(path.join(mmDir, MESSAGES_FILE));

  // Build a map of messages, applying updates in order
  const messageMap = new Map<string, MessageJsonlRecord>();

  for (const record of records) {
    if (record.type === 'message') {
      messageMap.set(record.id, record);
    } else if (record.type === 'message_update') {
      const existing = messageMap.get(record.id);
      if (existing) {
        // Apply update fields
        if (record.body !== undefined) existing.body = record.body;
        if (record.edited_at !== undefined) existing.edited_at = record.edited_at;
        if (record.archived_at !== undefined) existing.archived_at = record.archived_at;
      }
    }
  }

  return Array.from(messageMap.values());
}

export function readAgents(projectPath: string): AgentJsonlRecord[] {
  const mmDir = resolveMmDir(projectPath);
  return readJsonlFile<AgentJsonlRecord>(path.join(mmDir, AGENTS_FILE));
}

export function readProjectConfig(projectPath: string): ProjectConfig | null {
  const mmDir = resolveMmDir(projectPath);
  const configPath = path.join(mmDir, PROJECT_CONFIG_FILE);
  if (!fs.existsSync(configPath)) {
    return null;
  }

  const raw = fs.readFileSync(configPath, 'utf8');
  return JSON.parse(raw) as ProjectConfig;
}

export function rebuildDatabaseFromJsonl(db: Database.Database, projectPath: string): void {
  const messages = readMessages(projectPath);
  const agents = readAgents(projectPath);
  const projectConfig = readProjectConfig(projectPath);

  const rebuild = db.transaction(() => {
    db.exec('DROP TABLE IF EXISTS mm_messages');
    db.exec('DROP TABLE IF EXISTS mm_agents');
    db.exec('DROP TABLE IF EXISTS mm_read_receipts');
    initSchema(db);

    if (projectConfig?.channel_id) {
      const setConfig = db.prepare('INSERT OR REPLACE INTO mm_config (key, value) VALUES (?, ?)');
      setConfig.run('channel_id', projectConfig.channel_id);
      if (projectConfig.channel_name) {
        setConfig.run('channel_name', projectConfig.channel_name);
      }
    }

    const insertAgent = db.prepare(`
      INSERT OR REPLACE INTO mm_agents (
        guid, agent_id, goal, bio, registered_at, last_seen, left_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?)
    `);

    for (const agent of agents) {
      insertAgent.run(
        agent.id,
        agent.agent_id,
        agent.goal ?? null,
        agent.bio ?? null,
        agent.registered_at,
        agent.last_seen,
        agent.left_at ?? null
      );
    }

    const insertMessage = db.prepare(`
      INSERT OR REPLACE INTO mm_messages (
        guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `);

    for (const message of messages) {
      insertMessage.run(
        message.id,
        message.ts,
        message.channel_id ?? null,
        message.from_agent,
        message.body,
        JSON.stringify(message.mentions),
        message.message_type ?? 'agent',
        message.reply_to ?? null,
        message.edited_at ?? null,
        message.archived_at ?? null
      );
    }
  });

  rebuild();
}
