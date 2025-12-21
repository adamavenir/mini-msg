import type { Server } from '@modelcontextprotocol/sdk/server/index.js';
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from '@modelcontextprotocol/sdk/types.js';
import type Database from 'better-sqlite3';
import {
  getAgent,
  getMessages,
  getMessagesWithMention,
  getActiveAgents,
  createMessage,
  updateAgent,
  getConfig,
  createClaim,
  deleteClaimsByAgent,
  deleteClaim,
  getClaimsByAgent,
  getAllClaims,
  pruneExpiredClaims,
  getClaimCountsByAgent,
} from '../db/queries.js';
import { extractMentions } from '../core/mentions.js';
import { parseAgentId } from '../core/agents.js';
import type { Message, ClaimType } from '../types.js';

export interface McpContext {
  agentId: string;
  db: Database.Database;
}

/**
 * Format a message for display.
 */
function formatMessage(msg: Message): string {
  const mentions = msg.mentions.length > 0 ? ` [mentions: ${msg.mentions.join(', ')}]` : '';
  return `[#${msg.id}] @${msg.from_agent}: ${msg.body}${mentions}`;
}

/**
 * Register all mm tools on the MCP server.
 */
export function registerTools(
  server: Server,
  getContext: () => McpContext
): void {
  // List available tools
  server.setRequestHandler(ListToolsRequestSchema, async () => {
    return {
      tools: [
        {
          name: 'mm_post',
          description: 'Post a message to the mm room. Use @mentions to direct messages to specific agents.',
          inputSchema: {
            type: 'object' as const,
            properties: {
              body: {
                type: 'string',
                description: 'Message body. Supports @mentions like @alice.1 or @all for broadcast.',
              },
            },
            required: ['body'],
          },
        },
        {
          name: 'mm_get',
          description: 'Get recent room messages. Use for catching up on conversation.',
          inputSchema: {
            type: 'object' as const,
            properties: {
              since: {
                type: 'string',
                description: 'Get messages after this GUID (for polling new messages)',
              },
              limit: {
                type: 'number',
                description: 'Maximum number of messages to return (default: 10)',
              },
            },
          },
        },
        {
          name: 'mm_mentions',
          description: 'Get messages that mention me. Use to check for messages directed at you.',
          inputSchema: {
            type: 'object' as const,
            properties: {
              since: {
                type: 'string',
                description: 'Get mentions after this GUID (for polling)',
              },
              limit: {
                type: 'number',
                description: 'Maximum number of mentions to return (default: 10)',
              },
            },
          },
        },
        {
          name: 'mm_here',
          description: 'List active agents in the room. See who is available to collaborate.',
          inputSchema: {
            type: 'object' as const,
            properties: {},
          },
        },
        {
          name: 'mm_whoami',
          description: 'Show my agent identity. Returns your agent ID and status.',
          inputSchema: {
            type: 'object' as const,
            properties: {},
          },
        },
        {
          name: 'mm_claim',
          description: 'Claim resources (files, bd issues, GitHub issues) to prevent collision with other agents.',
          inputSchema: {
            type: 'object' as const,
            properties: {
              files: {
                type: 'array',
                items: { type: 'string' },
                description: 'File paths or glob patterns to claim (e.g., ["src/auth.ts", "lib/*.ts"])',
              },
              bd: {
                type: 'array',
                items: { type: 'string' },
                description: 'Beads issue IDs to claim (e.g., ["xyz-123"])',
              },
              issues: {
                type: 'array',
                items: { type: 'string' },
                description: 'GitHub issue numbers to claim (e.g., ["456"])',
              },
              reason: {
                type: 'string',
                description: 'Reason for claim (optional)',
              },
              ttl_minutes: {
                type: 'number',
                description: 'Time to live in minutes (optional, default: no expiry)',
              },
            },
          },
        },
        {
          name: 'mm_clear',
          description: 'Clear claims. Clear all your claims or specific ones.',
          inputSchema: {
            type: 'object' as const,
            properties: {
              file: {
                type: 'string',
                description: 'Specific file claim to clear (optional)',
              },
              bd: {
                type: 'string',
                description: 'Specific bd issue claim to clear (optional)',
              },
              issue: {
                type: 'string',
                description: 'Specific GitHub issue claim to clear (optional)',
              },
            },
          },
        },
        {
          name: 'mm_claims',
          description: 'List active claims. Shows who has claimed what resources.',
          inputSchema: {
            type: 'object' as const,
            properties: {
              agent: {
                type: 'string',
                description: 'Filter by agent ID (optional, defaults to all agents)',
              },
            },
          },
        },
        {
          name: 'mm_status',
          description: 'Update your status with optional resource claims. Sets your status and claims resources in one operation.',
          inputSchema: {
            type: 'object' as const,
            properties: {
              message: {
                type: 'string',
                description: 'Status message (your current task/focus)',
              },
              files: {
                type: 'array',
                items: { type: 'string' },
                description: 'File paths or glob patterns to claim',
              },
              bd: {
                type: 'array',
                items: { type: 'string' },
                description: 'Beads issue IDs to claim',
              },
              issues: {
                type: 'array',
                items: { type: 'string' },
                description: 'GitHub issue numbers to claim',
              },
              ttl_minutes: {
                type: 'number',
                description: 'Time to live in minutes for claims',
              },
              clear: {
                type: 'boolean',
                description: 'Clear all claims and reset status',
              },
            },
          },
        },
      ],
    };
  });

  // Handle tool calls
  server.setRequestHandler(CallToolRequestSchema, async (request) => {
    const { name, arguments: args } = request.params;
    const ctx = getContext();

    try {
      switch (name) {
        case 'mm_post':
          return handlePost(ctx, args as { body: string });

        case 'mm_get':
          return handleGet(ctx, args as { since?: string; limit?: number });

        case 'mm_mentions':
          return handleMentions(ctx, args as { since?: string; limit?: number });

        case 'mm_here':
          return handleHere(ctx);

        case 'mm_whoami':
          return handleWhoami(ctx);

        case 'mm_claim':
          return handleClaim(ctx, args as {
            files?: string[];
            bd?: string[];
            issues?: string[];
            reason?: string;
            ttl_minutes?: number;
          });

        case 'mm_clear':
          return handleClear(ctx, args as {
            file?: string;
            bd?: string;
            issue?: string;
          });

        case 'mm_claims':
          return handleClaims(ctx, args as { agent?: string });

        case 'mm_status':
          return handleStatus(ctx, args as {
            message?: string;
            files?: string[];
            bd?: string[];
            issues?: string[];
            ttl_minutes?: number;
            clear?: boolean;
          });

        default:
          return {
            content: [{ type: 'text' as const, text: `Unknown tool: ${name}` }],
            isError: true,
          };
      }
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : String(error);
      return {
        content: [{ type: 'text' as const, text: `Error: ${errorMsg}` }],
        isError: true,
      };
    }
  });
}

