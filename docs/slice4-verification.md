# Slice 4: Verification Guide

## Prerequisites

- Docker and Docker Compose installed
- Port 8080 available on your machine

---

## 1. Build

```bash
docker-compose build
```

All three containers (proxy, signal-engine, backend) should build without errors.

---

## 2. Start

```bash
docker-compose up -d
```

Verify all containers are running:

```bash
docker-compose ps
```

Expected output — all three containers with status `Up`:

```
NAME                   IMAGE                STATUS     PORTS
hsar-backend-1         python:3.10-slim     Up         8081/tcp
hsar-proxy-1           hsar-proxy           Up         0.0.0.0:8080->8080/tcp
hsar-signal-engine-1   hsar-signal-engine   Up         50051/tcp
```

Wait 2-3 seconds for the gRPC connection to establish before running tests.

---

## 3. Test: Frustrated Input

Send a message with negative keywords and exclamation marks:

```bash
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  --data-binary @- <<'EOF'
{"messages":[{"role":"user","content":"This is broken and terrible, I hate this useless thing!"}]}
EOF
```

Then check the proxy logs:

```bash
docker-compose logs proxy --tail=5
```

**Expected**: A `signal_engine_signalframe` log entry with:
- `abstain: false`
- `confidence` around `0.77` (elevated due to negative keywords)
- `latency_ms` at `0` or `1` (well under 2ms)

---

## 4. Test: Empty Input (Abstain)

Send a request with no messages:

```bash
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{}'
```

Check the proxy logs:

```bash
docker-compose logs proxy --tail=5
```

**Expected**: A `signal_engine_signalframe` log entry with:
- `abstain: true`
- `confidence: 0`

---

## 5. Test: Calm Input

Send a neutral, polite message:

```bash
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  --data-binary @- <<'EOF'
{"messages":[{"role":"user","content":"Can you help me write a short poem about the ocean?"}]}
EOF
```

Check the proxy logs:

```bash
docker-compose logs proxy --tail=5
```

**Expected**: A `signal_engine_signalframe` log entry with:
- `abstain: false`
- `confidence` around `0.69` (lower than the frustrated input)
- `latency_ms` at `0` or `1`

---

## 6. Latency Check

All three tests above should show `latency_ms` of 0 or 1 in the proxy logs. Tier 1 heuristics are pure string operations with no I/O, so they complete well under the 2ms budget.

---

## Understanding the Log Output

The proxy logs signal frames as structured JSON. Here is what each field means:

| Field | Description |
|---|---|
| `tier` | Always `TIER_1` for these heuristics |
| `abstain` | `true` if the engine could not produce a meaningful signal (empty input, parse error, too-short text) |
| `confidence` | `0.0`-`1.0`, scales with signal strength. Higher values mean the engine is more certain about its risk assessment |
| `latency_ms` | Processing time inside the signal engine. Should be 0-1ms for Tier 1 |

The actual signal values (`frustration_risk`, `failure_risk`) and feature metadata (`brevity`, `caps_ratio`, etc.) are part of the `SignalFrame` protobuf but are not printed in the proxy's summary log. To inspect them, you can call the signal engine directly via gRPC or add additional logging fields to the proxy.

---

## Troubleshooting

### First request fails with `DeadlineExceeded`

The proxy uses a 30ms timeout for gRPC calls. The very first request pays the cost of TCP + HTTP/2 connection establishment, which can exceed 30ms. Subsequent requests reuse the connection and work fine. This is expected behavior — the proxy is designed to fail-open (the request still reaches the backend).

### Signal engine container has no stdout logs

Python buffers stdout by default. The gRPC server is running even if `docker-compose logs signal-engine` shows no output. You can verify by checking the port:

```bash
docker exec hsar-signal-engine-1 python3 -c "
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
print('Port open:', s.connect_ex(('127.0.0.1', 50051)) == 0)
s.close()
"
```

### Shell escaping issues with `!` in curl

Bash interprets `!` for history expansion inside double-quoted strings. Always use heredoc syntax (`--data-binary @- <<'EOF'`) or single-quoted `-d '...'` for payloads containing `!`. The examples in this guide use heredoc where needed.

---

## Tear Down

```bash
docker-compose down
```
