# Verification and Evidence

Stratum treats reproducibility as part of the product. The commands below are
also encoded in CI so the evidence can be regenerated from a clean checkout.

## Test layers

| Layer | What it demonstrates | Command |
|---|---|---|
| Core unit and property tests | Tree identity, matching, edit scripts, budgets, semantic classification, cache, jobs | `cd server && go test ./...` |
| Fuzz targets | Parser and matcher invariants under malformed and unexpected input | `cd server && go test -fuzz <target>` |
| Database integration | Migrations, persistence, idempotency, recovery, cleanup | included in `go test ./...` with Docker available |
| HTTP integration | Request validation, async workers, polling/SSE, CORS, and semantic results through Postgres | included in `go test ./...` with Docker available |
| Golden corpus | Expected operations for 21 before/after pairs across eight languages | `cd server && STRATUM_EVAL=1 go test ./internal/parse -run TestEvalCorpus -v` |
| Browser smoke | Forms, health errors/retry, navigation, and help content with controlled network responses | `cd web && npm run test:e2e` |
| Full-stack browser | Production frontend image through API, worker, Postgres, and rendered semantic verdict | CI `Full-stack browser test` job |
| Static/build checks | Go vet, Go lint, TypeScript, Biome, generated-contract drift, Docker images | CI |
| Supply-chain checks | npm advisories, secret scanning, private-file leak prevention | `cd web && npm audit` and CI |

The repository currently contains 191 Go test entry points, 10 fuzz targets,
15 Go benchmarks, 21 golden corpus cases, and 25 Playwright browser tests.

## Latest local release verification

Verified on 2026-09-12 using Windows/amd64, Go 1.25, Node 24, Docker Desktop,
Postgres 16, and installed Chrome:

- `go test -count=1 ./...`: 191 tests passed, including Docker-backed HTTP and store suites.
- `golangci-lint run` (v2.13.2): 0 issues.
- `go vet ./...`: passed.
- `npm run typecheck`, `npm run lint`, and `npm run build`: passed.
- Playwright smoke suite: 24 passed, one full-stack test intentionally gated.
- Playwright against an isolated production Docker stack: 25 passed.
- API and frontend production Docker images: built successfully.
- `npm audit`: zero known production or development dependency vulnerabilities.
- Generated Go and TypeScript models: regenerated from `contract/openapi.yaml`.

The full-stack case submits two Go functions whose dependent statements were
reordered and asserts that the result page renders `behavior-changing` with
the reason `dependent statements reordered`. This exercises the browser,
production frontend bundle, HTTP API, asynchronous worker, database, parser,
matcher, semantic classifier, SSE/result retrieval, and final UI.

## Evaluation results

The current golden corpus contains 21 cases across C, C++, Go, Java,
JavaScript, Python, TypeScript, and XSLT. Its latest average match coverage is
92.1% over 391 total nodes.

Match coverage means matched node pairs divided by the total nodes across both
inputs. It is useful for regression tracking, but it is not an accuracy score:
the corpus does not yet contain independently labeled ground truth for every
possible correspondence. The planned public benchmark will report precision
and recall against a labeled real-world dataset.

## Representative benchmarks

Three-run samples on an AMD Ryzen 7 7435HS (Windows/amd64):

| Benchmark | Median time | Memory | Allocations |
|---|---:|---:|---:|
| Match and edit script, about 1,500 nodes | 5.34 ms/op | 3.60 MB/op | 47,371/op |
| Parse, match, and edit script, 100 Go functions | 18.50 ms/op | 10.99 MB/op | 176,637/op |

These are development-machine measurements, not service-level guarantees.
Run the benchmark commands on the target deployment hardware before setting a
latency objective:

```bash
cd server
go test ./internal/core -run '^$' -bench '^BenchmarkMatch_1500nodes$' -benchmem -count=3
go test ./internal/parse/treesitter -run '^$' -bench '^BenchmarkPipeline_100funcs$' -benchmem -count=3
```

## Known evidence gaps

- Match quality has regression fixtures but not yet a large independently
  labeled real-world dataset.
- Fuzz targets exist, but long-duration fuzzing is not yet scheduled in CI.
- Browser CI covers Chromium; Firefox, WebKit, and mobile viewports are not yet
  release gates.
- Load, soak, and failure-injection tests are still required before claiming
  production scale or availability guarantees.
