#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

const REQUIRED = ['.claude-plugin/plugin.json', 'package.json', 'project.json', 'README.md'];

function fail(msg) {
  console.error(`ERROR: ${msg}`);
  process.exitCode = 1;
}

function readJson(p) {
  return JSON.parse(fs.readFileSync(p, 'utf8'));
}

function validate(pluginPath) {
  if (!fs.existsSync(pluginPath)) {
    fail(`plugin path does not exist: ${pluginPath}`);
    return;
  }

  for (const rel of REQUIRED) {
    const p = path.join(pluginPath, rel);
    if (!fs.existsSync(p)) {
      fail(`missing required file ${rel} in ${pluginPath}`);
    }
  }

  const pluginJsonPath = path.join(pluginPath, '.claude-plugin/plugin.json');
  const pkgJsonPath = path.join(pluginPath, 'package.json');
  const projectJsonPath = path.join(pluginPath, 'project.json');

  try {
    const pluginJson = readJson(pluginJsonPath);
    for (const field of ['name', 'version', 'description']) {
      if (!pluginJson[field]) {
        fail(`${pluginJsonPath} missing field: ${field}`);
      }
    }
  } catch (err) {
    fail(`invalid JSON in ${pluginJsonPath}: ${err.message}`);
  }

  try {
    const pkg = readJson(pkgJsonPath);
    if (!pkg.name) fail(`${pkgJsonPath} missing field: name`);
    if (!pkg.version) fail(`${pkgJsonPath} missing field: version`);
  } catch (err) {
    fail(`invalid JSON in ${pkgJsonPath}: ${err.message}`);
  }

  try {
    const project = readJson(projectJsonPath);
    if (!project.name) fail(`${projectJsonPath} missing field: name`);
    if (!Array.isArray(project.tags) || !project.tags.includes('type:plugin')) {
      fail(`${projectJsonPath} must include tag type:plugin`);
    }
  } catch (err) {
    fail(`invalid JSON in ${projectJsonPath}: ${err.message}`);
  }

  if (process.exitCode !== 1) {
    console.log(`OK: ${pluginPath}`);
  }
}

const pluginPath = process.argv[2];
if (!pluginPath) {
  console.error('Usage: node scripts/validate-plugin.cjs <plugin-path>');
  process.exit(1);
}

validate(pluginPath);
if (process.exitCode) process.exit(process.exitCode);
