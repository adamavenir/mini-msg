import { Command } from 'commander';
import readline from 'readline';
import path from 'path';
import { getContext, handleError } from './shared.js';
import { createAgent, createMessage, getAgent, getConfig, isAgentActive, updateAgent } from '../db/queries.js';
import { appendAgent, readProjectConfig, updateProjectConfig } from '../db/jsonl.js';
import { isValidAgentId, normalizeAgentRef } from '../core/agents.js';
import { generateGuid } from '../core/guid.js';
import { extractMentions } from '../core/mentions.js';
import { writeClaudeEnv } from './hooks.js';
import { uniqueNamesGenerator, adjectives, animals } from 'unique-names-generator';
import type { ProjectConfig } from '../db/jsonl.js';

/**
 * Generate a random agent name like "eager-beaver" or "cosmic-dolphin".
 */
function generateRandomName(): string {
  return uniqueNamesGenerator({
    dictionaries: [adjectives, animals],
    separator: '-',
    length: 2,
    style: 'lowerCase',
  });
}

/**
 * Generate a unique random name that doesn't conflict with existing agents.
 */
function generateUniqueName(db: ReturnType<typeof import('better-sqlite3')>, maxAttempts = 10): string {
  for (let i = 0; i < maxAttempts; i++) {
    const name = generateRandomName();
    const existing = getAgent(db, name);
    if (!existing) {
      return name;
    }
  }
  // Fallback: add timestamp suffix
  return `${generateRandomName()}-${Date.now() % 10000}`;
}

function findKnownAgentByName(
  config: ProjectConfig | null,
  name: string
): { guid: string; entry: NonNullable<ProjectConfig['known_agents']>[string] } | null {
  const known = config?.known_agents;
  if (!known) return null;

  for (const [guid, entry] of Object.entries(known)) {
    if (entry.name === name) {
      return { guid, entry };
    }
  }

  return null;
}

function promptReuseKnownAgent(name: string, guid: string): Promise<boolean> {
  if (!process.stdin.isTTY) {
    return Promise.resolve(true);
  }

  return new Promise((resolve) => {
    const rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
    });
    rl.question(`Use existing @${name} (${guid})? [Y/n] `, (answer) => {
      rl.close();
      const normalized = answer.trim().toLowerCase();
      resolve(normalized === '' || normalized === 'y' || normalized === 'yes');
    });
  });
}

export function newCommand(): Command {
  return new Command('new')
    .description('Create new agent session')
    .argument('[name]', 'agent name (generates random if omitted)')
    .argument('[message]', 'optional join message to post')
    .option('--goal <goal>', 'agent goal/focus')
    .option('--bio <bio>', 'agent bio/identity')
    .action(async (name: string | undefined, message: string | undefined, options, cmd) => {
      try {
        const { db, project, jsonMode } = getContext(cmd);

        // Get stale hours config
        const staleHours = parseInt(getConfig(db, 'stale_hours') || '4', 10);
        const projectConfig = readProjectConfig(project.dbPath);
        const channelName = projectConfig?.channel_name ?? path.basename(project.root);
        const channelId = projectConfig?.channel_id ?? null;

        let agentId: string;
        let isRejoin = false;

        if (!name) {
          // No name provided - generate a random one
          agentId = generateUniqueName(db);
        } else {
          // Name provided - normalize it (strip @ prefix)
          agentId = normalizeAgentRef(name);

          // Validate the name
          if (!isValidAgentId(agentId)) {
            throw new Error(
              `Invalid agent name: ${agentId}\n` +
              `Names must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens.\n` +
              `Examples: alice, pm, eager-beaver, frontend-dev`
            );
          }

        }

        const existingAgent = getAgent(db, agentId);
        if (existingAgent) {
          // Check if they're active
          if (isAgentActive(db, agentId, staleHours)) {
            throw new Error(
              `Agent @${agentId} is currently active.\n\n` +
              `Options:\n` +
              `  - Use a different name: mm new @other-name\n` +
              `  - Generate a random name: mm new\n` +
              `  - If this is you rejoining: mm back @${agentId}`
            );
          }

          // Agent exists but is stale or left - allow reclaiming
          isRejoin = true;
          const now = Math.floor(Date.now() / 1000);
          updateAgent(db, agentId, {
            last_seen: now,
            left_at: null,
            goal: options.goal || existingAgent.goal,
            bio: options.bio || existingAgent.bio,
          });
        }

        const now = Math.floor(Date.now() / 1000);

        // Create new agent if not rejoining
        if (!isRejoin) {
          const knownMatch = findKnownAgentByName(projectConfig, agentId);
          let agentGuid: string | undefined;

          if (knownMatch) {
            const reuse = await promptReuseKnownAgent(agentId, knownMatch.guid);
            if (reuse) {
              agentGuid = knownMatch.guid;
            }
          }

          if (!agentGuid) {
            const knownGuids = new Set(Object.keys(projectConfig?.known_agents ?? {}));
            do {
              agentGuid = generateGuid('usr');
            } while (knownGuids.has(agentGuid));
          }

          createAgent(db, {
            guid: agentGuid,
            agent_id: agentId,
            goal: options.goal || null,
            bio: options.bio || null,
            registered_at: now,
            last_seen: now,
          });
        }

        const agentRecord = getAgent(db, agentId);
        if (!agentRecord) {
          throw new Error(`Failed to load agent after creation: ${agentId}`);
        }

        appendAgent(project.dbPath, agentRecord);

        const existingKnown = projectConfig?.known_agents?.[agentRecord.guid];
        const createdAt = existingKnown?.created_at
          ?? existingKnown?.first_seen
          ?? new Date(agentRecord.registered_at * 1000).toISOString();
        const globalName = channelName ? `${channelName}-${agentRecord.agent_id}` : agentRecord.agent_id;
        const status = agentRecord.left_at ? 'inactive' : 'active';

        updateProjectConfig(project.dbPath, {
          known_agents: {
            [agentRecord.guid]: {
              name: agentRecord.agent_id,
              global_name: globalName,
              home_channel: channelId,
              created_at: createdAt,
              status,
            },
          },
        });

        // Post join/rejoin message
        let postedMessage = null;
        const joinMessage = message || (isRejoin ? 'rejoined' : 'joined');
        const mentions = extractMentions(joinMessage, db);
        postedMessage = createMessage(db, {
          ts: now,
          from_agent: agentId,
          body: joinMessage,
          mentions,
        });

        // Write to CLAUDE_ENV_FILE if in hooks context
        const wroteEnv = writeClaudeEnv(agentId);

        if (jsonMode) {
          console.log(JSON.stringify({
            agent_id: agentId,
            rejoin: isRejoin,
            message_id: postedMessage?.id || null,
            claude_env: wroteEnv,
          }));
        } else {
          if (isRejoin) {
            console.log(`Rejoined as @${agentId}`);
          } else {
            console.log(`Joined as @${agentId}`);
          }
          if (options.goal) console.log(`  Goal: ${options.goal}`);
          if (options.bio) console.log(`  Bio: ${options.bio}`);
          console.log(`  Posted: [${postedMessage.id}] ${joinMessage}`);
          if (wroteEnv) {
            console.log(`  Registered with Claude hooks`);
          } else {
            console.log(`  Post with: mm post --as ${agentId} "message"`);
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
