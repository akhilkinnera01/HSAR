# HSAR Demo

One-command walkthrough of enforce-mode governance: calm passthrough, then high-risk applied action visible in `policy_trace`.

## Prerequisites

- Docker and Docker Compose
- `curl`
- ~3–5 minutes after images are built (`make up` once)

The demo **requires a healthy signal-engine**. If perception is down, `make demo` exits with a clear preflight error (fail-open is documented in [architecture.md](architecture.md) and [runbook.md](runbook.md), not demonstrated here).

## Run

```bash
make demo
```

Equivalent:

```bash
make up
./scripts/demo.sh
```

## Expected output

```
==> HSAR Demo (enforce mode)
==> Step 1/2: calm request ... OK
    policy_trace: ... demo-calm-1 ... PASSTHROUGH or low-risk action
==> Step 2/2: high-risk request ... OK
    policy_trace: ... demo-risk-1 ... INJECT_SYSTEM_CONTEXT (or stronger action)
demo: all checks passed
```

Duration: under 2 minutes when services are already built; first run may take longer for image build.

## Regenerate README visual (`docs/assets/demo.gif`)

1. Run `make demo` in a terminal with readable font size.
2. Record the session (macOS QuickTime, asciinema, or terminalizer).
3. Export a short GIF (≤60s) and save as `docs/assets/demo.gif`.
4. Optional: publish an asciinema cast and link it below the GIF in README.

```bash
# Example with asciinema (optional)
asciinema rec /tmp/hsar-demo.cast
make demo
asciinema upload /tmp/hsar-demo.cast
```

## Related docs

- [Architecture](architecture.md) — three planes and enforce path
- [Policy engine](policy-engine.md) — actions and rollout modes
- [Runbook](runbook.md) — moving from shadow to enforce in production