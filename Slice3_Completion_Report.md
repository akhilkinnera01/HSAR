# Slice 3 Completion Report: Signal Engine Stub — End-to-End Wiring

**Date:** 2026-02-05
**Branch:** Feb5_changes
**Status:** COMPLETE — End-to-end trace verified via `docker-compose up` + `curl`

---

## Objective

Slice 3 of the HSAR execution plan requires:

> - Python gRPC server returns fixed SignalFrame
> - Proxy logs trace
> - **Proof: end-to-end trace exists**

Before this work, the Python signal engine stub and Go gRPC client existed as standalone pieces but were never connected. The proxy forwarded requests to the backend without ever calling the signal engine. Additionally, several files contained bugs that prevented compilation and execution entirely.

---

## Pre-Existing State (Before Slice 3)

### What existed
- `proto/hsar/v1/signal_frame.proto` — SignalFrame data contract
- `proto/hsar/v1/signal_service.proto` — gRPC service definition (`ProcessSignal` RPC)
- `proto/hsar/v1/policy_trace.proto` — PolicyTrace data contract
- `gen/go/hsar/v1/` — Generated Go protobuf/gRPC bindings
- `gen/python/hsar/v1/` — Generated Python protobuf/gRPC bindings
- `cmd/proxy/main.go` — Go HTTP reverse proxy with middleware (logging, tracing, deadlines)
- `internal/engine/client.go` — Go gRPC client for signal engine (not wired into proxy)
- `signal-engine/main.py` — Python gRPC server returning fixed signals (not callable due to bugs)
- `deploy/echo_backend.py` — Mock OpenAI-compatible backend
- `deploy/Dockerfile.proxy` — Two-stage Go build
- `deploy/Dockerfile.signal` — Python signal engine container
- `docker-compose.yml` — Three-service orchestration (proxy, signal-engine, backend)

### What was broken

**1. Shell heredoc wrappers left in source files**

Three files were created using shell heredoc syntax (`cat > file << 'EOF' ... EOF`) and the wrapper lines were accidentally committed as part of the file contents:

- `internal/engine/client.go` — Had `mkdir -p internal/engine` and `cat > internal/engine/client.go << 'EOF'` at the top, `EOF` at the bottom. Go compiler would reject this.
- `signal-engine/main.py` — Had `cat > signal-engine/main.py << 'EOF'` at the top, `EOF` at the bottom. Python interpreter would crash.
- `deploy/echo_backend.py` — Had `cat > deploy/echo_backend.py << 'EOF'` at the top, `EOF` at the bottom. Python interpreter would crash.

**2. Proto RPC method name mismatch**

The proto defines the RPC as `ProcessSignal`:

```protobuf
service SignalService {
  rpc ProcessSignal(SignalRequest) returns (SignalFrame);
}
```

But both implementations used a non-existent method name `GetSignals`:
- `internal/engine/client.go` called `c.stub.GetSignals(ctx, req)` — would fail to compile since the generated client interface has `ProcessSignal`, not `GetSignals`.
- `signal-engine/main.py` implemented `def GetSignals(self, request, context)` — gRPC would return `UNIMPLEMENTED` since the server registered `ProcessSignal` as the method name.

**3. Proto field name mismatch in Go client**

The proto defines the request field as `text_payload`:

```protobuf
message SignalRequest {
  string tenant_id = 1;
  string request_id = 2;
  string text_payload = 3;
}
```

The generated Go struct has the field as `TextPayload`, but `client.go` used:
- `Text: text` — non-existent field, would fail to compile.
- `TsRequestMs: time.Now().UnixMilli()` — non-existent field (not in the proto), would fail to compile.

**4. Python grpcio version mismatch**

`signal-engine/requirements.txt` pinned:
```
grpcio==1.60.0
grpcio-tools==1.60.0
protobuf==4.25.1
```

But the generated Python gRPC code (`gen/python/hsar/v1/signal_service_pb2_grpc.py`) was generated with `protoc-gen-grpc` v1.76.0 and contained a runtime version check:

```python
GRPC_GENERATED_VERSION = '1.76.0'
```

This check raises `RuntimeError` if the installed `grpcio` version is lower than `1.76.0`. The signal engine container would crash immediately on import.

**5. Proxy never called the signal engine**

The proxy's request handler chain was:
```
withLogging → withTraceID → withMethodEnforcement → withRequestDeadline → reverse proxy
```

The `internal/engine` package was never imported. No gRPC client was initialized. No signal engine call was made. Requests went straight through to the backend without any signal processing.

---

## Changes Made

### File 1: `internal/engine/client.go`

