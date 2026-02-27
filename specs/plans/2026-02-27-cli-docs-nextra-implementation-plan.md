# CLI Docs Nextra Website Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Nextra documentation website in `docs/`, document the full CLI surface in English, and mark implemented vs planned items while migrating planning docs to `specs/plans`.

**Architecture:** Create a standalone Next.js + Nextra app under `docs/` using Pages Router and MDX files. Use CLI source files as authoritative references for command docs and add a dedicated roadmap page for planned items. Move existing planning docs out of `docs/plans` into `specs/plans` so `docs/` is purely site-oriented.

**Tech Stack:** Next.js, Nextra, Nextra Docs Theme, MDX, npm

---

### Task 1: Baseline RED check and plan-doc migration

**Files:**
- Create: `specs/plans/2026-02-27-cli-docs-nextra-design.md`
- Create: `specs/plans/2026-02-27-cli-docs-nextra-implementation-plan.md`
- Move: `docs/plans/2026-02-26-mantle-ai-full-scaffold-design.md` -> `specs/plans/2026-02-26-mantle-ai-full-scaffold-design.md`

**Step 1: Write the failing test**

```bash
npm --prefix docs run build
```

**Step 2: Run test to verify it fails**

Run: `npm --prefix docs run build`
Expected: FAIL because `docs/package.json` does not exist yet.

**Step 3: Write minimal implementation**

- Create `specs/plans/` and place design + implementation plan docs there.
- Move existing `docs/plans/*` into `specs/plans/*`.

**Step 4: Run test to verify it still fails for the right reason**

Run: `npm --prefix docs run build`
Expected: still FAIL until Nextra app scaffolding is added.

**Step 5: Commit**

```bash
git add specs/plans docs/plans
git commit -m "docs: move planning docs to specs and add docs-site plan"
```

### Task 2: Scaffold Nextra app in `docs/`

**Files:**
- Create: `docs/package.json`
- Create: `docs/next.config.mjs`
- Create: `docs/theme.config.tsx`
- Create: `docs/pages/_meta.js`
- Create: `docs/pages/index.mdx`
- Create: `docs/pages/cli/_meta.js`

**Step 1: Write the failing test**

```bash
npm --prefix docs run build
```

**Step 2: Run test to verify it fails**

Run: `npm --prefix docs run build`
Expected: FAIL due missing Next/Nextra app files.

**Step 3: Write minimal implementation**

- Add Nextra dependencies and scripts.
- Add Nextra Next config and theme config.
- Add initial home page and CLI section index.

**Step 4: Run test to verify it passes at app scaffold level**

Run: `npm --prefix docs install && npm --prefix docs run build`
Expected: PASS with generated Next build output.

**Step 5: Commit**

```bash
git add docs/package.json docs/next.config.mjs docs/theme.config.tsx docs/pages
git commit -m "docs: scaffold nextra site in docs"
```

### Task 3: Add CLI learning pages

**Files:**
- Create: `docs/pages/cli/getting-started.mdx`
- Create: `docs/pages/cli/configuration.mdx`
- Create: `docs/pages/cli/output-contract.mdx`

**Step 1: Write the failing test**

```bash
npm --prefix docs run build
```

**Step 2: Run test to verify it fails**

Run: `npm --prefix docs run build`
Expected: FAIL if links/pages referenced by nav are missing.

**Step 3: Write minimal implementation**

- Document install/build/run basics.
- Document config precedence and examples.
- Document JSON envelope, `--results-only`, `--select`, and exit codes.

**Step 4: Run test to verify it passes**

Run: `npm --prefix docs run build`
Expected: PASS with no missing page references.

**Step 5: Commit**

```bash
git add docs/pages/cli/getting-started.mdx docs/pages/cli/configuration.mdx docs/pages/cli/output-contract.mdx
git commit -m "docs: add cli learn pages"
```

### Task 4: Add full CLI command reference pages

**Files:**
- Create: `docs/pages/cli/reference/_meta.js`
- Create: `docs/pages/cli/reference/chain-balance-tx.mdx`
- Create: `docs/pages/cli/reference/contract-token.mdx`
- Create: `docs/pages/cli/reference/transfer-swap.mdx`
- Create: `docs/pages/cli/reference/lend-stake-yield-bridge.mdx`
- Create: `docs/pages/cli/reference/providers-schema-version.mdx`

**Step 1: Write the failing test**

```bash
npm --prefix docs run build
```

**Step 2: Run test to verify it fails**

Run: `npm --prefix docs run build`
Expected: FAIL while nav references pages not yet created.

**Step 3: Write minimal implementation**

- Add all implemented CLI command docs with syntax, flags, examples, output notes.
- For each command group include `Status: Implemented` and source references.

**Step 4: Run test to verify it passes**

Run: `npm --prefix docs run build`
Expected: PASS and static pages generated.

**Step 5: Commit**

```bash
git add docs/pages/cli/reference
git commit -m "docs: add full cli command reference"
```

### Task 5: Add roadmap/changelog and root pointers

**Files:**
- Create: `docs/pages/cli/roadmap.mdx`
- Create: `docs/pages/cli/changelog.mdx`
- Modify: `README.md`

**Step 1: Write the failing test**

```bash
npm --prefix docs run build
```

**Step 2: Run test to verify it fails**

Run: `npm --prefix docs run build`
Expected: FAIL when nav includes missing roadmap/changelog pages.

**Step 3: Write minimal implementation**

- Add roadmap page with explicit `Planned` labels.
- Add changelog page starter.
- Update root README with docs and specs paths + run commands.

**Step 4: Run test to verify it passes**

Run: `npm --prefix docs run build`
Expected: PASS.

**Step 5: Commit**

```bash
git add docs/pages/cli/roadmap.mdx docs/pages/cli/changelog.mdx README.md
git commit -m "docs: add cli roadmap labels and project docs pointers"
```

### Task 6: Final verification

**Files:**
- Verify only (no new files required)

**Step 1: Write the failing test**

```bash
./cli/mantle schema --results-only | jq '.path'
```

**Step 2: Run test to verify it fails if CLI unavailable**

Run: `./cli/mantle schema --results-only | jq '.path'`
Expected: If CLI binary missing, FAIL and rebuild CLI.

**Step 3: Write minimal implementation**

- Ensure the CLI binary exists and command schema can be queried for documentation validation.

**Step 4: Run full verification**

Run:

```bash
./cli/mantle schema --results-only > /tmp/mantle-schema.json
npm --prefix docs run build
```

Expected: both commands PASS.

**Step 5: Commit**

```bash
git add -A
git commit -m "docs: complete cli docs site on nextra"
```
