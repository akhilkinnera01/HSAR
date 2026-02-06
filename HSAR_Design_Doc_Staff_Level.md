# **HSAR: A Human-Aware Control Plane for Foundation Model**

Human Signal–Aware AI Runtime (HSAR)

### **Abstract**

Modern AI systems fail less often due to model capability and more often due to unmanaged human–system feedback loops. While foundation models continue to improve in accuracy and scale, production reliability degrades when systems cannot adapt to human interaction under real-world latency, cost, privacy, and safety constraints.

HSAR defines a model-agnostic control plane that operates as a production-grade sidecar between users and foundation models (LLMs, TTS, multimodal agents). The system observes lightweight human interaction signals in real time, forecasts interaction failure risk, and dynamically governs model behavior using bounded, deterministic controls. HSAR improves last-mile AI reliability without retraining models, introducing new single points of failure, or increasing GPU utilization.

### **1\. Core System Invariants**

* **Fail-Open by Design:** HSAR must never block or degrade core model availability. If HSAR fails or exceeds its latency budget, requests pass through unmodified.  
* **Deterministic Degradation:** On timeout or error, HSAR deterministically degrades to the next lower capability tier (Tier 2 → Tier 1 → passthrough) and emits an internal event.  
* **Strictly Bounded Overhead:** All signal extraction, policy evaluation, and request mutation execute within predefined latency and cost envelopes using CPU-bound inference.  
* **Monotonic Safety:** HSAR interventions may reduce response richness or verbosity but must never increase operational, compliance, or user risk.  
* **Privacy & Ephemerality:** Raw user inputs (audio or text) are processed in ephemeral memory buffers and discarded immediately after signal extraction. Only anonymized, derived signal vectors may persist, with explicit retention limits and encryption at rest.  
* **Model Churn Tolerance:** Foundation models are treated as replaceable backends. HSAR remains stable across model upgrades, vendor changes, and API evolution.  
* **Multi-Tenant Isolation:** Policies, metrics, and derived signals are strictly tenant-scoped with hard isolation and least-privilege access.

### **2\. High-Level Architecture**

**User Request** →

![][image1]→

![][image2]↓

![][image3]↓

![][image4]↓

### **![][image5]3\. Cascaded Human Signal Extraction**

HSAR treats human-state inference as a **forecasting problem**, not a classification problem. The goal is early detection of interaction failure risk, not perfect emotion labeling.

**Data Contract:** All tiers emit a standardized SignalFrame Protobuf message containing:

* signal vectors  
* confidence intervals  
* abstention reason (if applicable)  
* processing timestamps  
* **Tier 1: Early-Exit Signal Gate (≤2 ms, CPU)**  
  * DSP-based features: RMS energy, speech-rate variance, typing jitter, response timing.  
  * Deterministic threshold checks.  
  * *Output:* signal estimate or explicit abstention.  
* **Tier 2: Distilled Signal Embedding (≤30 ms, CPU / ONNX)**  
  * Quantized distilled transformer or VAD-based models.  
  * *Output:* vectorized signal state with confidence estimates.

*Invariant:* Each tier must either emit a signal with confidence or abstain, enabling predictable latency, graceful degradation, and early exit.

### **4\. Adaptive Policy Engine (Control-Theoretic)**

HSAR executes policies as a **control system**, not a rule engine.

* **Policy Characteristics:**  
  * Deterministic execution.  
  * Semantic versioning and artifact-based deployment.  
  * Static validation prior to rollout.  
  * Hot reload with instantaneous rollback.  
* **Stability Mechanisms:**  
  * **Hysteresis and dampening** to prevent oscillation.  
  * Cooldown windows to avoid overreaction to transient signals.  
* **Outputs:** Policies map signal forecasts to bounded system actions, including:  
  * Response verbosity limits.  
  * Latency budgets.  
  * Escalation or deflection strategies.

Each decision emits a compact policy trace containing policy ID and version.

### **5\. Model Runtime Adapter**

The adapter layer translates policy decisions into backend-specific controls using capability discovery and a **Strict Fallback Hierarchy**:

1. **Control Vectors / Activation Steering:** (Highest Precision, Open Weights only) Directly modifying internal model representations.  
2. **Logit Bias:** (Medium Precision) Penalizing specific token probabilities (e.g., reducing "hedging" words).  
3. **System Context Injection:** (Universal Fallback) Pre-pending XML-tagged instructions to the prompt context.

This guarantees effective control across heterogeneous providers (OpenAI, Anthropic, or local Llama deployments).

### **6\. Counterfactual Evaluation Mode (Shadow Control Plane)**

HSAR supports risk-free deployment through counterfactual evaluation:

