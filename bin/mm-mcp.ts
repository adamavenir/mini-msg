#!/usr/bin/env node
import { MmMcpServer } from '../src/mcp/server.js';

async function main() {
  const projectPath = process.argv[2];

  if (!projectPath) {
    console.error('Usage: mm-mcp <project-path>');
    console.error('');
    console.error('Example:');
    console.error('  mm-mcp /Users/adam/dev/myproject');
    console.error('');
    console.error('Configure in Claude Desktop (~/Library/Application Support/Claude/claude_desktop_config.json):');
    console.error('  {');
    console.error('    "mcpServers": {');
    console.error('      "mm-myproject": {');
    console.error('        "command": "npx",');
    console.error('        "args": ["-y", "mm-mcp", "/Users/adam/dev/myproject"]');
    console.error('      }');
    console.error('    }');
    console.error('  }');
    process.exit(1);
  }

  try {
    const server = new MmMcpServer(projectPath);

    // Handle graceful shutdown
    const shutdown = async () => {
      await server.close();
      process.exit(0);
    };

    process.on('SIGINT', shutdown);
    process.on('SIGTERM', shutdown);

    await server.run();
  } catch (error) {
    console.error('Failed to start MCP server:', error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}

main().catch((error) => {
  console.error('Fatal error:', error);
  process.exit(1);
});
