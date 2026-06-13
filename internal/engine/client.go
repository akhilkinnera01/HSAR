package engine

import (
	"context"
	"log/slog"
	"os"
	"time"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/policy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	target    string
	conn      *grpc.ClientConn
	stub      hsarv1.SignalServiceClient
	evaluator *policy.Evaluator
}

func NewClientFromEnv(evaluator *policy.Evaluator) (*Client, error) {
	target := os.Getenv("SIGNAL_ENGINE_TARGET")
	if target == "" {
		target = "signal-engine:50051"
	}
	return NewClient(target, evaluator)
}

func NewClient(target string, evaluator *policy.Evaluator) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		target:    target,
		conn:      conn,
		stub:      hsarv1.NewSignalServiceClient(conn),
		evaluator: evaluator,
	}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// ShadowGetSignals: never blocks request path. Safe to run in a goroutine.
func (c *Client) ShadowGetSignals(tenantID, requestID, conversationID, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := &hsarv1.SignalRequest{
		TenantId:    tenantID,
		RequestId:   requestID,
		TextPayload: text, // Corrected field name
		// TsRequestMs: removed as it is not in the proto
	}

	sf, err := c.stub.ProcessSignal(ctx, req) // Corrected method name
	if err != nil {
		slog.Warn("signal_engine_call_failed", "target", c.target, "trace_id", requestID, "error", err)
		return
	}

	slog.Info("signal_engine_signalframe",
		"trace_id", requestID,
		"tier", sf.Tier.String(),
		"abstain", sf.Abstain,
		"confidence", sf.Confidence,
		"latency_ms", sf.ProcessingLatencyMs,
	)

	if c.evaluator == nil {
		return
	}

	decision := c.evaluator.Evaluate(tenantID, conversationID, sf)
	trace := policy.BuildTrace(tenantID, requestID, c.evaluator.Policy, decision)
	policy.LogTrace(trace, false)
}

// InlineGetSignals runs synchronous perception within the given budget.
func (c *Client) InlineGetSignals(ctx context.Context, tenantID, requestID, text string) (*hsarv1.SignalFrame, error) {
	if c == nil {
		return nil, context.DeadlineExceeded
	}

	req := &hsarv1.SignalRequest{
		TenantId:    tenantID,
		RequestId:   requestID,
		TextPayload: text,
	}

	sf, err := c.stub.ProcessSignal(ctx, req)
	if err != nil {
		return nil, err
	}

	slog.Info("signal_engine_signalframe",
		"trace_id", requestID,
		"tier", sf.Tier.String(),
		"abstain", sf.Abstain,
		"confidence", sf.Confidence,
		"latency_ms", sf.ProcessingLatencyMs,
		"inline", true,
	)
	return sf, nil
}

// Evaluator returns the policy evaluator attached to this client.
func (c *Client) Evaluator() *policy.Evaluator {
	if c == nil {
		return nil
	}
	return c.evaluator
}
