#!/usr/bin/env python3
"""Update docs/benchmarks.md Runtime SLOs table from k6 summary exports."""

from __future__ import annotations

import argparse
import json
import re
from datetime import date
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BENCH_MD = ROOT / "docs" / "benchmarks.md"
RESULTS = ROOT / "bench" / "results"

HARNESS = """### Harness commands

```bash
make bench-load    # baseline vs proxy (shadow + enforce), updates this section
make bench-chaos   # stop signal-engine mid-load in enforce mode
make up-observability  # Prometheus :9090, Grafana :3000, OTel collector :4317
```
"""


def metric_values(summary: dict, metric: str) -> dict:
    m = summary.get("metrics", {}).get(metric, {})
    nested = m.get("values")
    if isinstance(nested, dict) and nested:
        return nested
    return m if isinstance(m, dict) else {}


def pctl(summary: dict, metric: str, p: str) -> float:
    values = metric_values(summary, metric)
    if p in values:
        return float(values[p])
    if p == "p(99)" and "p(95)" in values:
        return float(values["p(95)"])
    if p == "p(50)" and "med" in values:
        return float(values["med"])
    return float(values.get("avg", 0.0))


def failed_rate(summary: dict) -> float:
    values = metric_values(summary, "http_req_failed")
    if "rate" in values:
        return float(values["rate"])
    if "value" in values:
        return float(values["value"])
    fails = float(values.get("fails", 0.0))
    total = float(values.get("passes", 0.0)) + fails
    return fails / total if total else 0.0


def load_summary(name: str) -> dict:
    path = RESULTS / f"{name}.json"
    with path.open() as f:
        return json.load(f)


def pass_fail(added_p99_ms: float, target_ms: float) -> str:
    return "pass" if added_p99_ms <= target_ms else "fail"


def format_load_table() -> str:
    baseline = load_summary("baseline")
    shadow = load_summary("shadow")
    enforce = load_summary("enforce")

    base_p50 = pctl(baseline, "http_req_duration", "p(50)")
    base_p99 = pctl(baseline, "http_req_duration", "p(99)")
    shadow_p50 = pctl(shadow, "http_req_duration", "p(50)")
    shadow_p99 = pctl(shadow, "http_req_duration", "p(99)")
    enforce_p50 = pctl(enforce, "http_req_duration", "p(50)")
    enforce_p99 = pctl(enforce, "http_req_duration", "p(99)")

    shadow_added = shadow_p99 - base_p99
    enforce_added = enforce_p99 - base_p99

    return (
        "| SLO | Target | Measured | Pass/Fail |\n"
        "|-----|--------|----------|-----------|\n"
        f"| Shadow added p99 | < 5 ms | baseline p99={base_p99:.2f} ms, proxy p99={shadow_p99:.2f} ms, **added={shadow_added:.2f} ms** | **{pass_fail(shadow_added, 5.0)}** |\n"
        f"| Enforce added p99 | < 30 ms | baseline p99={base_p99:.2f} ms, proxy p99={enforce_p99:.2f} ms, **added={enforce_added:.2f} ms** | **{pass_fail(enforce_added, 30.0)}** |\n"
        f"| Shadow added p50 | — | added={shadow_p50 - base_p50:.2f} ms | — |\n"
        f"| Enforce added p50 | — | added={enforce_p50 - base_p50:.2f} ms | — |"
    )


def format_chaos_table() -> str:
    healthy = load_summary("chaos-healthy")
    chaos = load_summary("chaos-outage")

    healthy_p99 = pctl(healthy, "http_req_duration", "p(99)")
    outage_p99 = pctl(chaos, "http_req_duration", "p(99)")
    drift = outage_p99 / healthy_p99 if healthy_p99 > 0 else 0.0
    success = failed_rate(chaos) == 0.0

    return (
        "| SLO | Target | Measured | Pass/Fail |\n"
        "|-----|--------|----------|-----------|\n"
        f"| Chaos fail-open success | 100% k6 success | failed_rate={failed_rate(chaos):.4f} | **{'pass' if success else 'fail'}** |\n"
        f"| Chaos p99 drift | ≤ 2× healthy proxy p99 | healthy p99={healthy_p99:.2f} ms, outage p99={outage_p99:.2f} ms, ratio={drift:.2f}× | **{'pass' if drift <= 2.0 else 'fail'}** |"
    )


def runtime_header() -> str:
    today = date.today().isoformat()
    return (
        f"## Runtime SLOs (proxy load + chaos)\n\n"
        f"**Last run**: {today}\n"
        f"**Environment**: Local Docker Compose (`make up`), k6 (`bench/load.js`, `bench/chaos.sh`)\n"
        f"**Dashboard**: [`dashboards/hsar.json`](../dashboards/hsar.json) — `make up-observability`\n\n"
    )


def replace_subsection(text: str, title: str, body: str) -> str:
    block = f"### {title}\n\n{body.strip()}\n\n"
    pattern = rf"### {re.escape(title)}\n\n.*?(?=\n### |\Z)"
    if re.search(pattern, text, flags=re.DOTALL):
        return re.sub(pattern, block, text, count=1, flags=re.DOTALL)
    return text.rstrip() + "\n\n" + block


def extract_subsection(text: str, title: str) -> str | None:
    pattern = rf"### {re.escape(title)}\n\n(.*?)(?=\n### |\n---|\Z)"
    match = re.search(pattern, text, flags=re.DOTALL)
    return match.group(1).strip() if match else None


def patch_benchmarks(load: bool, chaos: bool) -> None:
    text = BENCH_MD.read_text()
    pattern = r"## Runtime SLOs \(proxy load \+ chaos\).*"
    existing = re.search(pattern, text, flags=re.DOTALL)
    prior = existing.group(0) if existing else ""

    load_body = format_load_table() if load else extract_subsection(prior, "Load SLOs")
    chaos_body = format_chaos_table() if chaos else extract_subsection(prior, "Chaos SLOs")

    section = runtime_header()
    if load_body:
        section += f"### Load SLOs\n\n{load_body}\n\n"
    if chaos_body:
        section += f"### Chaos SLOs\n\n{chaos_body}\n\n"
    section += HARNESS.strip() + "\n"

    if existing:
        text = re.sub(pattern, section.strip(), text, flags=re.DOTALL)
    else:
        text = text.rstrip() + "\n\n---\n\n" + section.strip() + "\n"

    BENCH_MD.write_text(text)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--load", action="store_true")
    parser.add_argument("--chaos", action="store_true")
    args = parser.parse_args()
    if not args.load and not args.chaos:
        parser.error("specify --load and/or --chaos")
    patch_benchmarks(load=args.load, chaos=args.chaos)
    print(f"updated {BENCH_MD}")


if __name__ == "__main__":
    main()