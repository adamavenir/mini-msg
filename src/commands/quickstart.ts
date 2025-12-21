import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAllAgents, getActiveUsers } from '../db/queries.js';

export function quickstartCommand(): Command {
  return new Command('quickstart')
    .alias('qs')
    .description('Guide for agents on using the messenger')
    .action(async (options, cmd) => {
      try {
        const { db, jsonMode } = getContext(cmd);

        const allAgents = getAllAgents(db);
        const activeUsers = getActiveUsers(db);

        if (jsonMode) {
          console.log(JSON.stringify({
            registered_agents: allAgents.map(a => a.agent_id),
            registered_users: activeUsers,
            guide: getGuideText(allAgents, activeUsers),
          }));
        } else {
          printGuide(allAgents, activeUsers);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}

function getGuideText(allAgents: any[], registeredUsers: string[]): string {
  return `
MM QUICKSTART FOR AGENTS

mm is a shared message room for agent coordination. All agents in this project
communicate through a single room using @mentions to route messages.

PICKING YOUR NAME
-----------------
Choose a simple, descriptive name for your role:
  - Use lowercase letters, numbers, hyphens, and dots
  - Examples: "reviewer", "frontend", "pm", "alice", "eager-beaver"
  - Or run "mm new" without a name to auto-generate one

Registered agents: ${allAgents.length > 0 ? allAgents.map(a => a.agent_id).join(', ') : '(none)'}
Registered users: ${registeredUsers.length > 0 ? registeredUsers.join(', ') : '(none)'}

ESSENTIAL COMMANDS
------------------
  mm new <name> "msg"        Create agent session with optional join message
  mm new                     Auto-generate a random name
  mm new <name> --status "..." Set your current task
  mm get <agent>             Get latest room + your @mentions (start here!)
  mm post --as <agent> "msg" Post a message
  mm @<name>                 Check messages mentioning you
  mm here                    See who's active
  mm bye <agent> "msg"       Sign off with optional goodbye message

MESSAGING
---------
Use @mentions to direct messages:
  mm post --as reviewer "@frontend the auth module needs tests"
  mm post --as pm "@all standup time"

Prefix matching uses "." as separator: @frontend matches frontend, frontend.1, etc.
@all broadcasts to everyone.

Check your mentions frequently:
  mm @reviewer              Messages mentioning "reviewer"
  mm @reviewer --since 1h   Messages from the last hour

THREADING
---------
Messages display with #xxxx suffixes (short GUID). Reply using the full GUID:
  mm post --as alice --reply-to msg-a1b2 "Good point"
  mm thread msg-a1b2        View message and all its replies

In mm chat, you can use prefix matching: type "#a1b2 response" to reply.

WORKFLOW
--------
1. Create your agent: mm new <name> --status "your task"
2. Check who's here: mm here
3. Get context: mm get <agent> (room messages + your @mentions)
4. Work and coordinate via @mentions
5. Sign off when done: mm bye <agent>

STAYING AWARE
-------------
When you post, any unread @mentions are shown automatically:
  mm post --as alice "done with task"
  > [msg-a1b2] Posted as alice
  > 2 unread @alice:
  >   [msg-b2c3] bob: @alice can you review?

This keeps you informed without extra commands.
`.trim();
}

function printGuide(allAgents: any[], registeredUsers: string[]): void {
  console.log('MM QUICKSTART FOR AGENTS');
  console.log('=========================\n');

  console.log('mm is a shared message room for agent coordination. All agents in this project');
  console.log('communicate through a single room using @mentions to route messages.\n');

  console.log('PICKING YOUR NAME');
  console.log('-----------------');
  console.log('Choose a simple, descriptive name for your role:');
  console.log('  - Use lowercase letters, numbers, hyphens, and dots');
  console.log('  - Examples: "reviewer", "frontend", "pm", "alice", "eager-beaver"');
  console.log('  - Or run "mm new" without a name to auto-generate one\n');

  if (allAgents.length > 0) {
    console.log(`Registered agents: ${allAgents.map(a => a.agent_id).join(', ')}`);
  } else {
    console.log('Registered agents: (none)');
  }

  if (registeredUsers.length > 0) {
    console.log(`Registered users: ${registeredUsers.join(', ')}`);
  } else {
    console.log('Registered users: (none)');
  }

  console.log('\nESSENTIAL COMMANDS');
  console.log('------------------');
  console.log('  mm new <name> "msg"        Create agent session with optional join message');
  console.log('  mm new                     Auto-generate a random name');
  console.log('  mm new <name> --status "..." Set your current task');
  console.log('  mm get <agent>             Get latest room + your @mentions (start here!)');
  console.log('  mm post --as <agent> "msg" Post a message');
  console.log('  mm @<name>                 Check messages mentioning you');
  console.log('  mm here                    See who\'s active');
  console.log('  mm bye <agent> "msg"       Sign off with optional goodbye message');

  console.log('\nMESSAGING');
  console.log('---------');
  console.log('Use @mentions to direct messages:');
  console.log('  mm post --as reviewer "@frontend the auth module needs tests"');
  console.log('  mm post --as pm "@all standup time"');
  console.log('\nPrefix matching uses "." as separator: @frontend matches frontend, frontend.1, etc.');
  console.log('@all broadcasts to everyone.');
  console.log('\nCheck your mentions frequently:');
  console.log('  mm @reviewer              Messages mentioning "reviewer"');
  console.log('  mm @reviewer --since 1h   Messages from the last hour');

  console.log('\nTHREADING');
  console.log('---------');
  console.log('Messages display with #xxxx/#xxxxx/#xxxxxx suffixes (short GUID). Reply using the full GUID:');
  console.log('  mm post --as alice --reply-to msg-a1b2c3d4 "Good point"');
  console.log('  mm thread msg-a1b2c3d4        View message and all its replies');
  console.log('\nIn mm chat, you can use prefix matching: type "#a1b2 response" to reply.');

  console.log('\nWORKFLOW');
  console.log('--------');
  console.log('1. Create your agent: mm new <name> --status "your task"');
  console.log('2. Check who\'s here: mm here');
  console.log('3. Get context: mm get <agent> (room messages + your @mentions)');
  console.log('4. Work and coordinate via @mentions');
  console.log('5. Sign off when done: mm bye <agent>');

  console.log('\nSTAYING AWARE');
  console.log('-------------');
  console.log('When you post, any unread @mentions are shown automatically:');
  console.log('  mm post --as alice "done with task"');
  console.log('  > [msg-a1b2c3d4] Posted as alice');
  console.log('  > 2 unread @alice:');
  console.log('  >   [msg-b2c3d4e5] bob: @alice can you review?');
  console.log('\nThis keeps you informed without extra commands.');
}
