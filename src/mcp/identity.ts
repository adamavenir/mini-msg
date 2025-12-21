import fs from 'fs';
import path from 'path';
import os from 'os';
import type Database from 'better-sqlite3';
import { formatAgentId } from '../core/agents.js';
import { getAgent, getMaxVersion, createAgent, updateAgent } from '../db/queries.js';

const CONFIG_DIR = path.join(os.homedir(), '.config', 'mm');
const CONFIG_FILE = path.join(CONFIG_DIR, 'mcp-agents.json');
const MCP_AGENT_BASE = 'desktop';

interface McpAgentsConfig {
  [projectPath: string]: string; // maps project path to agent ID
}

/**
 * Load persisted agent ID for a project.
 * @returns agent ID or null if not found
 */
export function loadPersistedAgentId(projectPath: string): string | null {
  try {
    if (!fs.existsSync(CONFIG_FILE)) {
      return null;
    }
    const content = fs.readFileSync(CONFIG_FILE, 'utf-8');
    const config: McpAgentsConfig = JSON.parse(content);
    return config[projectPath] ?? null;
  } catch {
    return null;
  }
}

/**
 * Persist agent ID for a project.
 */
export function persistAgentId(projectPath: string, agentId: string): void {
  try {
    // Ensure config directory exists
    if (!fs.existsSync(CONFIG_DIR)) {
      fs.mkdirSync(CONFIG_DIR, { recursive: true });
    }

    // Load existing config or create new
    let config: McpAgentsConfig = {};
    if (fs.existsSync(CONFIG_FILE)) {
      try {
        const content = fs.readFileSync(CONFIG_FILE, 'utf-8');
        config = JSON.parse(content);
      } catch {
        // Corrupted file, start fresh
      }
    }

    // Update mapping
    config[projectPath] = agentId;

    // Write back
    fs.writeFileSync(CONFIG_FILE, JSON.stringify(config, null, 2));
  } catch (error) {
    // Log to stderr, don't fail
    console.error(`[mm-mcp] Failed to persist agent ID: ${error}`);
  }
}

/**
 * Check if a persisted agent is still valid in the database.
 * @returns true if agent exists and hasn't left
 */
export function isAgentValid(db: Database.Database, agentId: string): boolean {
  const agent = getAgent(db, agentId);
  if (!agent) {
    return false;
  }
  // Agent is valid if they haven't left
  return agent.left_at === null;
}

/**
 * Get or create an MCP agent.
 * Uses desktop.N as the agent base.
 * @returns agent ID
 */
export function getOrCreateMcpAgent(db: Database.Database): string {
  // Find next available version
  const version = getMaxVersion(db, MCP_AGENT_BASE) + 1;
  const agentId = formatAgentId(MCP_AGENT_BASE, version);

  const now = Math.floor(Date.now() / 1000);

  // Create the agent
  createAgent(db, {
    agent_id: agentId,
    status: 'Claude Desktop MCP session',
    purpose: null,
    registered_at: now,
    last_seen: now,
  });

  return agentId;
}

/**
 * Reactivate a previously left agent.
 */
export function reactivateAgent(db: Database.Database, agentId: string): void {
  const now = Math.floor(Date.now() / 1000);
  updateAgent(db, agentId, {
    left_at: undefined, // Clear left_at by not setting it - need to use null
    last_seen: now,
  });
  // Explicitly clear left_at
  const stmt = db.prepare('UPDATE mm_agents SET left_at = NULL WHERE agent_id = ?');
  stmt.run(agentId);
}

/**
 * Initialize MCP agent for a project.
 * Main entry point for MCP server startup.
 *
 * Flow:
 * 1. Try to load persisted agent ID
 * 2. If found and valid, reactivate and use it
 * 3. Otherwise, create new agent and persist
 *
 * @returns agent ID for this session
 */
export function initializeMcpAgent(db: Database.Database, projectPath: string): string {
  // Try to load persisted agent
  const persistedId = loadPersistedAgentId(projectPath);

  if (persistedId) {
    const agent = getAgent(db, persistedId);
    if (agent) {
      // Agent exists - reactivate if left, update last_seen
      if (agent.left_at !== null) {
        reactivateAgent(db, persistedId);
        console.error(`[mm-mcp] Reactivated agent: ${persistedId}`);
      } else {
        // Just update last_seen
        updateAgent(db, persistedId, { last_seen: Math.floor(Date.now() / 1000) });
      }
      return persistedId;
    }
    // Persisted agent no longer exists in DB, create new one
  }

  // Create new agent
  const agentId = getOrCreateMcpAgent(db);
  persistAgentId(projectPath, agentId);
  console.error(`[mm-mcp] Created agent: ${agentId}`);

  return agentId;
}
