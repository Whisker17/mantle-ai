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

## Deploy Docs to GitHub Pages

The repository includes a Pages workflow at `.github/workflows/deploy-docs.yml`.

- Trigger: push to `main` when files under `docs/**` change
- Publish target: `https://<username>.github.io/<repo>/`

One-time setup in GitHub repository settings:

1. Open `Settings -> Pages`.
2. Set `Source` to `GitHub Actions`.
