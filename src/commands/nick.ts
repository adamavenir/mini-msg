import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { updateProjectConfig, type ProjectConfig } from '../db/jsonl.js';
import { normalizeAgentRef, isValidAgentId } from '../core/agents.js';

type KnownAgentEntry = NonNullable<ProjectConfig['known_agents']>[string];

function findKnownAgent(config: ProjectConfig | null, ref: string): { guid: string; entry: KnownAgentEntry } | null {
  if (!config?.known_agents) return null;
  const normalized = normalizeAgentRef(ref);

  if (config.known_agents[normalized]) {
    return { guid: normalized, entry: config.known_agents[normalized] };
  }

  for (const [guid, entry] of Object.entries(config.known_agents)) {
    if (entry.name === normalized || entry.global_name === normalized) {
      return { guid, entry };
    }
    if (entry.nicks && entry.nicks.includes(normalized)) {
      return { guid, entry };
    }
  }

  return null;
}

export function nickCommand(): Command {
  return new Command('nick')
    .description('Add a nickname for an agent in this channel')
    .argument('<agent>', 'agent name or GUID')
    .requiredOption('--as <nickname>', 'nickname to add')
    .action((agentRef: string, options, cmd) => {
      try {
        const { project, jsonMode, projectConfig } = getContext(cmd);
        const nickname = normalizeAgentRef(options.as);

        if (!isValidAgentId(nickname)) {
          throw new Error(`Invalid nickname: ${nickname}`);
        }

        const found = findKnownAgent(projectConfig ?? null, agentRef);
        if (!found) {
          throw new Error(`Agent not found in known_agents: ${agentRef}`);
        }

        const existing = found.entry.nicks ?? [];
        const next = existing.includes(nickname) ? existing : [...existing, nickname];
        updateProjectConfig(project.dbPath, {
          known_agents: {
            [found.guid]: {
              nicks: next,
            },
          },
        });

        if (jsonMode) {
          console.log(JSON.stringify({
            agent_id: found.guid,
            name: found.entry.name,
            nicks: next,
          }, null, 2));
        } else {
          console.log(`Added nickname @${nickname} for @${found.entry.name}`);
        }
      } catch (error) {
        handleError(error);
      }
    });
}

export function nicksCommand(): Command {
  return new Command('nicks')
    .description('Show nicknames for an agent in this channel')
    .argument('<agent>', 'agent name or GUID')
    .action((agentRef: string, options, cmd) => {
      try {
        const { jsonMode, projectConfig } = getContext(cmd);
        const found = findKnownAgent(projectConfig ?? null, agentRef);
        if (!found) {
          throw new Error(`Agent not found in known_agents: ${agentRef}`);
        }

        const nicks = found.entry.nicks ?? [];
        if (jsonMode) {
          console.log(JSON.stringify({
            agent_id: found.guid,
            name: found.entry.name,
            global_name: found.entry.global_name ?? null,
            nicks,
          }, null, 2));
        } else {
          console.log(`@${found.entry.name} (${found.guid})`);
          if (nicks.length === 0) {
            console.log('  (no nicknames)');
          } else {
            for (const nick of nicks) {
              console.log(`  @${nick}`);
            }
          }
        }
      } catch (error) {
        handleError(error);
      }
    });
}

export function whoamiCommand(): Command {
  return new Command('whoami')
    .description('Show your known names and nicknames')
    .action((options, cmd) => {
      try {
        const { jsonMode, projectConfig } = getContext(cmd);
        const envAgent = process.env.MM_AGENT_ID;
        if (!envAgent) {
          throw new Error('MM_AGENT_ID not set. Run `mm new` or `mm back` first.');
        }

        const found = findKnownAgent(projectConfig ?? null, envAgent);
        if (!found) {
          throw new Error(`Agent not found in known_agents: ${envAgent}`);
        }

        const nicks = found.entry.nicks ?? [];
        const payload = {
          agent_id: found.guid,
          name: found.entry.name,
          global_name: found.entry.global_name ?? null,
          nicks,
        };

        if (jsonMode) {
          console.log(JSON.stringify(payload, null, 2));
        } else {
          console.log(`You are @${found.entry.name} (${found.guid})`);
          if (payload.global_name) {
            console.log(`  global: @${payload.global_name}`);
          }
          if (nicks.length === 0) {
            console.log('  nicknames: (none)');
          } else {
            console.log('  nicknames:');
            for (const nick of nicks) {
              console.log(`    @${nick}`);
            }
          }
        }
      } catch (error) {
        handleError(error);
      }
    });
}