/**
 * Handle mm_post tool.
 */
function handlePost(
  ctx: McpContext,
  args: { body: string }
): { content: Array<{ type: 'text'; text: string }> } {
  const { agentId, db } = ctx;
  const { body } = args;

  if (!body || body.trim().length === 0) {
    return {
      content: [{ type: 'text', text: 'Error: Message body cannot be empty' }],
    };
  }

  // Verify agent exists and hasn't left
  const agent = getAgent(db, agentId);
  if (!agent) {
    return {
      content: [{ type: 'text', text: `Error: Agent ${agentId} not found` }],
    };
  }
  if (agent.left_at !== null) {
    return {
      content: [{ type: 'text', text: `Error: Agent ${agentId} has left the room` }],
    };
  }

  // Extract mentions from body
  const mentions = extractMentions(body, db);

  // Create message
  const message = createMessage(db, {
    from_agent: agentId,
    body: body.trim(),
    mentions,
    type: 'agent',
  });

  // Update last_seen
  updateAgent(db, agentId, { last_seen: Math.floor(Date.now() / 1000) });

  const mentionInfo = mentions.length > 0 ? ` (mentioned: ${mentions.join(', ')})` : '';
  return {
    content: [{ type: 'text', text: `Posted message #${message.id}${mentionInfo}` }],
  };
}

/**
 * Handle mm_get tool.
 */
