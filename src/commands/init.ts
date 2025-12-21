import { Command } from 'commander';
import readline from 'readline';
import path from 'path';
import fs from 'fs';
import { initProject, openDatabase, discoverProject } from '../core/project.js';
import { registerChannel } from '../core/config.js';
import { generateGuid } from '../core/guid.js';
import { initSchema } from '../db/schema.js';
import { readProjectConfig, updateProjectConfig } from '../db/jsonl.js';
import { setConfig, getConfig } from '../db/queries.js';

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

interface InitResult {
  initialized: boolean;
  already_existed: boolean;
  channel_id: string;
  channel_name: string;
  path: string;
}

export function initCommand(): Command {
  return new Command('init')
    .description('Initialize mm in current directory')
    .option('--force', 'reinitialize even if already exists')
    .option('--defaults', 'use default values without prompting (idempotent)')
    .action(async (options, cmd) => {
      const jsonMode = cmd.optsWithGlobals().json || false;
      const useDefaults = options.defaults || false;

      try {
        // Check if already initialized when using --defaults
        if (useDefaults && !options.force) {
          try {
            const existing = discoverProject();
            const mmConfigPath = path.join(existing.root, '.mm', 'mm-config.json');
            if (fs.existsSync(mmConfigPath)) {
              const existingConfig = readProjectConfig(path.join(existing.root, '.mm', 'mm.db'));
              if (existingConfig?.channel_id && existingConfig?.channel_name) {
                const result: InitResult = {
                  initialized: true,
                  already_existed: true,
                  channel_id: existingConfig.channel_id,
                  channel_name: existingConfig.channel_name,
                  path: existing.root,
                };

                if (jsonMode) {
                  console.log(JSON.stringify(result, null, 2));
                } else {
                  console.log(`Already initialized: ${existingConfig.channel_name} (${existingConfig.channel_id})`);
                }
                return;
              }
            }
          } catch {
            // Not initialized, continue with init
          }
        }

        const project = initProject(undefined, options.force);
        const existingConfig = readProjectConfig(project.dbPath);
        let channelId = existingConfig?.channel_id;
        let channelName = existingConfig?.channel_name;
        const alreadyExisted = !!(channelId && channelName);

        if (!channelId) {
          const defaultName = path.basename(project.root);
          channelName = useDefaults ? defaultName : await promptForChannelName(defaultName);
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

          const result: InitResult = {
            initialized: true,
            already_existed: alreadyExisted,
            channel_id: channelId,
            channel_name: channelName,
            path: project.root,
          };

          if (jsonMode) {
            console.log(JSON.stringify(result, null, 2));
          } else {
            if (!alreadyExisted) {
              console.log(`✓ Registered channel ${channelId} as '${channelName}'`);
            }
            console.log('Initialized .mm/');
            console.log('');
            console.log('Next steps:');
            console.log('  mm new <name>                # Join as an agent');
            console.log('  mm hook-install              # Install Claude Code hooks');
            console.log('  mm hook-install --precommit  # Add git pre-commit hook for claims');
          }
        }
      } catch (error) {
        if (jsonMode) {
          console.log(JSON.stringify({ initialized: false, error: error instanceof Error ? error.message : 'Failed to initialize' }, null, 2));
          process.exit(1);
        }
        if (error instanceof Error) {
          console.error(error.message);
        } else {
          console.error('Failed to initialize');
        }
        process.exit(1);
      }
    });
}
