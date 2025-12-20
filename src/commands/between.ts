import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, getAllAgents, getAgentBases, getMessages } from '../db/queries.js';
import { matchesPrefix } from '../core/agents.js';
import { resolveAgentRef } from '../core/context.js';
import { formatMessage, getProjectName } from './format.js';
import type Database from 'better-sqlite3';
import type { ProjectConfig } from '../db/jsonl.js';
import { parseTimeExpression } from '../core/time-query.js';

function resolveAgent(db: Database.Database, ref: string, config: ProjectConfig | null) {
  const normalized = resolveAgentRef(ref, config ?? null);
  let agent = getAgent(db, normalized);
  if (agent) return agent;

  const matches = getAllAgents(db).filter(a => matchesPrefix(a.agent_id, normalized));
  if (matches.length === 0) {
    throw new Error(`Agent not found: ${ref}`);
  }
  if (matches.length > 1) {
    throw new Error(`Ambiguous prefix '${ref}' matches: ${matches.map(a => a.agent_id).join(', ')}`);
  }
  agent = matches[0];
  return agent;
}

export function betweenCommand(): Command {
  return new Command('between')
    .description('Show messages between two agents')
    .argument('<agentA>', 'first agent name or prefix')
    .argument('<agentB>', 'second agent name or prefix')
    .option('--since <time|guid>', 'show messages after time or GUID')
    .option('--before <time|guid>', 'show messages before time or GUID')
    .option('--from <time|guid>', 'range start (time or GUID)')
    .option('--to <time|guid>', 'range end (time or GUID)')
    .action((agentARef: string, agentBRef: string, options, cmd) => {
      try {
        const { db, project, jsonMode, projectConfig } = getContext(cmd);
        const agentA = resolveAgent(db, agentARef, projectConfig);
        const agentB = resolveAgent(db, agentBRef, projectConfig);

        if (options.since && options.from) {
          throw new Error('Use --since or --from, not both');
        }
        if (options.before && options.to) {
          throw new Error('Use --before or --to, not both');
        }
        const sinceValue = options.from ?? options.since;
        const beforeValue = options.to ?? options.before;
        const since = sinceValue ? parseTimeExpression(db, sinceValue, 'since') : undefined;
        const before = beforeValue ? parseTimeExpression(db, beforeValue, 'before') : undefined;

        const rows = getMessages(db, { since, before })
          .filter(msg => msg.from_agent === agentA.agent_id || msg.from_agent === agentB.agent_id);

        if (jsonMode) {
          const now = Math.floor(Date.now() / 1000);
          const guidByAgent = new Map([
            [agentA.agent_id, agentA.guid],
            [agentB.agent_id, agentB.guid],
          ]);
          const messages = rows.map(row => ({
            id: row.id,
            agent_id: guidByAgent.get(row.from_agent) ?? null,
            body: row.body,
            created_at: new Date(row.ts * 1000).toISOString(),
            age_seconds: Math.max(0, now - row.ts),
            mentions: row.mentions,
            reply_to: row.reply_to ?? null,
          }));
          const payload = {
            agents: [agentA.agent_id, agentB.agent_id],
            messages,
            total: messages.length,
          };
          console.log(JSON.stringify(payload, null, 2));
        } else {
          if (rows.length === 0) {
            console.log(`No messages between @${agentA.agent_id} and @${agentB.agent_id}`);
          } else {
            const projectName = getProjectName(project);
            const agentBases = getAgentBases(db);
            for (const row of rows) {
              console.log(formatMessage({
                id: row.id,
                ts: row.ts,
                from_agent: row.from_agent,
                body: row.body,
                mentions: row.mentions,
                type: row.type ?? 'agent',
                reply_to: row.reply_to,
              }, projectName, agentBases));
            }
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
