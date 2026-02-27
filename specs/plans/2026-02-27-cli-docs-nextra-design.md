# CLI Documentation Website (Nextra) Design

Date: 2026-02-27
Owner: CLI docs initiative

## Goal

Create a documentation website for this monorepo using Nextra, implemented under `docs/`, and fully document the CLI surface in English. Include roadmap content while clearly labeling what is implemented vs planned.

## Constraints

- Nextra site implementation lives in `docs/`.
- Existing `docs/plans` content is migrated to `specs/plans`.
- CLI documentation is English-first.
- Roadmap items are included and explicitly labeled as `Planned`.

## Architecture

### Site Location

- `docs/` becomes a standalone Next.js + Nextra app.
- It is intentionally separated from root npm workspaces to reduce coupling.

### Content Structure

- `docs/pages/index.mdx`
- `docs/pages/cli/index.mdx`
- `docs/pages/cli/getting-started.mdx`
- `docs/pages/cli/configuration.mdx`
- `docs/pages/cli/output-contract.mdx`
- `docs/pages/cli/reference/*.mdx`
- `docs/pages/cli/roadmap.mdx`
- `docs/pages/cli/changelog.mdx`

### Status Model

Each command/reference page starts with a status block:

- `Status: Implemented` when present in current Cobra command tree.
- `Status: Planned` when present only in specs/roadmap.

Each status block includes:

- `Verified against:` one or more source-of-truth files.

## Source-of-Truth Hierarchy

1. `cli/internal/app/runner.go` and `cli/internal/app/runner_phase2.go` (runtime command definitions)
2. `cli/README.md` (examples and operator-facing usage)
3. `specs/plans/*.md` (roadmap/planned shape)

## Information Architecture

### Learn

- Getting Started
- Configuration
- Output Contract

### Reference

- Core: `chain`, `balance`, `tx`, `contract`, `token`
- DeFi and advanced: `transfer`, `swap`, `lend`, `stake`, `yield`, `bridge`
- Utility/admin: `providers`, `schema`, `version`

### Roadmap

- CLI roadmap page listing planned capabilities and noting they are not implemented.

## Migration Plan

1. Create `specs/plans/`.
2. Move existing `docs/plans/*` to `specs/plans/*`.
3. Scaffold Nextra in `docs/`.
4. Author CLI docs pages and status annotations.
5. Verify site builds successfully.
6. Update root README with docs/specs pointers.

## Risks and Mitigations

- Risk: Documentation drift from runtime behavior.
  - Mitigation: command pages cite exact source files and path names.
- Risk: Future CLI changes not reflected.
  - Mitigation: include a changelog page and verification guidance using `mantle schema`.

## Acceptance Criteria

- `docs/` runs as a Nextra site (`npm run dev`, `npm run build`).
- CLI commands are fully documented in English.
- Each command page marks implemented/planned status.
- `docs/plans` is migrated to `specs/plans`.
- Root README includes docs and specs locations.
