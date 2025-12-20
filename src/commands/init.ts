import { Command } from 'commander';
import readline from 'readline';
import path from 'path';
import { initProject, openDatabase } from '../core/project.js';
import { registerChannel } from '../core/config.js';
import { generateGuid } from '../core/guid.js';
import { initSchema } from '../db/schema.js';
import { readProjectConfig, updateProjectConfig } from '../db/jsonl.js';
import { setConfig } from '../db/queries.js';

function promptForChannelName(defaultName: string): Promise<string> {
  if (!process.stdin.isTTY) {
    return Promise.resolve(defaultName);
  }

  return new Promise((resolve) => {
    const rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
    });
    rl.question(`Channel name for this project? [${defaultName}]: `, (answer) => {
      rl.close();
      const trimmed = answer.trim();
      resolve(trimmed === '' ? defaultName : trimmed);
    });
  });
}

export function initCommand(): Command {
  return new Command('init')
    .description('Initialize mm in current directory')
    .option('--force', 'reinitialize even if already exists')
    .action(async (options) => {
      try {
        const project = initProject(undefined, options.force);
        const existingConfig = readProjectConfig(project.dbPath);
        let channelId = existingConfig?.channel_id;
        let channelName = existingConfig?.channel_name;

        if (!channelId) {
          const defaultName = path.basename(project.root);
          channelName = await promptForChannelName(defaultName);
          channelId = generateGuid('ch');
          updateProjectConfig(project.dbPath, {
            version: existingConfig?.version ?? 1,
            channel_id: channelId,
            channel_name: channelName,
            created_at: new Date().toISOString(),
            known_agents: existingConfig?.known_agents ?? {},
          });
        } else if (!channelName) {
          channelName = path.basename(project.root);
          updateProjectConfig(project.dbPath, { channel_name: channelName });
        }

        const db = openDatabase(project);
        initSchema(db);
        if (channelId) {
          setConfig(db, 'channel_id', channelId);
          if (channelName) {
            setConfig(db, 'channel_name', channelName);
          }
        }
        db.close();

        if (channelId && channelName) {
          registerChannel(channelId, channelName, project.root);
          console.log(`✓ Registered channel ${channelId} as '${channelName}'`);
        }

        console.log('Initialized .mm/');
        console.log('');
        console.log('Next steps:');
        console.log('  mm new <name>                # Join as an agent');
        console.log('  mm hook-install              # Install Claude Code hooks');
        console.log('  mm hook-install --precommit  # Add git pre-commit hook for claims');
      } catch (error) {
        if (error instanceof Error) {
          console.error(error.message);
        } else {
          console.error('Failed to initialize');
        }
        process.exit(1);
      }
    });
}
