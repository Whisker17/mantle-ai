# CLAUDE.md - mantle-ai

## Overview

This repository contains Mantle-specific AI tooling:

- Go CLI (`cli/`)
- MCP server (`mcp-server/`)
- Claude Code plugins (`packages/plugins/`)

## Workspace conventions

- Use Nx workspace metadata where needed.
- Keep plugin packages self-contained and valid against `scripts/validate-plugin.cjs`.
- Preserve deterministic machine-readable outputs for agent-facing runtime components.
