#!/usr/bin/env node
'use strict';

const { spawnSync } = require('child_process');
const path = require('path');

const binName = process.platform === 'win32' ? 'fray.exe' : 'fray';
const binPath = path.join(__dirname, binName);

const result = spawnSync(binPath, process.argv.slice(2), {
  stdio: 'inherit'
});

if (result.error) {
  console.error(`fray: failed to run ${binName}: ${result.error.message}`);
  process.exit(1);
}

process.exit(result.status === null ? 1 : result.status);
