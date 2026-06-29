"""Interpretable feature extraction for failure_risk model."""

from __future__ import annotations

import re
from typing import Sequence

import numpy as np

MAX_TEXT_LEN = 512
FEATURE_NAMES: Sequence[str] = (
    "caps_ratio",
    "exclamation_density",
    "question_density",
    "avg_word_len",
    "hedging_score",
    "negative_lexicon_score",
    "repetition_score",
    "turn_length_norm",
)

HEDGE_TOKENS = frozenset(
    {"maybe", "perhaps", "unsure", "not", "sure", "kind", "of", "sort", "possibly"}
)
NEGATIVE_TOKENS = frozenset(
    {
        "hate",
        "stupid",
        "angry",
        "terrible",
        "awful",
        "useless",
        "broken",
        "frustrated",
        "worst",
        "ridiculous",
    }
)


def truncate_text(text: str) -> str:
    return (text or "")[:MAX_TEXT_LEN]


def extract_features(text: str) -> np.ndarray:
    """Return feature vector of shape (8,) in FEATURE_NAMES order."""
    stripped = truncate_text(text).strip()
    if not stripped:
        return np.zeros(len(FEATURE_NAMES), dtype=np.float32)

    length = len(stripped)
    words = re.findall(r"[A-Za-z']+", stripped)
    word_count = max(len(words), 1)

    letters = [c for c in stripped if c.isalpha()]
    caps_ratio = (
        sum(1 for c in letters if c.isupper()) / len(letters) if letters else 0.0
    )

    exclamation_density = min(stripped.count("!") / length, 1.0)
    question_density = min(stripped.count("?") / length, 1.0)
    avg_word_len = min(sum(len(w) for w in words) / word_count / 20.0, 1.0)

    lower_words = [w.lower() for w in words]
    hedging_score = min(
        sum(1 for w in lower_words if w in HEDGE_TOKENS) / word_count, 1.0
    )
    negative_lexicon_score = min(
        sum(1 for w in lower_words if w in NEGATIVE_TOKENS) / word_count, 1.0
    )

    if len(lower_words) > 1:
        unique_ratio = len(set(lower_words)) / len(lower_words)
        repetition_score = max(0.0, 1.0 - unique_ratio)
    else:
        repetition_score = 0.0

    turn_length_norm = min(length / MAX_TEXT_LEN, 1.0)

    return np.array(
        [
            caps_ratio,
            exclamation_density,
            question_density,
            avg_word_len,
            hedging_score,
            negative_lexicon_score,
            repetition_score,
            turn_length_norm,
        ],
        dtype=np.float32,
    )