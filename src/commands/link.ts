import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { linkProject, getLinkedProject } from '../db/queries.js';
import { discoverProject } from '../core/project.js';
import path from 'path';

export function linkCommand(): Command {
  return new Command('link')
    .description('Link another beads project')
    .argument('<alias>', 'alias for the linked project')
    .argument('<path>', 'path to the project (containing .beads/)')
    .action(async (alias: string, projectPath: string, options, cmd) => {
      try {
        const { db, jsonMode } = getContext(cmd);

        // Resolve and validate the target project path
        const resolvedPath = path.resolve(projectPath);

        // Try to discover the project to validate it exists
        let targetProject;
        try {
          targetProject = discoverProject(resolvedPath);
        } catch (error) {
          throw new Error(`Not a valid beads project: ${resolvedPath}`);
        }

        // Check if alias already exists
        const existing = getLinkedProject(db, alias);
        if (existing) {
          throw new Error(`Project alias '${alias}' already exists (points to ${existing.path})`);
        }

        // Link the project (store path to the .db file)
        linkProject(db, alias, targetProject.dbPath);

        if (jsonMode) {
          console.log(JSON.stringify({ alias, path: targetProject.dbPath }));
        } else {
          console.log(`Linked project '${alias}' -> ${targetProject.root}`);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
