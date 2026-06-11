package engine_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/engine"
	"github.com/hsar-org/hsar/internal/policy"
	"google.golang.org/grpc"
)

type policySignalServer struct {
	hsarv1.UnimplementedSignalServiceServer
}

func (s *policySignalServer) ProcessSignal(_ context.Context, req *hsarv1.SignalRequest) (*hsarv1.SignalFrame, error) {
	return &hsarv1.SignalFrame{
		TenantId:  req.TenantId,
		RequestId: req.RequestId,
		Abstain:   false,
		Signals:   []*hsarv1.Signal{{Name: "failure_risk", Value: 0.92}},
	}, nil
}

func startPolicySignalGRPC(t *testing.T) (addr string, stop func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := grpc.NewServer()
	hsarv1.RegisterSignalServiceServer(srv, &policySignalServer{})
	go srv.Serve(lis)

	return lis.Addr().String(), func() {
		srv.Stop()
		_ = lis.Close()
	}
}

func TestShadowGetSignalsEmitsPolicyTrace(t *testing.T) {
	t.Parallel()

	addr, stop := startPolicySignalGRPC(t)
	defer stop()

	path := filepath.Join("..", "..", "policies", "standard-safety-policy.yaml")
	p, err := policy.Load(path)
	if err != nil {
		t.Fatalf("Load policy: %v", err)
	}
	evaluator := policy.NewEvaluator(p)

	client, err := engine.NewClient(addr, evaluator)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	client.ShadowGetSignals("tenant-a", "req-policy-1", "conv-1", "angry text")

	deadline := time.Now().Add(2 * time.Second)
	for {
		out := buf.String()
		if strings.Contains(out, "policy_trace") &&
			strings.Contains(out, "standard-safety-policy") &&
			strings.Contains(out, "req-policy-1") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected policy_trace log, got: %s", out)
		}
		time.Sleep(10 * time.Millisecond)
	}
}