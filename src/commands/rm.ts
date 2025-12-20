import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { deleteMessage, getMessage, getMessageByPrefix } from '../db/queries.js';
import { appendMessageUpdate } from '../db/jsonl.js';

export function rmCommand(): Command {
  return new Command('rm')
    .description('Delete a message')
    .argument('<msgid>', 'message GUID to delete (with or without # prefix)')
    .action(async (msgidStr: string, options, cmd) => {
      try {
        const { db, jsonMode, project } = getContext(cmd);

        // Strip # prefix if present
        const input = msgidStr.trim().replace(/^#/, '');

        // Try exact match first, then prefix match
        let msg = getMessage(db, input);
        if (!msg) {
          msg = getMessage(db, `msg-${input}`);
        }
        if (!msg) {
          msg = getMessageByPrefix(db, input);
        }

        if (!msg) {
          throw new Error(`Message not found: ${input}`);
        }

        const deletedAt = Math.floor(Date.now() / 1000);

        // Update SQLite
        deleteMessage(db, msg.id);

        // Append update to JSONL (source of truth)
        appendMessageUpdate(project.dbPath, {
          id: msg.id,
          body: '[deleted]',
          archived_at: deletedAt,
        });

        if (jsonMode) {
          console.log(JSON.stringify({ id: msg.id, deleted: true }));
        } else {
          console.log(`Deleted message #${msg.id}`);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
