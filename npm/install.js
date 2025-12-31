#!/usr/bin/env node
'use strict';

const fs = require('fs');
const https = require('https');
const os = require('os');
const path = require('path');
const tar = require('tar');

const pkg = require('../package.json');

const skipDownload = [
  '1',
  'true',
  'yes'
].includes(String(process.env.FRAY_SKIP_DOWNLOAD || '').toLowerCase());

if (skipDownload) {
  process.exit(0);
}

const goosMap = {
  darwin: 'darwin',
  linux: 'linux',
  win32: 'windows'
};

const goarchMap = {
  x64: 'amd64',
  arm64: 'arm64'
};

const goos = goosMap[process.platform];
const goarch = goarchMap[process.arch];

if (!goos || !goarch) {
  console.error(`fray: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}

const version = pkg.version;
const archiveName = `fray_${version}_${goos}_${goarch}.tar.gz`;
const downloadUrl = `https://github.com/adamavenir/fray/releases/download/v${version}/${archiveName}`;

const binDir = path.join(__dirname, 'bin');
const markerPath = path.join(binDir, '.version');
const expectedExt = process.platform === 'win32' ? '.exe' : '';
const expectedFray = path.join(binDir, `fray${expectedExt}`);
const expectedMcp = path.join(binDir, `fray-mcp${expectedExt}`);

try {
  if (fs.existsSync(markerPath)) {
    const existingVersion = fs.readFileSync(markerPath, 'utf8').trim();
    if (existingVersion === version && fs.existsSync(expectedFray)) {
      process.exit(0);
    }
  }
} catch (err) {
  // Ignore cache errors and continue to download.
}

fs.mkdirSync(binDir, { recursive: true });

const archivePath = path.join(os.tmpdir(), archiveName);

download(downloadUrl, archivePath)
  .then(() => extractArchive(archivePath, binDir))
  .then(() => ensureBinaries(binDir, expectedFray, expectedMcp))
  .then(() => {
    fs.writeFileSync(markerPath, `${version}\n`);
    safeUnlink(archivePath);
  })
  .catch((err) => {
    console.error(`fray: install failed: ${err.message}`);
    process.exit(1);
  });

function download(url, destination) {
  return new Promise((resolve, reject) => {
    https.get(url, (res) => {
      if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        res.resume();
        return resolve(download(res.headers.location, destination));
      }
      if (res.statusCode !== 200) {
        res.resume();
        return reject(new Error(`download failed (${res.statusCode})`));
      }
      const file = fs.createWriteStream(destination);
      res.pipe(file);
      file.on('finish', () => file.close(resolve));
      file.on('error', reject);
    }).on('error', reject);
  });
}

function extractArchive(archive, destination) {
  return tar.x({
    file: archive,
    cwd: destination,
    overwrite: true
  });
}

function ensureBinaries(destination, frayPath, mcpPath) {
  if (!fs.existsSync(frayPath)) {
    throw new Error('fray binary missing after extract');
  }
  if (!fs.existsSync(mcpPath)) {
    throw new Error('fray-mcp binary missing after extract');
  }
  if (process.platform !== 'win32') {
    fs.chmodSync(frayPath, 0o755);
    fs.chmodSync(mcpPath, 0o755);
  }
}

function safeUnlink(filePath) {
  try {
    if (fs.existsSync(filePath)) {
      fs.unlinkSync(filePath);
    }
  } catch (err) {
    // Ignore cleanup errors.
  }
}
