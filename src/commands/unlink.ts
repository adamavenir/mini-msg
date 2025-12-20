import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { unlinkProject, getLinkedProject } from '../db/queries.js';

export function unlinkCommand(): Command {
  return new Command('unlink')
    .description('Unlink a project')
    .argument('<alias>', 'alias of the linked project')
    .action(async (alias: string, options, cmd) => {
      try {
        const { db, jsonMode } = getContext(cmd);

        // Check if alias exists
        const existing = getLinkedProject(db, alias);
        if (!existing) {
          throw new Error(`Project alias '${alias}' not found`);
        }

        unlinkProject(db, alias);

        if (jsonMode) {
          console.log(JSON.stringify({ alias, status: 'unlinked' }));
        } else {
          console.log(`Unlinked project '${alias}'`);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
