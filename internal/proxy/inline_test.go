package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/engine"
	"github.com/hsar-org/hsar/internal/policy"
	"github.com/hsar-org/hsar/internal/proxy"
	"google.golang.org/grpc"
)

type inlineSignalServer struct {
	hsarv1.UnimplementedSignalServiceServer
	delay  time.Duration
	abstain bool
}

func (s *inlineSignalServer) ProcessSignal(ctx context.Context, req *hsarv1.SignalRequest) (*hsarv1.SignalFrame, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &hsarv1.SignalFrame{
		TenantId:  req.TenantId,
		RequestId: req.RequestId,
		Abstain:   s.abstain,
		Signals:   []*hsarv1.Signal{{Name: "failure_risk", Value: 0.92}},
	}, nil
}

func startInlineGRPC(t *testing.T, delay time.Duration, abstain bool) (addr string, stop func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	hsarv1.RegisterSignalServiceServer(srv, &inlineSignalServer{delay: delay, abstain: abstain})
	go srv.Serve(lis)
	return lis.Addr().String(), func() {
		srv.Stop()
		_ = lis.Close()
	}
}

func testEnforceConfig() config.Config {
	return config.Config{
		Mode:           config.ModeEnforce,
		InlineBudgetMs: 30,
	}
}

func TestInlineGovernanceFailOpenOnSlowSignal(t *testing.T) {
	t.Parallel()

	addr, stop := startInlineGRPC(t, 2*time.Second, false)
	defer stop()

	path := filepath.Join("..", "..", "policies", "standard-safety-policy.yaml")
	p, err := policy.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	client, err := engine.NewClient(addr, policy.NewEvaluator(p))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	var gotBody string
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	handler := proxy.WithInlineGovernance(testEnforceConfig(), client, backend)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"angry"}]}`))
	req.Header.Set("X-Request-ID", "inline-slow")

	start := time.Now()
	handler.ServeHTTP(rec, req)
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("inline blocked too long")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(gotBody, "angry") {
		t.Fatalf("body mutated on fail-open: %s", gotBody)
	}
}

func TestInlineGovernanceMutatesOnHighRisk(t *testing.T) {
	t.Parallel()

	addr, stop := startInlineGRPC(t, 0, false)
	defer stop()

	path := filepath.Join("..", "..", "policies", "standard-safety-policy.yaml")
	p, err := policy.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	client, err := engine.NewClient(addr, policy.NewEvaluator(p))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	var gotBody string
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	})

	handler := proxy.WithInlineGovernance(testEnforceConfig(), client, backend)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"angry"}]}`))
	req.Header.Set("X-Request-ID", "inline-enforce-1")

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var parsed struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &parsed); err != nil {
		t.Fatalf("unmarshal body: %v body=%s", err, gotBody)
	}
	if len(parsed.Messages) == 0 || parsed.Messages[0].Role != "system" {
		t.Fatalf("expected injected system message, body=%s", gotBody)
	}
}

func TestInlineGovernanceFailOpenOnAbstain(t *testing.T) {
	t.Parallel()

	addr, stop := startInlineGRPC(t, 0, true)
	defer stop()

	path := filepath.Join("..", "..", "policies", "standard-safety-policy.yaml")
	p, err := policy.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	client, err := engine.NewClient(addr, policy.NewEvaluator(p))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	var gotBody string
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	})

	handler := proxy.WithInlineGovernance(testEnforceConfig(), client, backend)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("X-Request-ID", "inline-abstain")

	handler.ServeHTTP(rec, req)
	if !strings.Contains(gotBody, "hi") {
		t.Fatalf("abstain should passthrough, got %s", gotBody)
	}
}

func TestKillSwitchSuppressesMutation(t *testing.T) {
	t.Parallel()

	addr, stop := startInlineGRPC(t, 0, false)
	defer stop()

	path := filepath.Join("..", "..", "policies", "standard-safety-policy.yaml")
	p, err := policy.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	client, err := engine.NewClient(addr, policy.NewEvaluator(p))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	var gotBody string
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	})

	cfg := testEnforceConfig()
	cfg.EnforceKillSwitch = true
	handler := proxy.WithInlineGovernance(cfg, client, backend)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"angry"}]}`))
	req.Header.Set("X-Request-ID", "inline-kill")

	handler.ServeHTTP(rec, req)
	if strings.Contains(gotBody, `"role":"system"`) {
		t.Fatalf("kill switch should prevent mutation, body=%s", gotBody)
	}
}