1. **Traffic Mirroring:** Live traffic passes through HSAR without behavioral intervention.  
2. **Counterfactual Logging:** HSAR records predicted actions and timestamps (e.g., *“Forecasted interaction failure at t=4s; would have triggered dampening.”*)  
3. **Correlation Analysis:** Offline analysis correlates forecasts with ground-truth proxies such as churn, escalation, or abandonment.

This enables policy validation prior to activation and avoids unsafe A/B experimentation.

### **7\. Observability, SLOs, and Cost Discipline**

HSAR elevates observability from raw performance metrics to human-facing SLOs, enforced through budgets and policies:

* **Frustration-to-Escalation Time**  
* **Silent Failure Rate**  
* **Cost per Successful Session**

By forecasting doomed interactions early, HSAR reduces token burn and enables cheaper exit strategies such as early escalation to a human or controlled session termination.

### **8\. Concurrency, Backpressure, and Load Shedding**

The sidecar proxy enforces:

* Bounded queues.  
* Request-scoped deadlines.  
* Circuit breakers on Tier 2 timeouts.

Under load, HSAR sheds work deterministically in the following order: **Tier 2 → Tier 1 → passthrough**, preserving availability and predictability.

### **9\. Technology Stack**

* **Proxy Runtime:** Go or Rust (High concurrency, strict typing).  
* **ML Inference:** ONNX Runtime (CPU-optimized, quantized).  
* **Deployment:** Docker / Kubernetes sidecar pattern.  
* **Protocols:** gRPC (internal), REST (external).

### **10\. Non-Goals**

* Perfect emotion classification.  
* Replacement of model training pipelines.  
* Long-term user profiling or identity inference.

### **Summary**

HSAR is not an emotion model or a prompt framework. It is a **human-aware control plane** that ensures system behavior improves faster than foundation models themselves. By introducing bounded, deterministic governance over human-facing AI systems, HSAR addresses the primary failure mode of modern AI deployments: unmanaged human interaction under real-world constraints.

[image1]: <data:image/png;base64,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>

[image2]: <data:image/png;base64,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>

[image3]: <data:image/png;base64,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>