**Full rewrite.** This file was completely rewritten to fix all issues simultaneously:

| Issue | Before | After |
|-------|--------|-------|
| Heredoc wrapper | Lines 1-2: shell commands, last line: `EOF` | Removed — clean Go source |
| RPC method | `c.stub.GetSignals(ctx, req)` | `c.stub.ProcessSignal(ctx, req)` |
| Text field | `Text: text` | `TextPayload: text` |
| Timestamp field | `TsRequestMs: time.Now().UnixMilli()` | Removed (not in proto) |
| Connection mode | `grpc.DialContext()` with `grpc.WithBlock()` — blocks startup for up to 2s, fails if signal engine isn't ready | `grpc.NewClient()` without blocking — lazy connection, fail-open at startup |

The connection mode change is architecturally significant. With `grpc.WithBlock()`, the proxy would block during startup waiting for the signal engine to accept connections. With Docker Compose, `depends_on` only waits for the container to start, not for the service to be ready. The lazy connection via `grpc.NewClient()` means:
- The proxy starts immediately regardless of signal engine state.
- The gRPC connection is established on the first actual RPC call.
- If the first call's connection handshake exceeds the 30ms timeout, it gracefully degrades (logs a warning, serves the request without signal data).
- Subsequent calls reuse the established connection and complete in ~1ms.

This is the correct fail-open behavior specified by the HSAR invariants.

### File 2: `signal-engine/main.py`

| Issue | Before | After |
|-------|--------|-------|
| Heredoc wrapper | Line 1: `cat > signal-engine/main.py << 'EOF'`, last line: `EOF` | Removed — clean Python source |
| Method name | `def GetSignals(self, request, context)` | `def ProcessSignal(self, request, context)` |

The stub logic is unchanged — it returns deterministic dummy signals (`frustration_risk=0.12`, `failure_risk=0.05`) with `TIER_1`, `confidence=0.90`, and `engine=stub_v1` metadata. This is intentional for Slice 3: proving wiring and contracts, not intelligence.

### File 3: `deploy/echo_backend.py`

| Issue | Before | After |
|-------|--------|-------|
| Heredoc wrapper | Line 1: `cat > deploy/echo_backend.py << 'EOF'`, last line: `EOF` | Removed — clean Python source |

No logic changes. The mock backend returns a fixed OpenAI-compatible JSON response.

### File 4: `signal-engine/requirements.txt`

| Package | Before | After | Reason |
|---------|--------|-------|--------|
| `grpcio` | `1.60.0` | `1.76.0` | Generated gRPC code requires `>=1.76.0` |
| `grpcio-tools` | `1.60.0` | `1.76.0` | Must match `grpcio` version |
| `protobuf` | `4.25.1` | `>=6.31.1,<7.0.0` | `grpcio-tools==1.76.0` requires `protobuf>=6.31.1` |
| `onnxruntime` | `1.16.3` | `1.16.3` | Unchanged (needed for future Tier 2) |
| `numpy` | unpinned | unpinned | Unchanged |

The protobuf version was initially set to `5.29.4` but Docker build revealed that `grpcio-tools==1.76.0` requires `protobuf>=6.31.1`. The range `>=6.31.1,<7.0.0` was used to allow pip to resolve the exact compatible version (resolved to `6.33.5` at build time).

### File 5: `cmd/proxy/main.go`

This is the core wiring change. Three additions were made:

**A. New imports**

```go
import (
    "bytes"
    "io"
    "github.com/hsar-org/hsar/internal/engine"
)
```

**B. Fail-open client initialization (in `main()`)**

```go
var signalClient *engine.Client
sc, err := engine.NewClientFromEnv()
if err != nil {
    logger.Warn("signal_engine_unavailable", "error", err)
} else {
    signalClient = sc
    logger.Info("signal_engine_connected", "target", os.Getenv("SIGNAL_ENGINE_TARGET"))
}
```

If `NewClientFromEnv()` fails (e.g., invalid target address), the proxy logs a warning and continues with `signalClient = nil`. All downstream code checks for nil before calling the signal engine.

On shutdown, the client is closed if it was initialized:
```go
if signalClient != nil {
    signalClient.Close()
}
```

**C. `withSignalEngine` middleware**

```go
func withSignalEngine(client *engine.Client, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if client != nil {
            body, err := io.ReadAll(r.Body)
            r.Body.Close()
            if err == nil {
                r.Body = io.NopCloser(bytes.NewReader(body))
                r.ContentLength = int64(len(body))
                traceID := r.Header.Get("X-Request-ID")
                go client.ShadowGetSignals("default", traceID, string(body))
            } else {
                slog.Warn("signal_engine_body_read_failed", "error", err)
                r.Body = io.NopCloser(bytes.NewReader(nil))
                r.ContentLength = 0
            }
        }
        next.ServeHTTP(w, r)
    })
}
```

