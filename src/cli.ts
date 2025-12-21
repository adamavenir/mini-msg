import { Command } from 'commander';
import { initCommand } from './commands/init.js';
import { newCommand } from './commands/new.js';
import { backCommand } from './commands/back.js';
import { byeCommand } from './commands/bye.js';
import { hereCommand } from './commands/here.js';
import { whoCommand } from './commands/who.js';
import { postCommand } from './commands/post.js';
import { editCommand } from './commands/edit.js';
import { rmCommand } from './commands/rm.js';
import { linkCommand } from './commands/link.js';
import { unlinkCommand } from './commands/unlink.js';
import { projectsCommand } from './commands/projects.js';
import { configCommand } from './commands/config.js';
import { quickstartCommand } from './commands/quickstart.js';
import { getCommand } from './commands/get.js';
import { watchCommand } from './commands/watch.js';
import { messagesAction } from './commands/messages.js';
import { mentionsAction } from './commands/mentions.js';
import { historyCommand } from './commands/history.js';
import { betweenCommand } from './commands/between.js';
import { hookInstallCommand } from './commands/hook-install.js';
import { hookSessionCommand } from './commands/hook-session.js';
import { hookPromptCommand } from './commands/hook-prompt.js';
import { hookPrecommitCommand } from './commands/hook-precommit.js';
import { chatCommand } from './commands/chat.js';
import { filterCommand } from './commands/filter.js';
import { viewCommand } from './commands/view.js';
import { pruneCommand } from './commands/prune.js';
import { threadCommand } from './commands/thread.js';
import { migrateCommand } from './commands/migrate.js';
import { renameCommand } from './commands/rename.js';
import { lsCommand } from './commands/ls.js';
import { claimCommand } from './commands/claim.js';
import { clearCommand } from './commands/clear.js';
import { claimsCommand } from './commands/claims.js';
import { statusCommand } from './commands/status.js';
import { nickCommand, nicksCommand, whoamiCommand } from './commands/nick.js';
import { rosterCommand } from './commands/roster.js';
import { infoCommand } from './commands/info.js';

const VERSION = '0.2.0';

export function main() {
  const program = new Command();

  program
    .name('mm')
    .description('Mini messenger - agent-to-agent messaging')
    .version(VERSION)
    .option('--project <alias>', 'operate in linked project')
    .option('--in <channel>', 'operate in channel context')
    .option('--json', 'output in JSON format');

  // Add subcommands
  program.addCommand(initCommand());
  program.addCommand(newCommand());
  program.addCommand(backCommand());
  program.addCommand(byeCommand());
  program.addCommand(hereCommand());
  program.addCommand(whoCommand());
  program.addCommand(postCommand());
  program.addCommand(editCommand());
  program.addCommand(rmCommand());
  program.addCommand(linkCommand());
  program.addCommand(unlinkCommand());
  program.addCommand(projectsCommand());
  program.addCommand(configCommand());
  program.addCommand(quickstartCommand());
  program.addCommand(getCommand());
  program.addCommand(watchCommand());
  program.addCommand(historyCommand());
  program.addCommand(betweenCommand());
  program.addCommand(hookInstallCommand());
  program.addCommand(hookSessionCommand());
  program.addCommand(hookPromptCommand());
  program.addCommand(hookPrecommitCommand());
  program.addCommand(chatCommand());
  program.addCommand(filterCommand());
  program.addCommand(viewCommand());
  program.addCommand(pruneCommand());
  program.addCommand(threadCommand());
  program.addCommand(migrateCommand());
  program.addCommand(renameCommand());
  program.addCommand(lsCommand());
  program.addCommand(claimCommand());
  program.addCommand(clearCommand());
  program.addCommand(claimsCommand());
  program.addCommand(statusCommand());
  program.addCommand(nickCommand());
  program.addCommand(nicksCommand());
  program.addCommand(whoamiCommand());
  program.addCommand(rosterCommand());
  program.addCommand(infoCommand());

  // Check for @mention shorthand (e.g., mm @alice)
  const args = process.argv.slice(2);
  if (args.length > 0 && args[0].startsWith('@') && !args[0].startsWith('--')) {
    // Parse global options before bypassing Commander
    const projectIndex = args.indexOf('--project');
    const projectValue = projectIndex !== -1 && projectIndex + 1 < args.length
      ? args[projectIndex + 1]
      : undefined;
    const jsonMode = args.includes('--json');

    // Set global options on program before calling mentionsAction
    if (projectValue) {
      program.setOptionValue('project', projectValue);
    }
    if (jsonMode) {
      program.setOptionValue('json', true);
    }

    // Rewrite to mentions command
    mentionsAction(args[0], {
      last: args.includes('--last') ? args[args.indexOf('--last') + 1] : undefined,
      since: args.includes('--since') ? args[args.indexOf('--since') + 1] : undefined,
      all: args.includes('--all'),
    }, program);
    return;
  }

  // Add explicit help command so "mm help" works like "mm --help"
  program
    .command('help')
    .description('Show help')
    .action(() => {
      program.outputHelp();
    });

  // Check for unknown commands before parsing
  const knownCommands = new Set([
    'init', 'new', 'back', 'bye', 'here', 'who', 'post', 'edit', 'rm',
    'link', 'unlink', 'projects', 'config',
    'quickstart', 'qs', 'get', 'watch', 'history', 'between', 'help',
    'hook-install', 'hook-session', 'hook-prompt', 'hook-precommit', 'chat', 'filter', 'view', 'prune', 'thread', 'migrate', 'rename',
    'ls', 'claim', 'clear', 'claims', 'status', 'nick', 'nicks', 'whoami', 'roster', 'info'
  ]);

  const firstArg = args[0];
  if (firstArg && !firstArg.startsWith('-') && !knownCommands.has(firstArg)) {
    console.error(`Unknown command: ${firstArg}\n`);
    program.outputHelp();
    process.exit(1);
  }

  // Default action: show recent messages (when no subcommand)
  // Equivalent to: mm get --last 20
  program.action((options, cmd) => {
    // Pass default options for recent messages
    messagesAction({ last: '20' }, cmd);
  });

  program.parse();
}
