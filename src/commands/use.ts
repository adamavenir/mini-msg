import { Command } from 'commander';
import { findChannelByRef, readGlobalConfig, setCurrentChannel } from '../core/config.js';
import { handleError } from './shared.js';

export function useCommand(): Command {
  return new Command('use')
    .description('Set current channel context')
    .argument('<channel>', 'channel name or ID')
    .action((channelRef: string, options, cmd) => {
      try {
        const jsonMode = cmd.optsWithGlobals().json || false;
        const config = readGlobalConfig();
        if (!config) {
          throw new Error('No channels registered. Run `mm init` in a project first.');
        }

        const resolved = findChannelByRef(channelRef, config);
        if (!resolved) {
          throw new Error(`Channel not found: ${channelRef}`);
        }

        setCurrentChannel(resolved.id);

        if (jsonMode) {
          console.log(JSON.stringify({
            channel_id: resolved.id,
            channel_name: resolved.name,
            path: resolved.path,
          }, null, 2));
        } else {
          console.log(`Current channel set to ${resolved.name} (${resolved.id})`);
        }
      } catch (error) {
        handleError(error);
      }
    });
}
