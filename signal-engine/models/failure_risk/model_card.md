# Model Card: failure_risk v1.0.0

## Model details

| Field | Value |
|-------|-------|
| Name | `failure_risk` |
| Version | `failure_risk-1.0.0` |
| Type | Binary classifier (logistic regression + StandardScaler) |
| Runtime | ONNX via `onnxruntime` (CPU) |
| Feature schema | `fv-1` (8 interpretable lexical features) |

## Intended use

Shadow-mode forecasting of interaction failure risk from a single user text turn. Outputs calibrated probability in `SignalFrame.signals[].name == "failure_risk"`.

**Not intended for**: content moderation, toxicity-only scoring, or enforce-mode policy decisions without Phase 3+ policy engine review.

## Training data

- **Public-style templates**: escalation/frustration vs successful support utterances
- **Synthetic augmentation**: hard positive/negative variants tagged `source=synthetic`
- **Holdout**: 20% of public conversations (synthetic never in test)
- **Class balance**: ~50% positive in test split

See `training/prepare_data.py` for provenance and split rules.

## Metrics (held-out)

See [docs/benchmarks.md](../../../docs/benchmarks.md). Summary:

- Test PR-AUC: 1.0 (MVP template corpus)
- Test ECE: 0.016 (logistic regression, isotonic calibration)
- CPU p99 latency: < 1 ms sklearn / < 30 ms ONNX budget in pytest

## Ethical & operational constraints

- No raw user text in logs, metrics, or `SignalFrame.meta`
- Heuristic fallback when model unavailable (`meta.inference_source=heuristic`)
- Abstain on empty, too-short, or low-confidence inputs

## Limitations

- Template-trained MVP; real production would need proprietary escalation-labeled logs
- English lexical features only
- Calibration valid only for similar support-chat domain