import { Command } from 'commander';
import { discoverProject, openDatabase } from '../core/project.js';
import { getMessages, getMessagesWithMention } from '../db/queries.js';

interface HookOutput {
  additionalContext?: string;
}

export function hookPromptCommand(): Command {
  return new Command('hook-prompt')
    .description('UserPromptSubmit hook handler (internal)')
    .action(() => {
      const output: HookOutput = {};

      try {
        const agentId = process.env.MM_AGENT_ID;

        // If no agent ID, stay silent
        if (!agentId) {
          console.log(JSON.stringify(output));
          return;
        }

        // Try to discover beads project
        let project;
        try {
          project = discoverProject();
        } catch {
          // Not in a beads project - stay silent
          console.log(JSON.stringify(output));
          return;
        }

        const db = openDatabase(project);

        const roomLimit = 5;
        const mentionsLimit = 3;

        const roomMessages = getMessages(db, { limit: roomLimit });
        const agentBase = agentId.includes('.')
          ? agentId.substring(0, agentId.lastIndexOf('.'))
          : agentId;
        const mentionMessages = getMessagesWithMention(db, agentBase, { limit: mentionsLimit + roomLimit });

        // Filter out mentions already in room
        const roomIds = new Set(roomMessages.map(m => m.id));
        const filteredMentions = mentionMessages.filter(m => !roomIds.has(m.id)).slice(0, mentionsLimit);

        // Only inject if there's something to show
        if (roomMessages.length === 0 && filteredMentions.length === 0) {
          db.close();
          console.log(JSON.stringify(output));
          return;
        }

        let context = `[mm ${agentId}] `;

        // Compact format for per-prompt injection
        const parts: string[] = [];

        if (roomMessages.length > 0) {
          const lastMsg = roomMessages[roomMessages.length - 1];
          parts.push(`Room[${roomMessages.length}]: latest [${lastMsg.id}] ${lastMsg.from_agent}`);
        }

        if (filteredMentions.length > 0) {
          parts.push(`@mentions[${filteredMentions.length}]`);
          // Show the mentions briefly
          for (const msg of filteredMentions.slice(0, 2)) {
            const truncated = msg.body.length > 60 ? msg.body.slice(0, 60) + '...' : msg.body;
            parts.push(`  [${msg.id}] ${msg.from_agent}: ${truncated}`);
          }
        }

        context += parts.join(' | ');
        context += ` (mm get ${agentId} for full view)`;

        output.additionalContext = context;
        db.close();
      } catch {
        // On error, stay silent
      }

      console.log(JSON.stringify(output));
    });
}
