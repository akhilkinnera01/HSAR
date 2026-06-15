package proxy_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/engine"
	"github.com/hsar-org/hsar/internal/proxy"
	"google.golang.org/grpc"
)

type slowSignalServer struct {
	hsarv1.UnimplementedSignalServiceServer
	delay time.Duration
}

func (s *slowSignalServer) ProcessSignal(ctx context.Context, req *hsarv1.SignalRequest) (*hsarv1.SignalFrame, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &hsarv1.SignalFrame{
		TenantId:   req.TenantId,
		RequestId:  req.RequestId,
		Confidence: 0.5,
	}, nil
}

func startSignalGRPC(t *testing.T, delay time.Duration) (addr string, stop func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := grpc.NewServer()
	hsarv1.RegisterSignalServiceServer(srv, &slowSignalServer{delay: delay})

	go srv.Serve(lis)

	return lis.Addr().String(), func() {
		srv.Stop()
		_ = lis.Close()
	}
}

func TestWithShadowSignalAnalysisFailOpenWhenEngineSlow(t *testing.T) {
	t.Parallel()

	addr, stop := startSignalGRPC(t, 2*time.Second)
	defer stop()

	client, err := engine.NewClient(addr, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	cfg := config.Config{Mode: config.ModeShadow}
	handler := proxy.WithShadowSignalAnalysis(cfg, client, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[]}`))
	req.Header.Set("X-Request-ID", "shadow-slow")

	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("shadow call blocked request for %v; want fail-open < 500ms", elapsed)
	}
}

func TestWithShadowSignalAnalysisFailOpenWhenClientNil(t *testing.T) {
	t.Parallel()

	var called bool
	cfg := config.Config{Mode: config.ModeShadow}
	handler := proxy.WithShadowSignalAnalysis(cfg, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[]}`))
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected backend handler to run when signal client is nil")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

type recordingAnalyzer struct {
	mu    sync.Mutex
	calls int
}

func (r *recordingAnalyzer) ShadowGetSignals(_, _, _, _ string) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
}

func TestWithShadowSignalAnalysisInvokesShadowWithoutBlocking(t *testing.T) {
	t.Parallel()

	analyzer := &recordingAnalyzer{}
	cfg := config.Config{Mode: config.ModeShadow}
	handler := proxy.WithShadowSignalAnalysis(cfg, analyzer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"content":"HELLO"}]}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		analyzer.mu.Lock()
		calls := analyzer.calls
		analyzer.mu.Unlock()
		if calls == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("shadow analyzer calls = %d, want 1", calls)
		}
		time.Sleep(10 * time.Millisecond)
	}
}