import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgent, createMessage, updateAgent, getMessagesWithMention, markMessagesRead, getMessage } from '../db/queries.js';
import { appendMessage } from '../db/jsonl.js';
import { extractMentions } from '../core/mentions.js';
import { parseAgentId } from '../core/agents.js';
import { resolveAgentRef } from '../core/context.js';

export function postCommand(): Command {
  return new Command('post')
    .description('Post message to room')
    .requiredOption('--as <agent>', 'agent ID to post as')
    .option('-r, --reply-to <id>', 'reply to message GUID (threading)')
    .option('-s, --silent', 'suppress output including unread mentions')
    .argument('<message>', 'message text')
    .action(async (message: string, options, cmd) => {
      try {
        const { db, project, jsonMode, projectConfig } = getContext(cmd);
        const agentId = resolveAgentRef(options.as, projectConfig ?? null);

        // Verify agent exists and is active
        const agent = getAgent(db, agentId);
        if (!agent) {
          throw new Error(`Agent not found: @${agentId}. Use 'mm new' first.`);
        }

        if (agent.left_at !== null) {
          throw new Error(`Agent @${agentId} has left. Use 'mm back @${agentId}' to resume.`);
        }

        // Validate reply_to if provided
        let replyTo: string | undefined;
        if (options.replyTo) {
          replyTo = options.replyTo.trim().replace(/^#/, '');
          const parentMsg = getMessage(db, replyTo);
          if (!parentMsg) {
            throw new Error(`Message ${replyTo} not found`);
          }
        }

        // Extract mentions
        const mentions = extractMentions(message, db);

        // Create message
        const now = Math.floor(Date.now() / 1000);
        const createdMessage = createMessage(db, {
          ts: now,
          from_agent: agentId,
          body: message,
          mentions,
          reply_to: replyTo,
        });
        appendMessage(project.dbPath, createdMessage);

        // Update agent last_seen (posting counts as activity)
        updateAgent(db, agentId, { last_seen: now });

        if (!options.silent) {
          // Check for unread mentions (excluding own messages)
          // For simple names, base equals the full ID
          const parsed = parseAgentId(agentId);
          const agentBase = parsed.base;
          const allUnread = getMessagesWithMention(db, agentBase, {
            unreadOnly: true,
            agentPrefix: agentBase,
          });
          // Filter out messages from the same agent
          const unreadMentions = allUnread.filter(m => {
            try {
              const senderParsed = parseAgentId(m.from_agent);
              return senderParsed.base !== agentBase;
            } catch {
              return true; // Keep messages from non-agent senders (users)
            }
          });

          if (jsonMode) {
            console.log(JSON.stringify({
              id: createdMessage.id,
              from: agentId,
              mentions,
              reply_to: replyTo ?? null,
              unread: unreadMentions.length,
            }));
          } else {
            const replyInfo = replyTo ? ` (reply to #${replyTo})` : '';
            console.log(`[${createdMessage.id}] Posted as @${agentId}${replyInfo}`);
            if (unreadMentions.length > 0) {
              console.log(`\n${unreadMentions.length} unread @${agentBase}:`);
              for (const msg of unreadMentions.slice(-5)) {
                const preview = msg.body.length > 60 ? msg.body.slice(0, 60) + '...' : msg.body;
                console.log(`  [${msg.id}] ${msg.from_agent}: ${preview}`);
              }
              if (unreadMentions.length > 5) {
                console.log(`  ... and ${unreadMentions.length - 5} more`);
              }
            }
          }

          // Mark shown mentions as read
          if (unreadMentions.length > 0) {
            const messageIds = unreadMentions.map(m => m.id);
            markMessagesRead(db, messageIds, agentBase);
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
