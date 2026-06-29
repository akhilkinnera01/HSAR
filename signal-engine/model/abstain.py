"""Pre- and post-model abstention rules."""

from __future__ import annotations

import os

from hsar.v1 import signal_frame_pb2

MIN_TEXT_LEN = 3
DEFAULT_CONFIDENCE_THRESHOLD = 0.55


def confidence_threshold() -> float:
    raw = os.getenv("ABSTAIN_CONFIDENCE_THRESHOLD", str(DEFAULT_CONFIDENCE_THRESHOLD))
    try:
        return float(raw)
    except ValueError:
        return DEFAULT_CONFIDENCE_THRESHOLD


def pre_model_abstain(text: str) -> signal_frame_pb2.AbstainReason | None:
    stripped = (text or "").strip()
    if not stripped:
        return signal_frame_pb2.ABSTAIN_EMPTY_INPUT
    if len(stripped) < MIN_TEXT_LEN:
        return signal_frame_pb2.ABSTAIN_TOO_SHORT
    return None


def post_model_abstain(calibrated_prob: float, tau: float | None = None) -> bool:
    """Abstain when model is uncertain (prob near 0.5)."""
    threshold = tau if tau is not None else confidence_threshold()
    certainty = max(calibrated_prob, 1.0 - calibrated_prob)
    return certainty < threshold


def abstain_frame(reason: signal_frame_pb2.AbstainReason) -> signal_frame_pb2.SignalFrame:
    return signal_frame_pb2.SignalFrame(
        abstain=True,
        abstain_reason=reason,
        confidence=0.0,
        tier=signal_frame_pb2.TIER_1,
        signals=[],
    )