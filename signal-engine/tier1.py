"""Tier 1 heuristic feature extraction — cheap text signals within 2ms budget."""

import json

_NEGATIVE_KEYWORDS = frozenset(
    ["stop", "help", "wrong", "broken", "bad", "terrible", "hate", "useless", "fail"]
)


def _clamp(v: float) -> float:
    if v < 0.0:
        return 0.0
    if v > 1.0:
        return 1.0
    return v


def _abstained(reason: str) -> dict:
    return {
        "signals": [],
        "confidence": 0.0,
        "abstain": True,
        "abstain_reason": reason,
        "meta": {},
    }


def extract_tier1(text_payload: str) -> dict:
    """Compute Tier 1 text heuristics from an OpenAI-format JSON body.

    Returns dict with keys: signals, confidence, abstain, abstain_reason, meta.
    """
    # --- Abstain: empty / whitespace payload ---
    if not text_payload or not text_payload.strip():
        return _abstained("ABSTAIN_EMPTY_INPUT")

    # --- Abstain: JSON parse failure ---
    try:
        body = json.loads(text_payload)
    except (json.JSONDecodeError, TypeError):
        return _abstained("ABSTAIN_ERROR")

    # --- Extract last user message ---
    messages = body.get("messages") if isinstance(body, dict) else None
    if not messages or not isinstance(messages, list):
        return _abstained("ABSTAIN_EMPTY_INPUT")

    text = None
    for msg in reversed(messages):
        if isinstance(msg, dict) and msg.get("role") == "user":
            content = msg.get("content")
            if isinstance(content, str):
                text = content
            break

    if text is None:
        return _abstained("ABSTAIN_EMPTY_INPUT")

    # --- Abstain: too short ---
    if len(text) < 2:
        return _abstained("ABSTAIN_TOO_SHORT")

    # --- Feature extraction (pure string ops) ---
    text_len = len(text)
    brevity = 1.0 - min(text_len / 100.0, 1.0)

    alpha_count = 0
    upper_count = 0
    for ch in text:
        if ch.isalpha():
            alpha_count += 1
            if ch.isupper():
                upper_count += 1
    caps_ratio = upper_count / alpha_count if alpha_count > 0 else 0.0

    question_intensity = min(text.count("?") / 3.0, 1.0)
    exclamation_intensity = min(text.count("!") / 3.0, 1.0)

    words = text.lower().split()
    neg_count = 0
    for w in words:
        if w in _NEGATIVE_KEYWORDS:
            neg_count += 1
    negative_kw = min(neg_count / 2.0, 1.0)

    # --- Signal computation ---
    frustration_risk = _clamp(
        0.30 * brevity
        + 0.25 * caps_ratio
        + 0.15 * question_intensity
        + 0.15 * exclamation_intensity
        + 0.15 * negative_kw
    )

    failure_risk = _clamp(
        0.40 * negative_kw
        + 0.30 * brevity
        + 0.20 * caps_ratio
        + 0.10 * question_intensity
    )

    # --- Confidence: scales with signal strength ---
    confidence = 0.5 + 0.5 * max(frustration_risk, failure_risk)

    return {
        "signals": [
            {"name": "frustration_risk", "value": frustration_risk},
            {"name": "failure_risk", "value": failure_risk},
        ],
        "confidence": confidence,
        "abstain": False,
        "abstain_reason": "ABSTAIN_REASON_UNSPECIFIED",
        "meta": {
            "brevity": f"{brevity:.3f}",
            "caps_ratio": f"{caps_ratio:.3f}",
            "question_intensity": f"{question_intensity:.3f}",
            "exclamation_intensity": f"{exclamation_intensity:.3f}",
            "negative_kw": f"{negative_kw:.3f}",
        },
    }
