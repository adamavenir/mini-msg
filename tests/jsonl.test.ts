import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import fs from 'fs';
import path from 'path';
import os from 'os';
import {
  appendMessage,
  appendAgent,
  readMessages,
  readAgents,
  readProjectConfig,
  updateProjectConfig,
} from '../src/db/jsonl.ts';
import type { Agent, Message } from '../src/types.ts';

describe('jsonl storage', () => {
  let tempDir: string;

  beforeEach(() => {
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-jsonl-test-'));
  });

  afterEach(() => {
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  it('should append and read messages', () => {
    const message: Message = {
      id: 'msg-abc12345',
      ts: 123,
      channel_id: 'ch-00000000',
      from_agent: 'alice.1',
      body: 'hello',
      mentions: ['bob.1'],
      type: 'agent',
      reply_to: null,
      edited_at: null,
      archived_at: null,
    };

    appendMessage(tempDir, message);
    const readBack = readMessages(tempDir);

    expect(readBack.length).toBe(1);
    expect(readBack[0].id).toBe(message.id);
    expect(readBack[0].mentions).toEqual(['bob.1']);
    expect(readBack[0].channel_id).toBe('ch-00000000');
  });

  it('should append and read agents', () => {
    const agent: Agent = {
      guid: 'usr-abc12345',
      agent_id: 'alice.1',
      goal: 'test',
      bio: null,
      registered_at: 100,
      last_seen: 120,
      left_at: null,
    };

    appendAgent(tempDir, agent);
    const readBack = readAgents(tempDir);

    expect(readBack.length).toBe(1);
    expect(readBack[0].id).toBe(agent.guid);
    expect(readBack[0].agent_id).toBe(agent.agent_id);
  });

  it('should merge project config updates', () => {
    updateProjectConfig(tempDir, {
      channel_id: 'ch-11111111',
      channel_name: 'Alpha',
      known_agents: {
        'usr-aaa11111': { name: 'alice.1' },
      },
    });

    updateProjectConfig(tempDir, {
      known_agents: {
        'usr-aaa11111': { global_name: 'alpha-alice' },
        'usr-bbb22222': { name: 'bob.1' },
      },
    });

    const config = readProjectConfig(tempDir);
    expect(config?.channel_id).toBe('ch-11111111');
    expect(config?.known_agents?.['usr-aaa11111']?.name).toBe('alice.1');
    expect(config?.known_agents?.['usr-aaa11111']?.global_name).toBe('alpha-alice');
    expect(config?.known_agents?.['usr-bbb22222']?.name).toBe('bob.1');
  });

  it('should append without overwriting existing JSONL', () => {
    const msg1: Message = {
      id: 'msg-first111',
      ts: 1,
      channel_id: null,
      from_agent: 'alice.1',
      body: 'first',
      mentions: [],
      type: 'agent',
      reply_to: null,
      edited_at: null,
      archived_at: null,
    };
    const msg2: Message = {
      id: 'msg-second1',
      ts: 2,
      channel_id: null,
      from_agent: 'bob.1',
      body: 'second',
      mentions: [],
      type: 'agent',
      reply_to: null,
      edited_at: null,
      archived_at: null,
    };

    appendMessage(tempDir, msg1);
    appendMessage(tempDir, msg2);

    const filePath = path.join(tempDir, '.mm', 'messages.jsonl');
    const lines = fs.readFileSync(filePath, 'utf8').trim().split('\n');
    expect(lines.length).toBe(2);

    const first = JSON.parse(lines[0]) as { id: string };
    const second = JSON.parse(lines[1]) as { id: string };
    expect(first.id).toBe(msg1.id);
    expect(second.id).toBe(msg2.id);
  });

  it('should skip malformed JSONL lines', () => {
    const mmDir = path.join(tempDir, '.mm');
    fs.mkdirSync(mmDir, { recursive: true });
    const filePath = path.join(mmDir, 'messages.jsonl');
    fs.writeFileSync(
      filePath,
      `{"type":"message","id":"msg-good1","mentions":[]}\n` +
      `not-json\n` +
      `{"type":"message","id":"msg-good2","mentions":[]}\n`,
      'utf8'
    );

    const readBack = readMessages(tempDir);
    expect(readBack.length).toBe(2);
    expect(readBack[0].id).toBe('msg-good1');
    expect(readBack[1].id).toBe('msg-good2');
  });
});