[image4]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAmwAAAA/CAYAAABdEJRVAAAG4ElEQVR4Xu3d6c8WVxkH4P4nwAuUrUBBuoFFkTYUu0KxldKFIqUgVsCKbIUWiKVQQCg7QZZSoPuG1tqFVm1xiVtijGlcaozRxH2LVb+M3EPmcZ45D1UCrznNe3248sy5zzkz8/Lplzkzh3Me/fq/CgAA8nVOswAAQF4ENgCAzAlsAACZE9gAADInsAEAZE5gAwDInMAGAJA5gQ0AIHMCGwBA5gQ2AIDMCWwAAJkT2AAAMiewAQBkTmADAMicwAYAkDmBDQAgcwIbAEDmBDYAgMwJbAAAmRPYAAAyJ7ABicFDRxS9evUq9Tt3YDFoyLCiq6tv2e7br39x8Ct/SeacynnDR7XO1exbs+94seHw95L6maqu924efu1Pybyz5fKrpyU1gDMhsAEdXTftrqJPV1dSrwJPs/5uYnwEvk71lTtfSepnw6nu89Abf+9YP1vuundP0f9EyG3WAc6EwAZ0FKHmtk/en9QHDh562oEnxk+ftyapd6e4Zu/evZN61desAeRMYAM6ilDTadkw6qcTeLY/97OT53r9z0lfd1l/6DvlNW+ec1+rtnr3a8WR4/8oj0/n/gFyILABiaUbn+0Yaj529/qyvnLHy231aEf9wjGXFecOHHwipP201Xf++0Yn5xoz7qpi9Lgrk3o4cOyP5VJsnCf6Hzz47bLe1bdfMfKiscWwERe2je90jgEDhyT1Zrsy6LzhZd9DT/6ouPSya8u/IdpDho5MxoZLx1/Turf7tr9UXDH59rK+6bEflO/6Na9zxaTpZW3p554rDp4IrcNGXFT+ffEuYPPcEZD79R9Q9OnTVQwfeXGx4/m3kzFAzySwAYkLRn8oCR7jJt5Q1rY98+O2ejw5izBVr9XnxvHYyye12vNX7S/WPfytZFy4d+uLZW3/sT+0+sMjX/1b2V68/qm2OfNW7k3OUZ9Xd/6o0cm4LU+9Vf5GQGqep9muajfdubw8PvzGO23njSDWnBdP9K6ZOrcYf+XUsv7+8Ve3nWvh2seS8+996Tfveg9AzySwAYkICuH6WxcUU6bfXdw4c0mx9+XfJuOqsVWgqtcifFXHq3e/3tYXv59YvjsJJNH+1GcfaWvH07txEz9StgcObn+C1WzX59XDUYSpZjgK8aSvGv/fntx1eiIY7fmrDpTH+175XTLvlrmrT/zb/LWs9e7dp8Pc/a12PNG7ff7aVvvwm+8Uc5Zsa5sD9FwCG9Bm9wu/LMPE5FvmJ31NEaSaISZEbdXOV1vHzf6qPm32ilY7lj5PNTbEcmL0b37ih23nmLt8VzI26vX376plyxBLnvWxe1781cnxtXfsYluO5r00rzVn6fZkzCUfmJjUqrmrdh1rte9c9FDbuLjXaE+ddU9xx8JN5VJr8xxAzyawAW2mTP90GR52HH076WuKcacKKPG78+jPk+XSsGLrl5J58SRvxAVjkrGVWIqsz6kCXvUhQWXtgW8k566L99vq7RtnLk7GR7u5l1rUYhm0avcfMKjjvGrJtFlvtuv3sWbvm8kYgDqBDWgTweF/DQ/xgnxzbLy4P2PByaW9CGHVdh6xgW41JvZkq/Z4u3jshPL3M+seLybfuiC5xqJ1T5S/oy4Z13atCdfdllw7xPti8U5asx62PfuT8uOCei3OUd9vbs+Xf13W4uOH+MCgPq4576obZrXaK7a80BoTHx80xzbb1RO3eKIZy5/NMaHTMi7QMwlsQEsVHDqFh04e2He8beyiB59sa0+cPKP8wGDhA48Waw98s1WPMbMXb2l7YlXV6+14R61aWv3oHcva+uM4+uvjq3p9O48Qy6mTbp6XnL8aP2PBulb748v+s9RZfUhQjYsnYXEcm+NGu748GxsNV/NiybOqL9t0tPjghOtb7WrLkTievWRrq958xy2eTs5atLmtBvRcAhtwxu7Z/IVi45HvJ/WwZMPTSS3EF5+d9maL97ci4EV4bPYdOf7Pct7Wp98qQ088MWuOOV1LNj6T1HZ98Rflf5vVrNd1Cn+xTLvh0HfbatXHCHX7X/19x/fU4gvcZZue7/i3Az2bwAa8J9QDUmx90Wkfs/+X+z//tY7bhAB0F4ENyF68T1YPbPEhQ9Sa47pLXDuWf+vt5nIuQHcS2ID3hNi3LbbniJf7m33dLZZuPzxlZnHtTXPL/wWi2Q/Q3QQ2AIDMCWwAAJkT2AAAMiewAQBkTmADAMicwAYAkDmBDQAgcwIbAEDmBDYAgMwJbAAAmRPYAAAyJ7ABAGROYAMAyJzABgCQOYENACBzAhsAQOYENgCAzAlsAACZE9gAADInsAEAZE5gAwDInMAGAJA5gQ0AIHMCGwBA5gQ2AIDMCWwAAJkT2AAAMvdvMKSL0iXbPkYAAAAASUVORK5CYII=>

