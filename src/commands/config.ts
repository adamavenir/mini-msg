import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getConfig, setConfig, getAllConfig } from '../db/queries.js';

export function configCommand(): Command {
  return new Command('config')
    .description('Get or set configuration')
    .argument('[key]', 'config key (stale-hours, etc.)')
    .argument('[value]', 'value to set')
    .action(async (key: string | undefined, value: string | undefined, options, cmd) => {
      try {
        const { db, jsonMode } = getContext(cmd);

        if (!key) {
          // Show all config
          const config = getAllConfig(db);

          if (jsonMode) {
            console.log(JSON.stringify(config));
          } else {
            if (config.length === 0) {
              console.log('No configuration set');
            } else {
              console.log('Configuration:');
              for (const { key, value } of config) {
                console.log(`  ${key}: ${value}`);
              }
            }
          }
        } else if (!value) {
          // Get single config value
          // Normalize key: stale-hours -> stale_hours
          const normalizedKey = key.replace(/-/g, '_');
          const configValue = getConfig(db, normalizedKey);

          if (configValue == null) {
            if (jsonMode) {
              console.log(JSON.stringify({ error: `Config key '${key}' not found` }));
            } else {
              console.log(`Config key '${key}' not found`);
            }
            db.close();
            process.exit(1);
          }

          if (jsonMode) {
            console.log(JSON.stringify({ [key]: configValue }));
          } else {
            console.log(`${key}: ${configValue}`);
          }
        } else {
          // Set config value
          // Normalize key: stale-hours -> stale_hours
          const normalizedKey = key.replace(/-/g, '_');
          setConfig(db, normalizedKey, value);

          if (jsonMode) {
            console.log(JSON.stringify({ [key]: value }));
          } else {
            console.log(`Set ${key} = ${value}`);
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