Key design decisions in this middleware:

1. **Nil check**: If `client` is nil (signal engine unavailable), the request passes straight through. No signal processing, no error — fail-open.

2. **Body read + restore**: The HTTP request body is a one-time-read `io.ReadCloser`. We read it fully via `io.ReadAll`, then restore it via `io.NopCloser(bytes.NewReader(body))` so the reverse proxy can read it again. `r.ContentLength` is also reset to match.

3. **Non-blocking goroutine**: `go client.ShadowGetSignals(...)` fires the gRPC call in a separate goroutine. The middleware immediately proceeds to `next.ServeHTTP(w, r)` without waiting. The signal engine response is logged asynchronously. This ensures the signal engine never adds latency to the user-facing request path.

4. **Tenant ID**: Hardcoded to `"default"` for the MVP (single-tenant). Multi-tenant support is a future slice.

5. **Full body as text payload**: The entire JSON request body is sent as `TextPayload`. In future slices, this will be parsed to extract only the user message content.

**Updated middleware chain:**

```
withLogging → withTraceID → withMethodEnforcement → withRequestDeadline → withSignalEngine → reverse proxy
```

The `withSignalEngine` middleware sits between `withRequestDeadline` and the reverse proxy, so it has access to the trace ID (set by `withTraceID`) and operates within the request deadline.

---

## Verification

### Build

```bash
docker-compose build
```

All three containers built successfully:
- `hsar-proxy` — Two-stage Go build (golang:1.24-alpine → scratch)
- `hsar-signal-engine` — Python 3.10-slim with grpcio 1.76.0
- `hsar-backend` — python:3.10-slim running `echo_backend.py`

### Run

```bash
docker-compose up -d
```

All three services started:
- `proxy` on port 8080
- `signal-engine` on port 50051 (internal)
- `backend` on port 8081 (internal)

### Test

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"hello"}]}'
```

Response:
```json
{"id":"echo","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"echo backend ok"},"finish_reason":"stop"}]}
```

### Proxy Logs (End-to-End Trace — Slice 3 Proof)

**Startup:**
```json
{"msg":"starting_hsar_proxy","port":"8080","backend":"http://backend:8081"}
{"msg":"signal_engine_connected","target":"signal-engine:50051"}
{"msg":"server_listening"}
```

**First request (cold connection — graceful degradation):**
```json
{"msg":"request_served","trace_id":"68424556-...","status":200,"duration_ms":26}
{"msg":"signal_engine_call_failed","trace_id":"68424556-...","error":"rpc error: code = DeadlineExceeded desc = context deadline exceeded"}
```

The first call exceeds 30ms because the lazy gRPC connection must establish TCP + HTTP/2 handshake. The proxy serves the request anyway (fail-open). This is correct behavior per the HSAR invariant: "If anything misses its deadline: timeout + abstain + degrade."

**Second request (warm connection — full signal trace):**
```json
{"msg":"signal_engine_signalframe","trace_id":"0abc15c8-...","tier":"TIER_1","abstain":false,"confidence":0.9,"latency_ms":1}
{"msg":"request_served","trace_id":"0abc15c8-...","status":200,"duration_ms":15}
```

The signal engine responds in 1ms (well within the 30ms Tier 2 budget). The `signal_engine_signalframe` log entry proves the end-to-end trace exists: proxy → gRPC → signal engine → SignalFrame → logged.

---

## Design Invariants Verified

| Invariant | Status | Evidence |
|-----------|--------|----------|
| **Fail-Open** | PASS | First request served successfully despite signal engine timeout. Proxy would continue serving if signal engine crashed entirely. |
| **Deterministic Degradation** | PASS | Cold-start timeout → warn log + passthrough. Warm calls → full signal trace. |
| **Hard Budgets** | PASS | 30ms gRPC deadline enforced. Warm calls complete in ~1ms. |
| **Privacy & Ephemerality** | PASS | Only derived signals logged (tier, confidence, abstain, latency). No raw user content in logs. |
| **Non-blocking** | PASS | Signal engine call runs in goroutine. Request path latency is unaffected. |

---

## What's Next (Slice 4)

Slice 4: **Tier 1 Heuristics**
- Replace the stub's fixed signals with real cheap text features (message length, repetition detection, question density, response timing)
- Implement abstain logic (empty input, too short, etc.)
- Prove ≤2ms processing time
- Proof: emit/abstain correctness
