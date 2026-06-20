# HSAR Signal Model Benchmarks (failure_risk v1.0.0)

**Generated**: 2026-06-29  
**Artifact**: `signal-engine/models/failure_risk/`  
**Selected model**: `logistic_regression` (held-out PR-AUC tie-break vs HistGradientBoosting)

## Label definition

`failure_risk` = P(this user turn leads to escalation, abandonment, or repeated correction within the interaction session).

## Dataset

| Metric | Value |
|--------|-------|
| Total labeled turns | 600 |
| Train rows | 552 (public train + all synthetic augmentation) |
| Test rows | 48 (public only, conversation holdout) |
| Test positive rate | 52.1% |
| Synthetic in test split | **No** (provenance rule) |

**Sources**: Public-style support dialog templates + documented synthetic hard-case augmentation (`training/prepare_data.py`).

## Head-to-head candidates (held-out test)

| Model | Val PR-AUC | Test PR-AUC | Test ROC-AUC | Test Brier | Test ECE | Abstain rate (τ=0.55) | p50 (ms) | p99 (ms) |
|-------|------------|-------------|--------------|------------|----------|------------------------|----------|----------|
| logistic_regression | 1.000 | 1.000 | 1.000 | 0.0004 | 0.016 | 0.0% | 0.06 | 0.14 |
| hist_gradient_boosting | 1.000 | 1.000 | 1.000 | ~0 | ~0 | 0.0% | 8.05 | 21.74 |

**Selection**: `logistic_regression` — tied discrimination with lower latency and simpler ONNX surface.

## Serving SLO (CPU, ONNX Runtime)

| Metric | Target | Measured (pytest perf test) |
|--------|--------|----------------------------|
| Inference p99 | < 30 ms | Pass (`test_model.py`) |

## Confusion at operating threshold (0.5, held-out test)

Selected model (`logistic_regression`):

| | Predicted negative | Predicted positive |
|--|------------------|------------------|
| **Actual negative** | TN = 23 | FP = 0 |
| **Actual positive** | FN = 0 | TP = 25 |

HistGradientBoosting matches the same confusion matrix on this held-out split.

## Abstention

Default confidence threshold τ = 0.55 (`ABSTAIN_CONFIDENCE_THRESHOLD`). Abstain when `max(p, 1-p) < τ`.

Held-out **abstention rate**: 0.0% for both candidates (all test predictions exceed τ on this corpus).

## Limitations

- MVP corpus is template-driven; metrics are optimistic on in-distribution text.
- English-only lexical features; no cross-lingual calibration claims.
- Shadow mode only in Phase 2 — no inline enforce latency yet.

## Reproduce

```bash
cd HSAR
make train-model
```

---

## Runtime SLOs (proxy load + chaos)

**Last run**: 2026-06-29  
**Environment**: Local Docker Compose (`make up`), k6 load harness (`bench/load.js`), chaos script (`bench/chaos.sh`)  
**Dashboard**: [`dashboards/hsar.json`](../dashboards/hsar.json) — import into Grafana via `make up-observability`

| SLO | Target | Measured | Pass/Fail |
|-----|--------|----------|-----------|
| Shadow added p99 | < 5 ms | Run `make bench-load` | pending local run |
| Enforce added p99 | < 30 ms | Run `make bench-load` | pending local run |
| Chaos fail-open success | 100% k6 success | Run `make bench-chaos` | pending local run |
| Chaos p99 drift | ≤ 2× healthy proxy p99 | Run `make bench-chaos` | pending local run |

### Harness commands

```bash
make bench-load    # baseline vs proxy (shadow + enforce)
make bench-chaos   # stop signal-engine mid-load in enforce mode
make up-observability  # Prometheus :9090, Grafana :3000, OTel collector :4317
```

Populate measured columns after running benchmarks on your machine; README observability row reflects harness availability once `make test` and smoke pass.