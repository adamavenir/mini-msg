import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['src/cli.ts', 'bin/mm.ts', 'bin/mm-mcp.ts'],
  format: ['esm'],
  dts: true,
  clean: true,
  splitting: false,
  sourcemap: true,
  outDir: 'dist',
});
