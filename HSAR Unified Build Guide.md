# **HSAR Unified Build Guide**

**HSAR Bible (Rules of Law) \+ MVP Execution Spec (How We Ship)**  
 Status: APPROVED FOR EXECUTION

## **0\) What we are building (one sentence)**

HSAR is a **human-aware control plane sidecar** that forecasts interaction failure risk and governs model behavior with **bounded, deterministic controls**, without ever becoming a SPOF.

---

## **1\) Non-negotiable rules (must hold for every line of code)**

### **1.1 Invariants (always true)**

1. **Fail-Open**  
    If HSAR fails, the model still serves requests.

2. **Deterministic Degradation**  
    Tier 2 → Tier 1 → passthrough, always.

3. **Hard Budgets (real-time)**  
    Per-stage deadlines are enforced. No deadline, no HSAR.

4. **Monotonic Safety**  
    HSAR can reduce richness, never increase risk.

5. **Privacy & Ephemerality**  
    No raw user inputs persisted by default. Only derived signals may persist, encrypted, with explicit retention.

6. **Model Churn Tolerance**  
    Backends are replaceable. HSAR stays stable.

7. **Multi-tenant Isolation**  
    Tenant scoped signals, policies, metrics. No bleed.

### **1.2 Never do these**

* never log or store raw text/audio

* never block on Tier 2

* never allow policy oscillation

* never activate a policy without counterfactual evaluation

* never assume model APIs support advanced controls

---

## **2\) MVP scope (locked)**

### **v1 modality and backend**

* **Modality:** text only

* **Backend:** one generation backend (local llama.cpp recommended)

* **Signal goal:** forecast interaction failure risk (frustration proxy)

* **Controls:** context injection \+ token/verbosity clamps

* **Evaluation:** shadow mode \+ offline correlation

* **SLO target:** Silent Failure Rate improvement

This is the smallest slice that proves HSAR is real.

---

## **3\) Technology choices (we do not mix and match)**

### **Build stack**

* Proxy runtime: **Go 1.21+**

* Signal engine runtime: **Python 3.10 slim** (separate container)

* Inference: **ONNX Runtime CPU** (quantized)

* Contracts: **Protobuf v3**

* Internal RPC: **gRPC**

* Orchestration: **Docker Compose**

* Analysis: **Python \+ Pandas**

### **Why this split**

* Go proxy stays fast and safe under load

* Python holds ML ecosystem without poisoning proxy latency

* ONNX keeps Tier 2 CPU-only and predictable

---

## **4\) Architecture (steel thread)**

### **Data flow (runtime)**

User → Go Proxy  
 Proxy → Python Signal Engine (gRPC, deadline)  
 Signal Engine → Tier 1 then Tier 2 (early exit)  
 SignalFrame → Proxy Policy Engine  
 Policy → Request Mutator  
 Mutated request → Model Backend  
 Response → User

### **Control mechanism fallback hierarchy (must be enforced)**

1. control vectors (open weights only)

2. logit bias (if supported)

3. system context injection (universal)

MVP uses \#3 and clamps, but the hierarchy is part of the design.

---

## **5\) Hard constraints for real-time usefulness**

### **Latency budgets (p99 added overhead target)**

* gRPC call: ≤ 5 ms

* Tier 1: ≤ 2 ms

* Tier 2: ≤ 30 ms

* policy \+ mutation: ≤ 2 ms

* total added overhead target: ≤ 35 ms local  
   If anything misses its deadline: **timeout \+ abstain \+ degrade**

### **Availability under load**

* bounded queues

* backpressure

* circuit breaker on Tier 2 timeouts

* deterministic load shedding: Tier 2 → Tier 1 → passthrough

---

## **6\) Data contracts (the glue)**

### **SignalFrame.proto must include**

* tenant\_id

* request\_id

* tier

* timestamps (start/end)

* signals vector

* confidence / CI

* abstain boolean

* abstain\_reason enum

No raw user content. Ever.

### **PolicyTrace must include**

* policy\_id

* policy\_version

* decision\_id

* actions applied

* cooldown/hysteresis state

This is how we debug without raw data.

---

## **7\) Policy engine rules (control-theoretic)**

Policies are artifacts:

* semver versions

* static validation before rollout

* hot reload

* instant rollback

Stability mechanisms are mandatory:

* hysteresis: separate enter/exit thresholds

* dampening: gradual change

* cooldown windows: prevent rapid toggling

Policies output bounded actions only:

* verbosity clamp

* token budget

* escalation/deflection flags

---

## **8\) Evaluation rules (how we ship without harming users)**

### **Counterfactual first (always)**

Before activation:

* mirror traffic

* log “would have acted”

* correlate with abandonment/escalation/re-ask signals

Activation gate:

* predictive power threshold met

* no safety regression

* latency budgets met

Rollback:

* revert policy artifact instantly

---

## **9\) Definition of done (MVP completion criteria)**

MVP is done only when:

1. proxy is fail-open and survives injected faults

2. deterministic degradation works under load

3. tiered cascade meets p99 budgets

4. policy engine does not oscillate

5. shadow mode produces measurable predictive metric

6. at least one SLO improves in controlled test

7. no raw user inputs persist by default

8. repo is reproducible via docker-compose up

---

# **Execution plan (8 vertical slices)**

## **Slice 1: Repo \+ contracts**

* initialize repo

* define SignalFrame.proto \+ PolicyTrace.proto

* generate Go and Python bindings

* docker compose skeleton

Proof: Go ↔ Python gRPC handshake.

## **Slice 2: Proxy hollow shell**

* Go proxy intercepts requests

* passthrough to backend

* deadlines \+ bounded queue

Proof: kill proxy dependencies, passthrough still works.

## **Slice 3: Signal engine stub**

* Python gRPC server returns fixed SignalFrame

* proxy logs trace

Proof: end-to-end trace exists.

## **Slice 4: Tier 1 heuristics**

* implement cheap text features \+ abstain logic

* prove ≤2ms

Proof: emit/abstain correctness.

## **Slice 5: Tier 2 ONNX inference**

* integrate ONNX CPU quantized model

* implement circuit breaker and timeouts

Proof: Tier 2 works, then degrades deterministically under forced timeout.

## **Slice 6: Policy engine \+ stability**

* policy artifacts \+ semver

* hysteresis \+ cooldown

* emit PolicyTrace

Proof: no oscillation on borderline signals.

## **Slice 7: Request mutator \+ adapter**

* context injection \+ token clamps

* capability discovery stub

* record control mechanism used

Proof: same input yields different outputs under different policies.

## **Slice 8: Shadow mode \+ offline analysis**

* traffic mirroring \+ counterfactual logs

* offline correlation

* SLO comparison report

Proof: one plot/table showing predictive value and SLO improvement.

---

## **The one sentence we execute by**

We only build what is needed to prove:  
 **observe → forecast → decide → act → measure**  
 under strict budgets, safe degradation, and privacy constraints.

