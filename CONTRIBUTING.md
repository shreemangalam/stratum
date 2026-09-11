# Contributing to Stratum

Stratum is a syntax-aware source-code diff engine with a Go API and a Next.js
frontend. Correctness, reproducibility, and honest claims are part of the
product.

## Before changing code

- Read `docs/functional-design.md`, `docs/technical-design.md`, the latest ADR,
  and the relevant tests before changing behavior.
- Preserve unrelated work and keep public documentation aligned with behavior
  covered by tests.
- Never commit files under `docs/private/`; a pre-commit hook and CI check
  enforce this boundary.

## Architecture boundaries

- Keep `server/internal/core` deterministic and free of database, HTTP,
  filesystem, and parser dependencies.
- Convert parser-specific syntax into the shared core tree before matching.
- Put persistence behind store interfaces and HTTP concerns in the HTTP
  package.
- Treat `contract/openapi.yaml` as the wire-format source of truth. Regenerate
  Go and TypeScript models with `make gen`; do not hand-edit generated files.
- Write an ADR before introducing a new architectural dependency or changing a
  core data model.

## Product rules

- Describe semantic verdicts as conservative evidence, never as proof of
  behavioral equivalence.
- Call the current corpus metric match coverage, not accuracy, unless labeled
  ground truth is introduced.
- Keep local Git comparison disabled on public deployments.
- Keep computation bounded and surface approximate regions to users.

## Verification

Run focused tests while iterating and the complete release checks before a
pull request:

```bash
cd server
go test ./...
go vet ./...

cd ../web
npm run typecheck
npm run lint
npm run build
npm run test:e2e

cd ..
docker compose --profile full build
```

Parser and matching changes should include source-level tests that parse real
code, not only hand-constructed trees. Browser tests must control network
conditions explicitly rather than depending on a developer's local services.

See `docs/testing.md` for the full verification matrix and current evidence.

## Style

- Use conventional commit messages.
- Keep Go code formatted with `gofumpt` and frontend code formatted with Biome.
- Preserve the established accessible, responsive interface and visible focus
  states.
- Report skipped checks and remaining risks plainly.
