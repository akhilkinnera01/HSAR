# Contributing to HSAR

Thank you for helping improve HSAR. Application code lives in this repository (`HSAR/`). Spec-kit planning artifacts live in the sibling `HSAR-workspace/` repo—do not commit specs here.

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.24.x | Proxy, policy engine |
| Python | 3.11+ (3.10 in Docker) | Signal engine |
| Docker + Compose | current | Local stack |
| [buf](https://buf.build/docs/installation) | current | Protobuf lint/generate |
| k6 | optional | `make bench-load`, `make bench-chaos` |

## Local setup

```bash
git clone <hsar-repo-url>
cd HSAR
make test    # proto gen + Go race tests + Python pytest
make smoke   # docker compose + auth/shadow/fail-open checks
```

## Common commands

```bash
make lint          # buf lint, go vet, ruff
make vuln          # govulncheck on Go modules
make up            # start compose stack
make demo          # enforce-mode governance walkthrough
make up-observability  # Prometheus + Grafana + OTel
make bench-load    # runtime SLO measurement (k6)
make bench-chaos   # fail-open under outage (k6)
```

## Quality gates (match CI)

CI runs on push/PR to `main`, `phase-*`, and `feat/*` branches (`.github/workflows/ci.yml`):

1. `buf` + protoc plugins install
2. `make lint test`
3. `govulncheck ./...`

Run the same locally before opening a PR.

## Repository boundaries

| Location | Contents |
|----------|----------|
| `HSAR/` (this repo) | Go proxy, Python signal engine, policies, docs, benchmarks, dashboards |
| `HSAR-workspace/` | Spec-kit `specs/`, plans, tasks—outside this git repo |

Code commits for features land here; planning artifacts stay in the workspace.

## Branch workflow

- Feature branches: `phase-N-short-name` or `feat/...`
- Phase work: ≥6 local commits per phase (project convention)
- Do not push unless explicitly requested

## Security & supply chain

See [docs/threat-model.md](docs/threat-model.md) for threat context. Required checks:

- `make vuln` (Go vulnerability scan)
- `buf lint` (schema correctness)
- Privacy allowlist tests in `internal/telemetry/*_test.go`

## Documentation

When changing behavior, update relevant docs:

- [docs/architecture.md](docs/architecture.md)
- [docs/policy-engine.md](docs/policy-engine.md)
- [docs/benchmarks.md](docs/benchmarks.md) (if SLOs change)
- [README.md](README.md) — Honest Claims: no capability without test/benchmark backing

## Questions

Open an issue or review [docs/runbook.md](docs/runbook.md) for operational procedures.