import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { discoverProject, openDatabase, initProject } from '../src/core/project.ts';
import { initSchema } from '../src/db/schema.ts';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('discoverProject', () => {
  let tempDir: string;
  let projectRoot: string;

  beforeAll(() => {
    // Create a temporary directory
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-test-'));
    projectRoot = path.join(tempDir, 'test-project');

    // Create mm project using initProject + openDatabase + initSchema
    fs.mkdirSync(projectRoot, { recursive: true });
    const project = initProject(projectRoot);
    const db = openDatabase(project);
    initSchema(db);
    db.close();
  });

  afterAll(() => {
    // Clean up
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  it('should discover project from root directory', () => {
    const project = discoverProject(projectRoot);

    expect(project.root).toBe(projectRoot);
    expect(project.dbPath).toContain('.mm');
    expect(project.dbPath).toContain('.db');
    expect(fs.existsSync(project.dbPath)).toBe(true);
  });

  it('should discover project from nested subdirectory', () => {
    const nestedDir = path.join(projectRoot, 'src', 'core');
    fs.mkdirSync(nestedDir, { recursive: true });

    const project = discoverProject(nestedDir);

    expect(project.root).toBe(projectRoot);
    expect(project.dbPath).toContain('.mm');
  });

  it('should throw error when not in an mm project', () => {
    const nonProjectDir = path.join(tempDir, 'not-a-project');
    fs.mkdirSync(nonProjectDir, { recursive: true });

    expect(() => discoverProject(nonProjectDir)).toThrow("Not initialized. Run 'mm init' first.");
  });

  it('should throw error when .mm exists but no .db file', () => {
    const emptyMmDir = path.join(tempDir, 'empty-mm');
    fs.mkdirSync(path.join(emptyMmDir, '.mm'), { recursive: true });

    expect(() => discoverProject(emptyMmDir)).toThrow("mm database not found. Run 'mm init' first.");
  });

  it('should open database successfully', () => {
    const project = discoverProject(projectRoot);
    const db = openDatabase(project);

    expect(db).toBeDefined();

    // Verify we can query the database
    const result = db.prepare('SELECT 1 as test').get() as { test: number };
    expect(result.test).toBe(1);

    db.close();
  });
});
