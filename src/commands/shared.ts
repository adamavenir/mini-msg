import { Command } from 'commander';
import Database from 'better-sqlite3';
import { existsSync } from 'fs';
import { discoverProject, openDatabase } from '../core/project.js';
import { resolveChannelContext } from '../core/context.js';
import { initSchema } from '../db/schema.js';
import { getLinkedProject } from '../db/queries.js';
import { readProjectConfig } from '../db/jsonl.js';

export interface CommandContext {
  db: Database.Database;
  project: ReturnType<typeof discoverProject>;
  jsonMode: boolean;
  channelId?: string;
  channelName?: string;
  projectConfig?: ReturnType<typeof readProjectConfig>;
}

/**
 * Get database and context for a command.
 * Handles --project flag for operating in linked project.
 */
export function getContext(cmd: Command): CommandContext {
  const opts = cmd.optsWithGlobals();
  const projectAlias = opts.project;
  const jsonMode = opts.json || false;
  const channelRef = opts.in as string | undefined;

  let project;

  if (projectAlias) {
    // Operating in linked project
    const mainProject = discoverProject();
    const mainDb = openDatabase(mainProject);
    initSchema(mainDb);

    const linkedProj = getLinkedProject(mainDb, projectAlias);
    if (!linkedProj) {
      throw new Error(`Linked project '${projectAlias}' not found. Use 'mm link' first.`);
    }

    mainDb.close();

    // Validate the linked project's database exists
    if (!existsSync(linkedProj.path)) {
      throw new Error(
        `Linked project '${projectAlias}' database not found at ${linkedProj.path}. ` +
        `The project may have been moved or deleted. Use 'mm unlink ${projectAlias}' and re-link.`
      );
    }

    // Open the linked project's database
    const linkedDb = new Database(linkedProj.path);
    linkedDb.pragma('foreign_keys = ON');
    linkedDb.pragma('journal_mode = WAL');
    linkedDb.pragma('busy_timeout = 5000');
    initSchema(linkedDb);

    return {
      db: linkedDb,
      project: { root: linkedProj.path, dbPath: linkedProj.path },
      jsonMode,
      projectConfig: readProjectConfig(linkedProj.path),
    };
  } else {
    const channelContext = resolveChannelContext({ channel: channelRef });
    project = channelContext.project;
    const db = openDatabase(channelContext.project);
    initSchema(db);

    return {
      db,
      project,
      jsonMode,
      channelId: channelContext.channelId,
      channelName: channelContext.channelName,
      projectConfig: channelContext.projectConfig,
    };
  }
}

/**
 * Format error message and exit.
 */
export function handleError(error: unknown): never {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`Error: ${message}`);
  process.exit(1);
}
