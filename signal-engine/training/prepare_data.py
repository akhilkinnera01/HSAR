"""Curate labeled dialog turns for failure_risk training."""

from __future__ import annotations

import csv
import random
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DATA_DIR = ROOT / "training" / "data"
LABELED_CSV = DATA_DIR / "labeled_turns.csv"

FAILURE_TEMPLATES = [
    "THIS IS UNACCEPTABLE!!! I keep repeating myself and nobody listens!!!",
    "I hate this stupid broken service, escalate me to a human NOW",
    "Wrong again. Wrong again. Wrong again. Fix it immediately!!!",
    "I am so angry and frustrated, this is the worst experience ever",
    "Stop ignoring me!!! I already told you three times!!!",
    "Terrible awful useless response, I am abandoning this chat",
    "ESCALATE ESCALATE ESCALATE this is ridiculous!!!",
]

SUCCESS_TEMPLATES = [
    "Thanks for the update, that answers my question.",
    "Please help me understand my account balance for March.",
    "Could you walk me through the refund process step by step?",
    "That worked perfectly, appreciate your help today.",
    "I would like to change my shipping address on order 4421.",
    "Great, the new password reset flow makes sense to me now.",
    "No further action needed from my side, all good here.",
]

SYNTHETIC_FAILURE_VARIANTS = [
    "{base} Again: {base}",
    "!!! {base} !!!",
    "{base} I need a supervisor immediately.",
]

SYNTHETIC_SUCCESS_VARIANTS = [
    "Hi, {base}",
    "Quick question: {base}",
    "{base} Thank you.",
]


@dataclass
class LabeledTurn:
    turn_id: str
    text: str
    label_failure: int
    outcome_type: str
    source: str
    conversation_id: str


def _build_rows(seed: int = 42) -> list[LabeledTurn]:
    rng = random.Random(seed)
    rows: list[LabeledTurn] = []
    conv_idx = 0

    def add_batch(
        templates: list[str],
        label: int,
        outcome: str,
        source: str,
        count: int,
    ) -> None:
        nonlocal conv_idx
        for i in range(count):
            conv_idx += 1
            base = rng.choice(templates)
            if source == "synthetic" and label == 1:
                text = rng.choice(SYNTHETIC_FAILURE_VARIANTS).format(base=base)
            elif source == "synthetic":
                text = rng.choice(SYNTHETIC_SUCCESS_VARIANTS).format(base=base)
            else:
                text = base
            rows.append(
                LabeledTurn(
                    turn_id=f"{source}-{conv_idx}-{i}",
                    text=text,
                    label_failure=label,
                    outcome_type=outcome,
                    source=source,
                    conversation_id=f"conv-{conv_idx}",
                )
            )

    # Public-style templates (proxy for escalation-labeled support dialogs)
    add_batch(FAILURE_TEMPLATES, 1, "escalation", "public", 120)
    add_batch(SUCCESS_TEMPLATES, 0, "none", "public", 120)

    # Synthetic augmentation for hard cases
    add_batch(FAILURE_TEMPLATES, 1, "repeated_correction", "synthetic", 180)
    add_batch(SUCCESS_TEMPLATES, 0, "none", "synthetic", 180)

    rng.shuffle(rows)
    return rows


def write_dataset(path: Path = LABELED_CSV) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    rows = _build_rows()
    with path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(
            f,
            fieldnames=[
                "turn_id",
                "text",
                "label_failure",
                "outcome_type",
                "source",
                "conversation_id",
            ],
        )
        writer.writeheader()
        for row in rows:
            writer.writerow(row.__dict__)
    return path


def conversation_safe_split(
    rows: list[dict],
    test_fraction: float = 0.2,
    seed: int = 42,
) -> tuple[list[dict], list[dict]]:
    """Hold out whole conversations; synthetic rows never in test."""
    rng = random.Random(seed)
    public_rows = [r for r in rows if r["source"] == "public"]
    synthetic_rows = [r for r in rows if r["source"] == "synthetic"]

    conv_ids = sorted({r["conversation_id"] for r in public_rows})
    rng.shuffle(conv_ids)
    test_conv_count = max(1, int(len(conv_ids) * test_fraction))
    test_convs = set(conv_ids[:test_conv_count])

    test_rows = [r for r in public_rows if r["conversation_id"] in test_convs]
    train_rows = [r for r in public_rows if r["conversation_id"] not in test_convs]
    train_rows.extend(synthetic_rows)
    rng.shuffle(train_rows)
    return train_rows, test_rows


def load_dataset(path: Path = LABELED_CSV) -> list[dict]:
    if not path.exists():
        write_dataset(path)
    with path.open(encoding="utf-8") as f:
        return list(csv.DictReader(f))


if __name__ == "__main__":
    out = write_dataset()
    rows = load_dataset(out)
    train, test = conversation_safe_split(rows)
    print(f"wrote {len(rows)} rows to {out}")
    print(f"train={len(train)} test={len(test)} (synthetic excluded from test)")