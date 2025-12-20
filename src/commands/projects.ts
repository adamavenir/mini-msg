import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getLinkedProjects } from '../db/queries.js';

export function projectsCommand(): Command {
  return new Command('projects')
    .description('List linked projects')
    .action(async (options, cmd) => {
      try {
        const { db, project, jsonMode } = getContext(cmd);

        const linkedProjects = getLinkedProjects(db);

        if (jsonMode) {
          console.log(JSON.stringify({ current: project.root, linked: linkedProjects }));
        } else {
          console.log(`Current project: ${project.root}`);
          if (linkedProjects.length === 0) {
            console.log('\nNo linked projects');
          } else {
            console.log(`\nLinked projects (${linkedProjects.length}):`);
            for (const proj of linkedProjects) {
              console.log(`  ${proj.alias.padEnd(15)} -> ${proj.path}`);
            }
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
