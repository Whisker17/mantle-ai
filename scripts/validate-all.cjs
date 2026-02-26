#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');

const root = process.cwd();
const marketplacePath = path.join(root, '.claude-plugin', 'marketplace.json');

if (!fs.existsSync(marketplacePath)) {
  console.error(`missing marketplace file: ${marketplacePath}`);
  process.exit(1);
}

const marketplace = JSON.parse(fs.readFileSync(marketplacePath, 'utf8'));
const plugins = marketplace.plugins || [];

let failed = false;
for (const plugin of plugins) {
  const pluginPath = path.resolve(root, plugin.source);
  const result = spawnSync('node', ['scripts/validate-plugin.cjs', pluginPath], {
    cwd: root,
    stdio: 'inherit'
  });
  if (result.status !== 0) {
    failed = true;
  }
}

if (failed) {
  process.exit(1);
}
console.log('All plugins validated.');
