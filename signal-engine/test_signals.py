import os
import sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
sys.path.insert(0, os.path.join(ROOT, "gen", "python"))

from hsar.v1 import signal_frame_pb2  # noqa: E402
from heuristics import analyze_text  # noqa: E402


def _values(frame):
    return {s.name: s.value for s in frame.signals}


def test_signal_values_are_bounded():
    frame = analyze_text("I HATE THIS!!!")
    for name, value in _values(frame).items():
        assert 0.0 <= value <= 1.0, f"{name} out of range: {value}"


def test_caps_ratio_drives_urgency():
    calm = analyze_text("please help with my account")
    heated = analyze_text("THIS IS UNACCEPTABLE!!!")

    assert _values(calm)["urgency"] < _values(heated)["urgency"]


def test_keyword_drives_frustration():
    calm = analyze_text("thanks for the update")
    angry = analyze_text("this is stupid and terrible")

    assert _values(calm)["frustration"] < _values(angry)["frustration"]


def test_empty_input_abstains():
    frame = analyze_text("   ")

    assert frame.abstain is True
    assert frame.abstain_reason == signal_frame_pb2.ABSTAIN_EMPTY_INPUT


def test_short_input_abstains():
    frame = analyze_text("ok")

    assert frame.abstain is True
    assert frame.abstain_reason == signal_frame_pb2.ABSTAIN_TOO_SHORT