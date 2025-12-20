import fs from 'fs';
import path from 'path';
import os from 'os';

export interface GlobalConfig {
  version: number;
  channels: Record<string, {
    name: string;
    path: string;
  }>;
}

const CONFIG_DIR = path.join(os.homedir(), '.config', 'mm');
const CONFIG_PATH = path.join(CONFIG_DIR, 'mm-config.json');

function ensureConfigDir(): void {
  if (!fs.existsSync(CONFIG_DIR)) {
    fs.mkdirSync(CONFIG_DIR, { recursive: true });
  }
}

export function readGlobalConfig(): GlobalConfig | null {
  if (!fs.existsSync(CONFIG_PATH)) {
    return null;
  }
  const raw = fs.readFileSync(CONFIG_PATH, 'utf8');
  return JSON.parse(raw) as GlobalConfig;
}

export function writeGlobalConfig(config: GlobalConfig): void {
  ensureConfigDir();
  fs.writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 2) + '\n', 'utf8');
}

export function registerChannel(
  channelId: string,
  channelName: string,
  projectRoot: string
): GlobalConfig {
  const existing = readGlobalConfig() ?? { version: 1, channels: {} };
  const channels = {
    ...existing.channels,
    [channelId]: {
      name: channelName,
      path: projectRoot,
    },
  };

  const updated: GlobalConfig = {
    ...existing,
    version: existing.version ?? 1,
    channels,
  };

  writeGlobalConfig(updated);
  return updated;
}

export function findChannelByRef(
  ref: string,
  config: GlobalConfig | null
): { id: string; name: string; path: string } | null {
  if (!config) return null;
  const direct = config.channels[ref];
  if (direct) {
    return { id: ref, name: direct.name, path: direct.path };
  }

  for (const [id, channel] of Object.entries(config.channels)) {
    if (channel.name === ref) {
      return { id, name: channel.name, path: channel.path };
    }
  }

  return null;
}

