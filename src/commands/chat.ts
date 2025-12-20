import { Command } from 'commander';
import * as readline from 'readline';
import { getContext, handleError } from './shared.js';
import { getMessages, getConfig, setConfig, createMessage, getMessage } from '../db/queries.js';
import { getProjectName, COLOR_PAIRS } from './format.js';
import { extractMentions } from '../core/mentions.js';
import { parseAgentId } from '../core/agents.js';
import { AnsiChatDisplay } from '../chat/display.js';
import { ReadlineChatInput } from '../chat/input.js';
import { resolveReplyReference } from '../chat/reply.js';
import { initProject, openDatabase } from '../core/project.js';
import { initSchema } from '../db/schema.js';
import { resolveChannelContext } from '../core/context.js';
import type { Message, MessageCursor } from '../types.js';
import type { FormattedMessage } from '../chat/types.js';
import type Database from 'better-sqlite3';
import { getDisplayPrefixLength, getGuidPrefix } from '../core/guid.js';

function promptForUsername(): Promise<string> {
  return new Promise((resolve) => {
    const rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
    });
    rl.question('Enter your username: ', (answer) => {
      rl.close();
      resolve(answer.trim());
    });
  });
}

function promptForInit(cwd: string): Promise<boolean> {
  return new Promise((resolve) => {
    const rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
    });
    console.log(`No .mm found in ${cwd}`);
    rl.question('Run `mm init`? [Y/n] ', (answer) => {
      rl.close();
      const normalized = answer.trim().toLowerCase();
      resolve(normalized === '' || normalized === 'y' || normalized === 'yes');
    });
  });
}

/**
 * Build a color map based on recent message activity.
 * Assigns colors 0-5 to the most recently active agent bases.
 * @param db Database instance
 * @param lookbackLimit Number of recent messages to analyze (default: 50)
 * @returns Map from agent base to color index (0-5)
 */
function buildColorMap(db: Database.Database, lookbackLimit: number = 50, includeArchived: boolean = false): Map<string, number> {
  const messages = getMessages(db, { limit: lookbackLimit, includeArchived });
  const agentBases = new Map<string, number>(); // base -> last seen timestamp

  for (const msg of messages) {
    if (msg.type === 'agent') {
      try {
        const parsed = parseAgentId(msg.from_agent);
        const base = parsed.base;
        const ts = msg.ts;
        if (!agentBases.has(base) || agentBases.get(base)! < ts) {
          agentBases.set(base, ts);
        }
      } catch {
        // Skip invalid agent IDs
      }
    }
  }

  // Sort by most recent activity
  const sorted = Array.from(agentBases.entries())
    .sort((a, b) => b[1] - a[1]);

  // Assign colors sequentially to the most recent agents
  const colorMap = new Map<string, number>();
  sorted.forEach(([base, _], idx) => {
    colorMap.set(base, idx % COLOR_PAIRS.length);
  });

  return colorMap;
}

