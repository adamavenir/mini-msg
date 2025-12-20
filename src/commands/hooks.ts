import { appendFileSync } from 'fs';

/**
 * Write MM_AGENT_ID to CLAUDE_ENV_FILE if in Claude Code hooks context.
 * Returns true if written, false otherwise.
 */
export function writeClaudeEnv(agentId: string): boolean {
  const envFile = process.env.CLAUDE_ENV_FILE;
  if (!envFile) {
    return false;
  }

  try {
    appendFileSync(envFile, `MM_AGENT_ID=${agentId}\n`);
    return true;
  } catch {
    // Silently fail - file might not be writable
    return false;
  }
}
