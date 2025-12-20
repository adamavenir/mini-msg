import { Command } from 'commander';
import { discoverProject, openDatabase } from '../core/project.js';
import { getMessages, getMessagesWithMention, getActiveAgents, getConfig } from '../db/queries.js';

interface HookOutput {
  additionalContext?: string;
  continue?: boolean;
}

export function hookSessionCommand(): Command {
  return new Command('hook-session')
    .description('SessionStart hook handler (internal)')
    .argument('<event>', 'startup or resume')
    .action((event: string) => {
      const output: HookOutput = {};

      try {
        const agentId = process.env.MM_AGENT_ID;

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

        if (!agentId) {
          // No agent registered - prompt for registration
          const staleHours = parseInt(getConfig(db, 'stale_hours') || '4', 10);
          const activeAgents = getActiveAgents(db, staleHours);

          let context = '[mm] This project uses mm for agent coordination.\n';
          context += 'You are not registered. To join the room:\n\n';
          context += '  mm new <name> --goal "your current task"\n\n';

          if (activeAgents.length > 0) {
            context += 'Active agents: ' + activeAgents.map(a => a.agent_id).join(', ') + '\n';
          }

          context += 'Use /skill mm-chat for conversation guidance.';

          output.additionalContext = context;
        } else {
          // Agent is registered - inject context
          const roomLimit = 10;
          const mentionsLimit = 5;

          const roomMessages = getMessages(db, { limit: roomLimit });
          const agentBase = agentId.includes('.')
            ? agentId.substring(0, agentId.lastIndexOf('.'))
            : agentId;
          const mentionMessages = getMessagesWithMention(db, agentBase, { limit: mentionsLimit + roomLimit });

          // Filter out mentions already in room
          const roomIds = new Set(roomMessages.map(m => m.id));
          const filteredMentions = mentionMessages.filter(m => !roomIds.has(m.id)).slice(0, mentionsLimit);

          let context = `[mm] You are ${agentId}. Session ${event}.\n\n`;

          if (roomMessages.length > 0) {
            context += 'ROOM:\n';
            for (const msg of roomMessages) {
              context += `[${msg.id}] ${msg.from_agent}: ${msg.body}\n`;
            }
          } else {
            context += 'ROOM: (no messages yet)\n';
          }

          if (filteredMentions.length > 0) {
            context += `\n@${agentBase}:\n`;
            for (const msg of filteredMentions) {
              context += `[${msg.id}] ${msg.from_agent}: ${msg.body}\n`;
            }
          }

          context += `\nPost: mm post --as ${agentId} "message"`;

          output.additionalContext = context;
        }

        db.close();
      } catch (error) {
        // On error, stay silent - don't break the session
      }

      console.log(JSON.stringify(output));
    });
}