export function chatCommand(): Command {
  return new Command('chat')
    .description('Interactive chat mode')
    .argument('[channel]', 'channel name or ID to chat in')
    .option('--last <n>', 'show last N messages', '20')
    .option('--show-updates', 'include system event messages')
    .option('--archived', 'include archived messages')
    .action(async (channelArg: string | undefined, options, cmd) => {
      try {
        let context;
        try {
          // If channel argument provided, use it; otherwise use normal resolution
          if (channelArg) {
            const channelContext = resolveChannelContext({ channel: channelArg });
            const db = openDatabase(channelContext.project);
            context = { db, project: channelContext.project, jsonMode: cmd.optsWithGlobals().json || false };
          } else {
            context = getContext(cmd);
          }
        } catch (error) {
          // Check if it's a "not initialized" error
          const message = error instanceof Error ? error.message : String(error);
          if (message.includes("Not initialized") || message.includes("mm init") || message.includes("No channel context")) {
            const cwd = process.cwd();
            const shouldInit = await promptForInit(cwd);
            if (!shouldInit) {
              process.exit(0);
            }
            // Initialize and get context
            const project = initProject();
            const db = openDatabase(project);
            initSchema(db);
            console.log('Initialized .mm/\n');
            context = { db, project, jsonMode: cmd.optsWithGlobals().json || false };
          } else {
            throw error;
          }
        }

        const { db, project, jsonMode } = context;

        if (jsonMode) {
          throw new Error('--json not supported for interactive chat');
        }

        let username = getConfig(db, 'username');
        if (!username) {
          username = await promptForUsername();
          if (!username) {
            throw new Error('Username is required');
          }
          setConfig(db, 'username', username);
        }

        const projectName = getProjectName(project);
        const colorMap = buildColorMap(db, 50, !!options.archived);
        const display = new AnsiChatDisplay(db, colorMap);
        const input = new ReadlineChatInput();

        let lastCursor: MessageCursor | null = null;
        let pollInterval: NodeJS.Timeout | null = null;

        const convertToFormattedMessage = (msg: Message): FormattedMessage => ({
          id: msg.id,
          projectName,
          type: msg.type,
          sender: msg.from_agent,
          body: msg.body,
          reply_to: msg.reply_to,
        });

        const recent = getMessages(db, {
          limit: parseInt(options.last, 10),
          showEvents: !!options.showUpdates,
          includeArchived: !!options.archived
        });
        if (recent.length > 0) {
          for (const msg of recent) {
            display.renderMessage(convertToFormattedMessage(msg));
          }
          const last = recent[recent.length - 1];
          lastCursor = { guid: last.id, ts: last.ts };
        }

        const cleanup = () => {
          if (pollInterval) {
            clearInterval(pollInterval);
            pollInterval = null;
          }
          input.destroy();
          display.destroy();
          db.close();
        };

        pollInterval = setInterval(() => {
          const newMessages = getMessages(db, {
            since: lastCursor ?? undefined,
            showEvents: !!options.showUpdates,
            includeArchived: !!options.archived
          });
          if (newMessages.length > 0) {
            for (const msg of newMessages) {
              display.renderMessage(convertToFormattedMessage(msg));
            }
            const last = newMessages[newMessages.length - 1];
            lastCursor = { guid: last.id, ts: last.ts };
          }
        }, 1000);

        input.onMessage((text) => {
          // Handle /view command
          const parts = text.trim().split(/\s+/);
          if (parts[0] === '/view' && parts[1]) {
            const msgId = parts[1].replace(/^#?/, '');
            const msg = getMessage(db, msgId);
            if (msg) {
              display.renderFullMessage(convertToFormattedMessage(msg));
            } else {
              display.showStatus(`Message ${msgId} not found`);
            }
            return;
          }

          const replyResolution = resolveReplyReference(db, text);
          if (replyResolution.kind === 'ambiguous') {
            const countRow = db.prepare(`
              SELECT COUNT(*) as count
              FROM mm_messages
              WHERE archived_at IS NULL
            `).get() as { count: number } | undefined;
            const prefixLength = getDisplayPrefixLength(countRow?.count ?? 0);
            const suggestions = replyResolution.matches
              .map(match => {
                const prefix = getGuidPrefix(match.guid, prefixLength);
                const preview = match.body.replace(/\s+/g, ' ').trim();
                const clipped = preview.length > 50 ? `${preview.slice(0, 47)}...` : preview;
                return `#${prefix} (@${match.from_agent}) ${clipped}`;
              })
              .join(' | ');
            display.showStatus(`Ambiguous #${replyResolution.prefix}: ${suggestions}`);
            return;
          }

          const body = replyResolution.kind === 'resolved' ? replyResolution.body : text;
          const replyTo = replyResolution.kind === 'resolved' ? replyResolution.reply_to : undefined;
          const mentions = extractMentions(body, db);
          const msg = createMessage(db, {
            from_agent: username,
            body,
            mentions,
            type: 'user',
            reply_to: replyTo,
          });

          display.renderMessage(convertToFormattedMessage(msg));
          lastCursor = { guid: msg.id, ts: msg.ts };
        });

        input.onQuit(() => {
          cleanup();
          process.exit(0);
        });

        input.start();
      } catch (error) {
        handleError(error);
      }
    });
}
