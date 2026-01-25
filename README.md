# HSAR
### Human Signal Aware AI Runtime

HSAR is a production-grade control plane that sits between users and foundation models to improve last-mile AI reliability.

It observes lightweight human interaction signals in real time and governs model behavior using bounded, deterministic controls, without retraining models or increasing GPU usage.

---

## Why HSAR Exists

Modern AI failures are rarely caused by model incapability. They arise from unmanaged human–system feedback loops under real-world latency, cost, and safety constraints.

Prompt engineering, agent frameworks, and RLHF optimize model behavior in isolation. HSAR addresses a different problem: **governing AI behavior at runtime when human interaction begins to fail**.

---

## What HSAR Does

- Runs as a fail-open sidecar between users and models  
- Forecasts interaction failure risk using CPU-only signal inference  
- Applies deterministic, reversible controls to model behavior  
- Degrades gracefully under load or failure  
- Works across proprietary and open-weight models  

HSAR introduces reliability without becoming a new single point of failure.

---

## High-Level Architecture

<p align="center">
  <img src="HSAR Inference Pipeline.png" alt="HSAR Inference Pipeline" width="800">
</p>

HSAR intercepts requests, extracts human interaction signals asynchronously, evaluates policies, and conditionally mutates model requests before forwarding them to the backend.

If HSAR exceeds its latency budget or fails, requests pass through unmodified.

---

## Design Principles

- Fail-open by default  
- Strictly bounded latency and cost  
- Deterministic degradation  
- Monotonic safety guarantees  
- Model and vendor agnostic  

---

## Documentation

- [Architecture Overview](docs/architecture.md)
- [Policy Engine Design](docs/policy-engine.md) (planned)
- [Threat Model](docs/threat-model.md) (planned)

---

## Project Status

HSAR is currently in active design and prototyping.  
The architecture is stable; implementation details are evolving.

This repository prioritizes correctness, observability, and operational realism over rapid feature expansion.

---

## License

Apache 2.0
