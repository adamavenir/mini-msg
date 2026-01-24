---
id: fray-376
status: closed
deps: [fray-wkx]
links: []
created: 2025-12-04T10:18:44.656474-08:00
type: task
priority: 1
parent: fray-po1
---
# Prepare npm package for publishing

Finalize package.json and prepare for npm publishing.

## Package.json Updates

```json
{
  "name": "beads-messenger",
  "version": "0.1.0",
  "description": "Agent messaging for beads projects",
  "type": "module",
  "main": "./dist/cli.js",
  "types": "./dist/cli.d.ts",
  "bin": {
    "bdm": "./dist/bin/bdm.js"
  },
  "files": [
    "dist",
    "README.md",
    "LICENSE"
  ],
  "scripts": {
    "build": "tsup",
    "test": "vitest",
    "prepublishOnly": "npm run build && npm test"
  },
  "keywords": [
    "beads",
    "agent",
    "messaging",
    "cli",
    "coordination"
  ],
  "repository": {
    "type": "git",
    "url": "https://github.com/user/beads-messenger"
  },
  "license": "MIT",
  "engines": {
    "node": ">=18"
  }
}
```

## Pre-publish Checklist

1. **Build verification**
   - `npm run build` succeeds
   - `dist/` contains expected files
   - `dist/bin/bdm.js` has correct shebang

2. **Package content**
   - `npm pack` and inspect tarball
   - Verify only dist/, README, LICENSE included
   - No source files or test files

3. **Local install test**
   ```bash
   npm pack
   npm install -g beads-messenger-0.1.0.tgz
   bdm --version
   bdm --help
   ```

4. **Clean install test**
   ```bash
   # In a fresh directory
   npm init -y
   npm install beads-messenger
   npx bdm --version
   ```

## Files to Create

### LICENSE
```
MIT License

Copyright (c) 2024 [Author]

Permission is hereby granted...
```

### .npmignore (if needed)
```
src/
tests/
tsconfig.json
tsup.config.ts
.github/
```

## Files
- package.json (update)
- LICENSE (create)
- .npmignore (if needed)

## Acceptance Criteria
- `npm pack` produces correct package
- Package installs globally
- `bdm` command works after install
- No unnecessary files in package


