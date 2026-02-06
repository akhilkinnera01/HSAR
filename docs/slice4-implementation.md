# Slice 4: Tier 1 Heuristics — Implementation Walkthrough

## Goal

Replace the hardcoded stub signals in `signal-engine/main.py` with real Tier 1 heuristic logic from `signal-engine/tier1.py`.

---

## Starting State

- `signal-engine/main.py` had a `ProcessSignal` gRPC handler returning hardcoded values:
  - `frustration_risk = 0.12`
  - `failure_risk = 0.05`
  - `confidence = 0.90`
  - `abstain = False`
- `signal-engine/tier1.py` existed as an untracked file with real heuristic logic but was never imported or called.

---

## Step 1: Understand `tier1.py`

`tier1.extract_tier1(text_payload: str) -> dict` does the following:

1. **Abstain checks** — returns early with `abstain=True` for:
   - Empty or whitespace-only input (`ABSTAIN_EMPTY_INPUT`)
   - JSON parse failures (`ABSTAIN_ERROR`)
   - Missing `messages` array or no user message (`ABSTAIN_EMPTY_INPUT`)
   - Text shorter than 2 characters (`ABSTAIN_TOO_SHORT`)

2. **Feature extraction** — pure string operations on the last user message:
   - `brevity` — inverse of text length (shorter = higher)
   - `caps_ratio` — fraction of uppercase letters
   - `question_intensity` — count of `?` normalized to [0, 1]
   - `exclamation_intensity` — count of `!` normalized to [0, 1]
   - `negative_kw` — count of negative keywords (stop, help, wrong, broken, bad, terrible, hate, useless, fail)

3. **Signal computation** — weighted sums of features:
   - `frustration_risk = 0.30*brevity + 0.25*caps + 0.15*question + 0.15*exclamation + 0.15*negative`
   - `failure_risk = 0.40*negative + 0.30*brevity + 0.20*caps + 0.10*question`

4. **Return format** — a plain Python dict:
   ```python
   {
       "signals": [{"name": "frustration_risk", "value": float}, ...],
       "confidence": float,
       "abstain": bool,
       "abstain_reason": str,  # e.g. "ABSTAIN_REASON_UNSPECIFIED"
       "meta": {"brevity": "0.450", ...}
   }
   ```

---

## Step 2: Wire `tier1.py` into `main.py`

### Import

Added at the top of `main.py`:

```python
from tier1 import extract_tier1
```

### Replace stub logic in `ProcessSignal`

Before (stub):
```python
def ProcessSignal(self, request, context):
    start = now_ms()
    signals = [
        signal_frame_pb2.Signal(name="frustration_risk", value=0.12),
        signal_frame_pb2.Signal(name="failure_risk", value=0.05),
    ]
    end = now_ms()
    return signal_frame_pb2.SignalFrame(
        ...
        signals=signals,
        confidence=0.90,
        abstain=False,
        abstain_reason=signal_frame_pb2.ABSTAIN_REASON_UNSPECIFIED,
        meta={"engine": "stub_v1"},
    )
```

After (real heuristics):
```python
def ProcessSignal(self, request, context):
    start = now_ms()

    result = extract_tier1(request.text_payload)

    end = now_ms()

    signals = [
        signal_frame_pb2.Signal(name=s["name"], value=s["value"])
        for s in result["signals"]
    ]

    abstain_reason = signal_frame_pb2.AbstainReason.Value(
        result["abstain_reason"]
    )

    return signal_frame_pb2.SignalFrame(
        ...
        signals=signals,
        confidence=result["confidence"],
        abstain=result["abstain"],
        abstain_reason=abstain_reason,
        meta=result["meta"],
    )
```

### Key mapping decisions

| `tier1.py` dict field | Protobuf field | Conversion |
|---|---|---|
| `result["signals"]` | `signals` | List comprehension building `Signal(name=..., value=...)` |
| `result["confidence"]` | `confidence` | Direct float passthrough |
| `result["abstain"]` | `abstain` | Direct bool passthrough |
| `result["abstain_reason"]` | `abstain_reason` | `AbstainReason.Value(str)` converts string enum name to int |
| `result["meta"]` | `meta` | Direct dict passthrough (already `dict[str, str]`) |

---

## Step 3: Fix the PYTHONPATH in the Dockerfile

The `tier1` module lives at `/app/signal-engine/tier1.py` inside the container, but `PYTHONPATH` only included `/app/gen/python` (for protobuf bindings). Python couldn't resolve `from tier1 import extract_tier1`.

**Fix in `deploy/Dockerfile.signal`:**

```dockerfile
# Before
ENV PYTHONPATH=/app/gen/python

# After
ENV PYTHONPATH=/app/gen/python:/app/signal-engine
```

---

## Files Changed

| File | Change |
|---|---|
| `signal-engine/main.py` | Replaced stub with `extract_tier1()` call and dict-to-protobuf mapping |
| `deploy/Dockerfile.signal` | Added `/app/signal-engine` to `PYTHONPATH` |

No changes were needed to `tier1.py`, the proxy, protobuf definitions, or `docker-compose.yml`.
