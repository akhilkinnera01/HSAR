# HSAR: Human Signal Aware AI Runtime  
### A Human-Aware Control Plane for Foundation Models

## Abstract

Modern AI systems fail less often due to insufficient model capability and more often due to unmanaged human–system feedback loops. While foundation models continue to improve in accuracy and scale, production reliability degrades when systems cannot adapt to human interaction under real-world latency, cost, privacy, and safety constraints.

Human Signal Aware AI Runtime (HSAR) defines a model-agnostic control plane that operates as a production-grade sidecar between users and foundation models, including LLMs, TTS systems, and multimodal agents. The system observes lightweight human interaction signals in real time, forecasts interaction failure risk, and dynamically governs model behavior using bounded, deterministic controls.

HSAR improves last-mile AI reliability without retraining models, introducing new single points of failure, or increasing GPU utilization.

---

## 1. Core System Invariants

**Fail-Open by Design**  
HSAR must never block or degrade core model availability. If HSAR fails or exceeds its latency budget, requests pass through unmodified.

**Deterministic Degradation**  
On timeout or error, HSAR deterministically degrades to the next lower capability tier (Tier 2 → Tier 1 → passthrough) and emits an internal event.

**Strictly Bounded Overhead**  
All signal extraction, policy evaluation, and request mutation execute within predefined latency and cost envelopes using CPU-bound inference.

**Monotonic Safety**  
HSAR interventions may reduce response richness or verbosity but must never increase operational, compliance, or user risk.

**Privacy and Ephemerality**  
Raw user inputs, including audio and text, are processed in ephemeral memory buffers and discarded immediately after signal extraction. Only anonymized, derived signal vectors may persist, with explicit retention limits and encryption at rest.

**Model Churn Tolerance**  
Foundation models are treated as replaceable backends. HSAR remains stable across model upgrades, vendor changes, and API evolution.

**Multi-Tenant Isolation**  
Policies, metrics, and derived signals are strictly tenant-scoped with hard isolation and least-privilege access.

---

## 2. High-Level Architecture
![HSAR Inference Pipeline](HSAR%20Inference%20Pipeline.png)

HSAR is deployed as a sidecar proxy that intercepts user requests before they reach the foundation model backend. The proxy performs non-blocking signal extraction, evaluates policies, and conditionally mutates requests based on forecasted interaction risk.

If any component of HSAR fails or exceeds its budget, the system fails open and forwards the original request unchanged.

---

## 3. Cascaded Human Signal Extraction

HSAR treats human-state inference as a forecasting problem rather than a classification problem. The objective is early detection of interaction failure risk, not perfect emotion labeling.

### Data Contract

All tiers emit a standardized `SignalFrame` Protobuf message containing:

- Signal vectors  
- Confidence intervals  
- Abstention reason, if applicable  
- Processing timestamps  

### Tier 1: Early-Exit Signal Gate  
Latency budget: ≤2 ms, CPU only

- DSP-based features such as RMS energy, speech-rate variance, typing jitter, and response timing  
- Deterministic threshold checks  
- Output is either a signal estimate or an explicit abstention  

### Tier 2: Distilled Signal Embedding  
Latency budget: ≤30 ms, CPU via ONNX Runtime

- Quantized, distilled transformer or VAD-based models  
- Output is a vectorized signal state with confidence estimates  

**Invariant**  
Each tier must either emit a signal with confidence or abstain. This enables predictable latency, graceful degradation, and early exit under load.

---

## 4. Adaptive Policy Engine (Control-Theoretic)

HSAR executes policies as a control system rather than a rule engine.

### Policy Characteristics

- Deterministic execution  
- Semantic versioning and artifact-based deployment  
- Static validation prior to rollout  
- Hot reload with instantaneous rollback  

### Stability Mechanisms

- Hysteresis and dampening to prevent oscillation  
- Cooldown windows to avoid overreaction to transient signals  

### Outputs

Policies map signal forecasts to bounded system actions, including:

- Response verbosity limits  
- Latency budgets  
- Escalation or deflection strategies  

Each decision emits a compact policy trace containing the policy ID and version.

---

## 5. Model Runtime Adapter

The runtime adapter translates policy decisions into backend-specific controls using capability discovery and a strict fallback hierarchy.

### Control Mechanisms

1. **Control Vectors / Activation Steering**  
   Highest precision. Available only for open-weight models. Directly modifies internal representations.

2. **Logit Bias**  
   Medium precision. Penalizes specific token probabilities, such as reducing hedging or verbosity.

3. **System Context Injection**  
   Universal fallback. Prepends structured, XML-tagged instructions to the prompt context.

This hierarchy guarantees effective control across heterogeneous providers, including OpenAI, Anthropic, and local Llama deployments.

---

## 6. Counterfactual Evaluation Mode (Shadow Control Plane)

HSAR supports risk-free deployment through counterfactual evaluation.

**Traffic Mirroring**  
Live traffic passes through HSAR without behavioral intervention.

**Counterfactual Logging**  
HSAR records predicted actions and timestamps, for example:  
“Forecasted interaction failure at t=4s; would have triggered dampening.”

**Correlation Analysis**  
Offline analysis correlates forecasts with ground-truth proxies such as churn, escalation, or abandonment.

This enables policy validation prior to activation and avoids unsafe A/B experimentation.

---

## 7. Observability, SLOs, and Cost Discipline

HSAR elevates observability from raw performance metrics to human-facing service level objectives, enforced through budgets and policies.

Representative SLOs include:

- Frustration-to-escalation time  
- Silent failure rate  
- Cost per successful session  

By forecasting doomed interactions early, HSAR reduces token burn and enables cheaper exit strategies such as early human escalation or controlled session termination.

---

## 8. Concurrency, Backpressure, and Load Shedding

The sidecar proxy enforces:

- Bounded queues  
- Request-scoped deadlines  
- Circuit breakers on Tier 2 timeouts  

Under load, HSAR sheds work deterministically in the following order:

Tier 2 → Tier 1 → passthrough

This preserves availability and predictability under stress.

---

## 9. Technology Stack

- **Proxy Runtime:** Go or Rust  
- **ML Inference:** ONNX Runtime, CPU-optimized and quantized  
- **Deployment:** Docker and Kubernetes sidecar pattern  
- **Protocols:** gRPC for internal communication, REST for external APIs  

---

## 10. Non-Goals

- Perfect emotion classification  
- Replacement of model training pipelines  
- Long-term user profiling or identity inference  

---

## Summary

HSAR is not an emotion model or a prompt framework. It is a human-aware control plane designed to ensure system behavior improves faster than foundation models themselves.

By introducing bounded, deterministic governance over human-facing AI systems, HSAR addresses the dominant failure mode of modern AI deployments: unmanaged human interaction under real-world constraints.
