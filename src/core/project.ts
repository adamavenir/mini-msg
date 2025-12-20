import fs from 'fs';
import path from 'path';
import Database from 'better-sqlite3';
import { rebuildDatabaseFromJsonl } from '../db/jsonl.js';

export interface MmProject {
  root: string;      // Absolute path to project root
  dbPath: string;    // Absolute path to .mm/mm.db
}

/**
 * Discover mm project from current directory.
 * Walks up the directory tree looking for .mm/
 * @throws Error if not in an mm project
 */
export function discoverProject(startDir?: string): MmProject {
  let currentDir = startDir ? path.resolve(startDir) : process.cwd();

  while (true) {
    const mmDir = path.join(currentDir, '.mm');

    if (fs.existsSync(mmDir) && fs.statSync(mmDir).isDirectory()) {
      const dbPath = path.join(mmDir, 'mm.db');

      if (!fs.existsSync(dbPath)) {
        throw new Error("mm database not found. Run 'mm init' first.");
      }

      return {
        root: currentDir,
        dbPath
      };
    }

    const parentDir = path.dirname(currentDir);

    // Reached filesystem root without finding .mm/
    if (parentDir === currentDir) {
      throw new Error("Not initialized. Run 'mm init' first.");
    }

    currentDir = parentDir;
  }
}

/**
 * Initialize a new mm project in the given directory.
 * Creates .mm/ directory and mm.db database.
 * @throws Error if already initialized (unless force=true)
 */
export function initProject(dir?: string, force?: boolean): MmProject {
  const root = dir ? path.resolve(dir) : process.cwd();
  const mmDir = path.join(root, '.mm');
  const dbPath = path.join(mmDir, 'mm.db');

  if (fs.existsSync(mmDir) && !force) {
    throw new Error("Already initialized. Use --force to reinitialize.");
  }

  // Create .mm directory
  if (!fs.existsSync(mmDir)) {
    fs.mkdirSync(mmDir, { recursive: true });
  }

  ensureMmGitignore(mmDir);

  // Remove existing database if force
  if (force && fs.existsSync(dbPath)) {
    fs.unlinkSync(dbPath);
  }

  return { root, dbPath };
}

/**
 * Open database connection to mm project.
 */
export function openDatabase(project: MmProject): Database.Database {
  const mmDir = path.dirname(project.dbPath);
  ensureMmGitignore(mmDir);
  const dbExists = fs.existsSync(project.dbPath);
  const jsonlMtime = getJsonlMtime(mmDir);
  const dbMtime = dbExists ? fs.statSync(project.dbPath).mtimeMs : 0;
  const shouldRebuild = jsonlMtime > 0 && (!dbExists || jsonlMtime > dbMtime);

  const db = new Database(project.dbPath);

  // Enable foreign keys
  db.pragma('foreign_keys = ON');

  // Enable WAL mode for better concurrency
  db.pragma('journal_mode = WAL');

  // Set busy timeout for multi-agent resilience
  db.pragma('busy_timeout = 5000');

  if (shouldRebuild) {
    console.log('Rebuilding SQLite from JSONL (stale database)');
    rebuildDatabaseFromJsonl(db, project.dbPath);
  }

  return db;
}

function ensureMmGitignore(mmDir: string): void {
  const gitignorePath = path.join(mmDir, '.gitignore');
  const entries = ['*.db', '*.db-wal', '*.db-shm'];

  if (!fs.existsSync(gitignorePath)) {
    fs.writeFileSync(gitignorePath, entries.join('\n') + '\n', 'utf8');
    return;
  }

  const existing = fs.readFileSync(gitignorePath, 'utf8');
  const lines = existing.split(/\r?\n/);
  const missing = entries.filter(entry => !lines.includes(entry));

  if (missing.length === 0) {
    return;
  }

  const needsNewline = existing.length > 0 && !existing.endsWith('\n');
  const prefix = needsNewline ? '\n' : '';
  fs.appendFileSync(gitignorePath, prefix + missing.join('\n') + '\n', 'utf8');
}

function getJsonlMtime(mmDir: string): number {
  const files = ['messages.jsonl', 'agents.jsonl'];
  let latest = 0;

  for (const file of files) {
    const filePath = path.join(mmDir, file);
    if (!fs.existsSync(filePath)) continue;
    const stat = fs.statSync(filePath);
    latest = Math.max(latest, stat.mtimeMs);
  }

  return latest;
}
