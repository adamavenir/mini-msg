import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import type Database from 'better-sqlite3';
import { discoverProject, openDatabase, type MmProject } from '../core/project.js';
import { initSchema } from '../db/schema.js';
import { initializeMcpAgent } from './identity.js';
import { registerTools, type McpContext } from './tools.js';

/**
 * MM MCP Server.
 * Provides mm messaging tools to Claude Desktop via MCP protocol.
 */
export class MmMcpServer {
  private server: Server;
  private project: MmProject;
  private db: Database.Database;
  private agentId: string;

  constructor(projectPath?: string) {
    // Discover project
    this.project = discoverProject(projectPath);
    this.log(`Discovered project: ${this.project.root}`);

    // Open database and initialize schema
    this.db = openDatabase(this.project);
    initSchema(this.db);
    this.log(`Database opened: ${this.project.dbPath}`);

    // Initialize agent identity
    this.agentId = initializeMcpAgent(this.db, this.project.root);
    this.log(`Agent initialized: ${this.agentId}`);

    // Create MCP server
    this.server = new Server(
      {
        name: 'mm',
        version: '0.1.0',
      },
      {
        capabilities: {
          tools: {},
        },
      }
    );

    // Register tools
    registerTools(this.server, () => this.getContext());
    this.log('Tools registered');
  }

  /**
   * Log to stderr (stdout reserved for MCP protocol).
   */
  private log(message: string): void {
    console.error(`[mm-mcp] ${message}`);
  }

  /**
   * Get current context for tool handlers.
   */
  private getContext(): McpContext {
    return {
      agentId: this.agentId,
      db: this.db,
    };
  }

  /**
   * Run the MCP server with stdio transport.
   */
  async run(): Promise<void> {
    const transport = new StdioServerTransport();
    await this.server.connect(transport);
    this.log('Connected via stdio');
  }

  /**
   * Close the server and cleanup resources.
   */
  async close(): Promise<void> {
    this.db.close();
    await this.server.close();
    this.log('Server closed');
  }
}
