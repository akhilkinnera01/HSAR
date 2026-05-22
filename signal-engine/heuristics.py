from hsar.v1 import signal_frame_pb2

MIN_TEXT_LEN = 3


def analyze_text(text: str) -> signal_frame_pb2.SignalFrame:
    stripped = (text or "").strip()

    if not stripped:
        return _abstain_frame(signal_frame_pb2.ABSTAIN_EMPTY_INPUT)

    if len(stripped) < MIN_TEXT_LEN:
        return _abstain_frame(signal_frame_pb2.ABSTAIN_TOO_SHORT)

    caps_ratio = _caps_ratio(stripped)

    frustration = 0.2
    urgency = 0.2
    if "!" in stripped or caps_ratio > 0.6:
        urgency = 0.8
    if any(w in stripped.lower() for w in ["hate", "stupid", "angry", "terrible"]):
        frustration = 0.9

    return signal_frame_pb2.SignalFrame(
        abstain=False,
        confidence=0.70,
        tier=signal_frame_pb2.TIER_1,
        signals=[
            signal_frame_pb2.Signal(name="frustration", value=frustration),
            signal_frame_pb2.Signal(name="urgency", value=urgency),
        ],
    )


def _caps_ratio(text: str) -> float:
    letters = [c for c in text if c.isalpha()]
    if not letters:
        return 0.0
    return sum(1 for c in letters if c.isupper()) / len(letters)


def _abstain_frame(reason: signal_frame_pb2.AbstainReason) -> signal_frame_pb2.SignalFrame:
    return signal_frame_pb2.SignalFrame(
        abstain=True,
        abstain_reason=reason,
        confidence=0.0,
        tier=signal_frame_pb2.TIER_1,
        signals=[],
    )