[image5]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAmwAAAA/CAYAAABdEJRVAAAHnUlEQVR4Xu3d53NVRRzGcf8TQmghQOgEkFCUIh3pKiJCYgGlM5QM0qQIIUQFFERBuiIgIoiMBRVhVOxlLOM41hc6OqKOqG+OPuvscc/uTSaDMrPmfl985p79bbnnhhf3mT3nXC7bf/aPBAAAAPG6zC8AAAAgLgQ2AACAyBHYAAAAIkdgAwAAiByBDQAAIHIENgAAgMgR2AAAACJHYAMAAIgcgQ0AACByBDYAAIDIEdgAAAAiR2ADAACIHIENAAAgcgQ2AACAyBHYAAAAIkdgAwAAiByBDQAAIHIENgAAgMgR2AAAACJHYAMAAIgcgQ3IE1uOfZm0bN02adSokdGsRcukqLhN2p6x7KFgzv9Zq5KO6Wfr3ntQ0G+17dgtHdemXeegHwBiQGAD8oyCSVnf4ZnaqgdfMnV/7P9BXee9qPrxZOyU+XWOqZi3wfSv3/NG0Fdf1968OFm6+WRQr4+6zg0ALAIbkGcUEFZsfT5nfdGGI0E9Zrcv2VZn4OnWa6B5rW2M5vfsN6LW/vrS/H1nfgvq9fFv3xtAfiCwAXkmV0DYevwrU9985NOgL2YtW5Xk/DyW7attzODRFaavtv76utj5G/a/kxQVlwR1APAR2IA8Mq58YVLYpGmmtvP5H03gaN+5RzC+ddtOpq9r2YDkjnuPZ/pWbD1l+rRDtXzLc+ZesOp9b5m+wsImSYfSnknbDl0zc3IFm4EjJ5t67wGjkrIrhwX9Pa4cau4ta9qsRbJmx1lTU9sGLcufp8814rrb0/f1d8Auv2Jo2jdj2fZgfteeA8znuHXhprRWtftc+l7Dr70tOId2Hbtn1igt62/updO9gnNW7UkmTF2Wvqdv7pr9mbl9h1yTlLQvNf8Gmm/r+vdrXlSc7H7xZ7N29z6DzXz//AE0LAQ2II/oi71jaa9k9KS5ybBrppkv+yZNmyU1j7wbjNXu1fq9b2bm+mu5x7a964WfzOuCqoOZMdMWbwnWGFe+wLDtJk2bZ/oV1m6Yvsocj7x+ZmZ+zaPvmfa+M79n5liTZqxONh7+2Bxr3OrtZ9K+y/sMSY/9c5KplfeZcKi/ldtvH9Jwxxa3aR/U7Lrlc6vN8X1HPzPtq66eFIzx59m67ouz7ZL2Xf4KaL+Yv2n1vrfNuWnMntO/5vy7Amh4CGxAHtEXu3ZmbHvJxhNBSBK76+XW+g+fmB63KGpl5tq23YnTcZ+BY8yrLvW5a2iO29ZunP8eBQWNM231j7lxnjkedcPszC7U4DF/X850x7saFxZm1imfs94c3//k58mSTU+bY7u76M+1Nb0OGl2eqY+eNCcYq0urbs3/rHbcXQ+/krYVNP0xon8P99xl7OT5ZneuceO/61dPmJGMr6g0x/qb6Ilffx0ADQuBDcgTC6oO5QwIqo2vWBTUdDlTIUdPWu59+ULaV1nzRLCO2nra0rZ3nTpvancf+CAzZtadOzNt0U7YzOUPJw+d/Dazpmj3zY4T/z3dMOVTiHTH6vKijvVetq4dvKHjbgnmuvPscdWe14NzqG2XT7VZK/75rKMmzgrmDhl7U1CbMqfK1BQk3bqCrN1ptOu7/QAaPgIbkCc6dCnL+UWvWsW8mrRtd36mVm4Oxkq/YROCddTWJTvb1uU8d0yu3TS167sztHLbC2b89KUPZubr8qA/VvQTG24w01hxL4va+t0H3g/miwJkQUFB2s6165grdNl1/Xaz5kVBzX/goLh1u2CuHeveg5drDICGjcAG5Al9yftf9AoBqmn3zR9bWXM0U9ODBXrVpTh/Hb/dqVufTM0NO5269U7nlPbol5m389T59FiXBd0b/jXe3YVz12/RsnVmHT0k4bbtDwbrQQi37p+3a2H1YXOfnzvW3vxv5+lVlz91rFCpBzFyrau2ds/8mg3FeuBCr7qc7M+deNuKTG3h+tw7pQAaNgIbkAe2P/Od+ZL3f/HfBjZd5lRbu2d61eVDPfVpxylI2fu0dOwGBj1JqQcX3HXtpUwd73juB3Ns2/ce/NC85voNNfdpSL9P7+O2bb8+m3tfnt0hfOCpr9Oagpe/3uyVu4OaSz914u6AaawefNh7+oJ5QtTW7JOotZ27gq7a+gkPPTTgjtl0+JNk3a5z5sd9VVMgVd3uVqpPbffSskJuXecNoGEisAF5buW2F00QGTZ+atA3f+2B9KlPn3Z69Kp7xbSG36/gZMfIjme/T2/2z7zHuseSLce+COqi34VbfM8xE/r8PoVNd/1LYduJb5J5a/abpzHV1j1rClnuGAXQtbteC+ZuPPRRsuz+Z9K2LtPadSz3wQ2XnirV+/p1Wbvz1WAdAA0fgQ3Av8JuDwBcegQ2ABdNu28ENgC49AhsAC6KLvHp/yTVzfa6bOn3AwD+OwQ2AACAyBHYAAAAIkdgAwAAiByBDQAAIHIENgAAgMgR2AAAACJHYAMAAIgcgQ0AACByBDYAAIDIEdgAAAAiR2ADAACIHIENAAAgcgQ2AACAyBHYAAAAIkdgAwAAiByBDQAAIHIENgAAgMgR2AAAACJHYAMAAIgcgQ0AACByBDYAAIDIEdgAAAAiR2ADAACIHIENAAAgcgQ2AACAyBHYAAAAIkdgAwAAiNyf+rMm8qTwy0kAAAAASUVORK5CYII=>