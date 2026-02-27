# mantle-ai

Mantle-focused AI tooling monorepo.

## Packages

- `cli/` - Go CLI (`mantle`)
- `mcp-server/` - TypeScript MCP server (Phase 3 implementation)
- `packages/plugins/*` - Claude Code plugin packages
- `docs/` - Nextra documentation website
- `specs/plans/` - design and implementation planning documents

## Validate

```bash
npm install
node scripts/validate-all.sh
```

## Documentation Site

```bash
npm --prefix docs install
npm run docs:dev
```

Build docs:

```bash
npm run docs:build
```
