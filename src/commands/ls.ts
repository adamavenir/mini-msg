import { Command } from 'commander';
import fs from 'fs';
import path from 'path';
import { readGlobalConfig } from '../core/config.js';
import { handleError } from './shared.js';

export function lsCommand(): Command {
  return new Command('ls')
    .description('List known channels')
    .action((options, cmd) => {
      try {
        const jsonMode = cmd.optsWithGlobals().json || false;
        const config = readGlobalConfig();

        if (!config || Object.keys(config.channels).length === 0) {
          if (jsonMode) {
            console.log(JSON.stringify({ channels: [], current_channel: null }, null, 2));
          } else {
            console.log('No channels registered');
          }
          return;
        }

        const current = config.current_channel ?? null;
        const channels = Object.entries(config.channels).map(([id, channel]) => {
          const mmDir = path.join(channel.path, '.mm');
          return {
            id,
            name: channel.name,
            path: channel.path,
            has_local: fs.existsSync(mmDir),
            is_current: current === id,
          };
        });

        if (jsonMode) {
          console.log(JSON.stringify({ current_channel: current, channels }, null, 2));
        } else {
          console.log(`Channels (${channels.length}):`);
          for (const channel of channels) {
            const marker = channel.is_current ? '*' : ' ';
            const status = channel.has_local ? 'local' : 'missing';
            console.log(`${marker} ${channel.id}  ${channel.name}  ${channel.path} (${status})`);
          }
        }
      } catch (error) {
        handleError(error);
      }
    });
}