function handleGet(
  ctx: McpContext,
  args: { since?: string; limit?: number }
): { content: Array<{ type: 'text'; text: string }> } {
  const { db } = ctx;
  const limit = args.limit ?? 10;
  const since = args.since?.replace(/^@?#/, '');

  const messages = getMessages(db, { since, limit });

  if (messages.length === 0) {
    const qualifier = since ? ` after message #${since}` : '';
    return {
      content: [{ type: 'text', text: `No messages${qualifier}` }],
    };
  }

  const formatted = messages.map(formatMessage).join('\n');
  const header = since
    ? `Messages after #${since} (${messages.length}):`
    : `Recent messages (${messages.length}):`;

  return {
    content: [{ type: 'text', text: `${header}\n\n${formatted}` }],
  };
}

/**
 * Handle mm_mentions tool.
 */
function handleMentions(
  ctx: McpContext,
  args: { since?: string; limit?: number }
): { content: Array<{ type: 'text'; text: string }> } {
  const { agentId, db } = ctx;
  const limit = args.limit ?? 10;
  const since = args.since?.replace(/^@?#/, '');

  // Extract base from agent ID for prefix matching
  const parsed = parseAgentId(agentId);
  const messages = getMessagesWithMention(db, parsed.base, { since, limit });

  if (messages.length === 0) {
    const qualifier = since ? ` after message #${since}` : '';
    return {
      content: [{ type: 'text', text: `No mentions${qualifier}` }],
    };
  }

  const formatted = messages.map(formatMessage).join('\n');
  const header = since
    ? `Mentions after #${since} (${messages.length}):`
    : `Recent mentions (${messages.length}):`;

  return {
    content: [{ type: 'text', text: `${header}\n\n${formatted}` }],
  };
}

/**
 * Handle mm_here tool.
 */
function handleHere(
  ctx: McpContext
): { content: Array<{ type: 'text'; text: string }> } {
  const { db } = ctx;

  // Get stale_hours config (default: 4)
  const staleHoursStr = getConfig(db, 'stale_hours');
  const staleHours = staleHoursStr ? parseInt(staleHoursStr, 10) : 4;

  const agents = getActiveAgents(db, staleHours);

  if (agents.length === 0) {
    return {
      content: [{ type: 'text', text: 'No active agents' }],
    };
  }

  const lines = agents.map((agent) => {
    const status = agent.status ? `: "${agent.status}"` : '';
    return `  ${agent.agent_id}${status}`;
  });

  return {
    content: [{ type: 'text', text: `Active agents (${agents.length}):\n${lines.join('\n')}` }],
  };
}

/**
 * Handle mm_whoami tool.
 */
function handleWhoami(
  ctx: McpContext
): { content: Array<{ type: 'text'; text: string }> } {
  const { agentId, db } = ctx;

  const agent = getAgent(db, agentId);
  if (!agent) {
    return {
      content: [{ type: 'text', text: `Agent ID: ${agentId} (not found in database)` }],
    };
  }

  const activeStatus = agent.left_at ? 'left' : 'active';
  const currentStatus = agent.status ? `\nStatus: ${agent.status}` : '';
  const purpose = agent.purpose ? `\nPurpose: ${agent.purpose}` : '';

  return {
    content: [{ type: 'text', text: `Agent ID: ${agentId}\nActive: ${activeStatus}${currentStatus}${purpose}` }],
  };
}

/**
 * Strip # prefix from bd/issue values.
 */
function stripHash(value: string): string {
  return value.startsWith('#') ? value.substring(1) : value;
}

/**
 * Handle mm_claim tool.
 */
function handleClaim(
  ctx: McpContext,
  args: {
    files?: string[];
    bd?: string[];
    issues?: string[];
    reason?: string;
    ttl_minutes?: number;
  }
): { content: Array<{ type: 'text'; text: string }> } {
  const { agentId, db } = ctx;

  // Verify agent exists
  const agent = getAgent(db, agentId);
  if (!agent) {
    return {
      content: [{ type: 'text', text: `Error: Agent ${agentId} not found` }],
    };
  }

  // Prune expired claims first
  pruneExpiredClaims(db);

  // Calculate expiration if TTL provided
  let expiresAt: number | null = null;
  if (args.ttl_minutes) {
    expiresAt = Math.floor(Date.now() / 1000) + args.ttl_minutes * 60;
  }

  const claims: { type: ClaimType; pattern: string }[] = [];

  // Collect file claims
  if (args.files) {
    for (const pattern of args.files) {
      claims.push({ type: 'file', pattern });
    }
  }

  // Collect bd claims
  if (args.bd) {
    for (const id of args.bd) {
      claims.push({ type: 'bd', pattern: stripHash(id) });
    }
  }

  // Collect issue claims
  if (args.issues) {
    for (const id of args.issues) {
      claims.push({ type: 'issue', pattern: stripHash(id) });
    }
  }

  if (claims.length === 0) {
    return {
      content: [{ type: 'text', text: 'Error: No claims specified. Provide files, bd, or issues.' }],
    };
  }

  // Create claims
  const created: { type: ClaimType; pattern: string }[] = [];
  const errors: string[] = [];

  for (const claim of claims) {
    try {
      createClaim(db, {
        agent_id: agentId,
        claim_type: claim.type,
        pattern: claim.pattern,
        reason: args.reason ?? null,
        expires_at: expiresAt,
      });
      created.push(claim);
    } catch (error) {
      errors.push(error instanceof Error ? error.message : String(error));
    }
  }

  // Post message about claims
  if (created.length > 0) {
    const claimList = created.map(c => {
      if (c.type === 'file') return c.pattern;
      return `${c.type}:${c.pattern}`;
    }).join(', ');

    createMessage(db, {
      from_agent: agentId,
      body: `claimed: ${claimList}`,
      mentions: [],
    });
  }

  let result = `Claimed ${created.length} resource${created.length !== 1 ? 's' : ''}`;
  if (created.length > 0) {
    const list = created.map(c => c.type === 'file' ? c.pattern : `${c.type}:${c.pattern}`);
    result += `:\n  ${list.join('\n  ')}`;
  }
  if (errors.length > 0) {
    result += `\n\nErrors:\n  ${errors.join('\n  ')}`;
  }
  if (expiresAt) {
    result += `\n\nExpires in ${args.ttl_minutes} minutes`;
  }

  return {
    content: [{ type: 'text', text: result }],
  };
}

/**
 * Handle mm_clear tool.
 */
function handleClear(
  ctx: McpContext,
  args: {
    file?: string;
    bd?: string;
    issue?: string;
  }
): { content: Array<{ type: 'text'; text: string }> } {
  const { agentId, db } = ctx;

  // Verify agent exists
  const agent = getAgent(db, agentId);
  if (!agent) {
    return {
      content: [{ type: 'text', text: `Error: Agent ${agentId} not found` }],
    };
  }

  let cleared = 0;
  const clearedItems: string[] = [];

  // Clear specific claims if provided
  if (args.file) {
    if (deleteClaim(db, 'file', args.file)) {
      cleared++;
      clearedItems.push(args.file);
    }
  }

  if (args.bd) {
    const pattern = stripHash(args.bd);
    if (deleteClaim(db, 'bd', pattern)) {
      cleared++;
      clearedItems.push(`bd:${pattern}`);
    }
  }

  if (args.issue) {
    const pattern = stripHash(args.issue);
    if (deleteClaim(db, 'issue', pattern)) {
      cleared++;
      clearedItems.push(`issue:${pattern}`);
    }
  }

  // If no specific claims specified, clear all
  if (!args.file && !args.bd && !args.issue) {
    const existingClaims = getClaimsByAgent(db, agentId);
    cleared = deleteClaimsByAgent(db, agentId);
    for (const claim of existingClaims) {
      if (claim.claim_type === 'file') {
        clearedItems.push(claim.pattern);
      } else {
        clearedItems.push(`${claim.claim_type}:${claim.pattern}`);
      }
    }
  }

  // Post message if anything was cleared
  if (cleared > 0) {
    createMessage(db, {
      from_agent: agentId,
      body: `cleared claims: ${clearedItems.join(', ')}`,
      mentions: [],
    });
  }

  if (cleared === 0) {
    return {
      content: [{ type: 'text', text: 'No claims to clear' }],
    };
  }

  return {
    content: [{ type: 'text', text: `Cleared ${cleared} claim${cleared !== 1 ? 's' : ''}:\n  ${clearedItems.join('\n  ')}` }],
  };
}

/**
 * Handle mm_claims tool.
 */
function handleClaims(
  ctx: McpContext,
  args: { agent?: string }
): { content: Array<{ type: 'text'; text: string }> } {
  const { db } = ctx;

  let claims;
  if (args.agent) {
    claims = getClaimsByAgent(db, args.agent);
  } else {
    claims = getAllClaims(db);
  }

  if (claims.length === 0) {
    const qualifier = args.agent ? ` for @${args.agent}` : '';
    return {
      content: [{ type: 'text', text: `No active claims${qualifier}` }],
    };
  }

  // Group by agent
  const byAgent = new Map<string, typeof claims>();
  for (const claim of claims) {
    const existing = byAgent.get(claim.agent_id) || [];
    existing.push(claim);
    byAgent.set(claim.agent_id, existing);
  }

  const lines: string[] = [`Active claims (${claims.length}):`];

  for (const [agent, agentClaims] of byAgent) {
    lines.push(`\n@${agent}:`);
    for (const claim of agentClaims) {
      const typePrefix = claim.claim_type === 'file' ? '' : `${claim.claim_type}:`;
      const reason = claim.reason ? ` - ${claim.reason}` : '';
      lines.push(`  ${typePrefix}${claim.pattern}${reason}`);
    }
  }

  return {
    content: [{ type: 'text', text: lines.join('\n') }],
  };
}

/**
 * Handle mm_status tool.
 */
function handleStatus(
  ctx: McpContext,
  args: {
    message?: string;
    files?: string[];
    bd?: string[];
    issues?: string[];
    ttl_minutes?: number;
    clear?: boolean;
  }
): { content: Array<{ type: 'text'; text: string }> } {
  const { agentId, db } = ctx;

  // Verify agent exists
  const agent = getAgent(db, agentId);
  if (!agent) {
    return {
      content: [{ type: 'text', text: `Error: Agent ${agentId} not found` }],
    };
  }

  // Handle --clear
  if (args.clear) {
    const clearedCount = deleteClaimsByAgent(db, agentId);
    updateAgent(db, agentId, { status: null, last_seen: Math.floor(Date.now() / 1000) });

    createMessage(db, {
      from_agent: agentId,
      body: clearedCount > 0
        ? `status cleared (released ${clearedCount} claim${clearedCount !== 1 ? 's' : ''})`
        : 'status cleared',
      mentions: [],
    });

    return {
      content: [{ type: 'text', text: `Status cleared${clearedCount > 0 ? `, released ${clearedCount} claim${clearedCount !== 1 ? 's' : ''}` : ''}` }],
    };
  }

  // Prune expired claims first
  pruneExpiredClaims(db);

  // Calculate expiration if TTL provided
  let expiresAt: number | null = null;
  if (args.ttl_minutes) {
    expiresAt = Math.floor(Date.now() / 1000) + args.ttl_minutes * 60;
  }

  // Collect claims
  const claims: { type: ClaimType; pattern: string }[] = [];

  if (args.files) {
    for (const pattern of args.files) {
      claims.push({ type: 'file', pattern });
    }
  }

  if (args.bd) {
    for (const id of args.bd) {
      claims.push({ type: 'bd', pattern: stripHash(id) });
    }
  }

  if (args.issues) {
    for (const id of args.issues) {
      claims.push({ type: 'issue', pattern: stripHash(id) });
    }
  }

  // Create claims
  const created: { type: ClaimType; pattern: string }[] = [];
  const errors: string[] = [];

  for (const claim of claims) {
    try {
      createClaim(db, {
        agent_id: agentId,
        claim_type: claim.type,
        pattern: claim.pattern,
        reason: args.message ?? null,
        expires_at: expiresAt,
      });
      created.push(claim);
    } catch (error) {
      errors.push(error instanceof Error ? error.message : String(error));
    }
  }

  // Update status if message provided
  if (args.message) {
    updateAgent(db, agentId, { status: args.message, last_seen: Math.floor(Date.now() / 1000) });
  } else {
    updateAgent(db, agentId, { last_seen: Math.floor(Date.now() / 1000) });
  }

  // Build status message
  let body = args.message || 'status update';
  if (created.length > 0) {
    const claimList = created.map(c => {
      if (c.type === 'file') return c.pattern;
      return `${c.type}:${c.pattern}`;
    }).join(', ');
    body = args.message
      ? `${args.message} [claimed: ${claimList}]`
      : `claimed: ${claimList}`;
  }

  // Post status message
  createMessage(db, {
    from_agent: agentId,
    body,
    mentions: [],
  });

  let result = args.message ? `Status: ${args.message}` : 'Status updated';
  if (created.length > 0) {
    const list = created.map(c => c.type === 'file' ? c.pattern : `${c.type}:${c.pattern}`);
    result += `\n\nClaimed:\n  ${list.join('\n  ')}`;
  }
  if (errors.length > 0) {
    result += `\n\nErrors:\n  ${errors.join('\n  ')}`;
  }

  return {
    content: [{ type: 'text', text: result }],
  };
}
