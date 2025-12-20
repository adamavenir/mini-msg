import fs from 'fs';
import path from 'path';
import { discoverProject, type MmProject } from './project.js';
import { readGlobalConfig, findChannelByRef } from './config.js';
import { readProjectConfig, type ProjectConfig } from '../db/jsonl.js';
import { normalizeAgentRef } from './agents.js';

export interface ChannelContext {
  project: MmProject;
  channelId: string;
  channelName?: string;
  projectConfig: ProjectConfig | null;
}

function loadProjectConfig(project: MmProject): ProjectConfig | null {
  return readProjectConfig(project.dbPath);
}

function resolveProjectFromPath(rootPath: string): MmProject {
  const dbPath = path.join(rootPath, '.mm', 'mm.db');
  if (!fs.existsSync(dbPath)) {
    throw new Error(`Channel database not found at ${dbPath}`);
  }
  return { root: rootPath, dbPath };
}

export function resolveChannelContext(options?: {
  channel?: string;
  cwd?: string;
}): ChannelContext {
  const channelRef = options?.channel;
  const cwd = options?.cwd ?? process.cwd();
  const globalConfig = readGlobalConfig();

  if (channelRef) {
    const resolved = findChannelByRef(channelRef, globalConfig);
    if (resolved) {
      const project = resolveProjectFromPath(resolved.path);
      const projectConfig = loadProjectConfig(project);
      return {
        project,
        channelId: resolved.id,
        channelName: resolved.name,
        projectConfig,
      };
    }

    try {
      const localProject = discoverProject(cwd);
      const localConfig = loadProjectConfig(localProject);
      if (localConfig?.channel_id &&
        (localConfig.channel_id === channelRef || localConfig.channel_name === channelRef)) {
        return {
          project: localProject,
          channelId: localConfig.channel_id,
          channelName: localConfig.channel_name,
          projectConfig: localConfig,
        };
      }
    } catch {
      // Ignore local lookup errors and fall through
    }

    throw new Error(`Channel not found: ${channelRef}`);
  }

  if (globalConfig?.current_channel) {
    const resolved = findChannelByRef(globalConfig.current_channel, globalConfig);
    if (!resolved) {
      throw new Error(`Current channel not found: ${globalConfig.current_channel}`);
    }
    const project = resolveProjectFromPath(resolved.path);
    const projectConfig = loadProjectConfig(project);
    return {
      project,
      channelId: resolved.id,
      channelName: resolved.name,
      projectConfig,
    };
  }

  const localProject = discoverProject(cwd);
  const localConfig = loadProjectConfig(localProject);
  if (!localConfig?.channel_id) {
    throw new Error('No channel context');
  }

  return {
    project: localProject,
    channelId: localConfig.channel_id,
    channelName: localConfig.channel_name,
    projectConfig: localConfig,
  };
}

export function resolveAgentRef(ref: string, config: ProjectConfig | null): string {
  const normalized = normalizeAgentRef(ref);
  const known = config?.known_agents;
  if (!known) return normalized;

  for (const entry of Object.values(known)) {
    if (entry.name === normalized) {
      return normalized;
    }
    if (entry.global_name === normalized) {
      return entry.name;
    }
    if (entry.nicks && entry.nicks.includes(normalized)) {
      return entry.name;
    }
  }

  return normalized;
}
