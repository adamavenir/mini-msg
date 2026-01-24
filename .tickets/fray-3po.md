---
id: fray-3po
status: closed
deps: []
links: []
created: 2025-12-04T09:56:11.150341-08:00
type: task
priority: 1
parent: fray-kit
---
# Initialize npm project with ESM configuration

Initialize the npm project in the current working directory with proper ESM configuration.

## Assumptions
- Working directory is ~/dev/beads-messenger/ (already exists)
- This is a fresh project with no existing package.json

## Tasks
1. Create package.json with:
   - name: "beads-messenger"
   - type: "module" (ESM)
   - version: "0.1.0"
   - bin entry: { "bdm": "./dist/bin/bdm.js" }
   - scripts: build, test, dev

2. Create tsconfig.json:
   - target: ES2022
   - module: ESNext
   - moduleResolution: bundler
   - strict: true
   - outDir: dist
   - rootDir: . (to include both src/ and bin/)

3. Create tsup.config.ts for bundling:
   - entry: ["src/cli.ts", "bin/bdm.ts"]
   - format: esm
   - dts: true
   - clean: true

## Files to Create
- package.json
- tsconfig.json
- tsup.config.ts

## Acceptance Criteria
- `npm install` succeeds (after dependencies added in next task)
- TypeScript configuration is valid
- ESM imports work correctly


