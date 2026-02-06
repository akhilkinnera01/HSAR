package engine

import (
	"context"
	"log/slog"
	"os"
	"time"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	target string
	conn   *grpc.ClientConn
	stub   hsarv1.SignalServiceClient
}

func NewClientFromEnv() (*Client, error) {
	target := os.Getenv("SIGNAL_ENGINE_TARGET")
	if target == "" {
		target = "signal-engine:50051"
	}

	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	return &Client{
		target: target,
		conn:   conn,
		stub:   hsarv1.NewSignalServiceClient(conn),
	}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// ShadowGetSignals never blocks the request path.
// It should be called in a goroutine.
func (c *Client) ShadowGetSignals(tenantID, requestID, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	req := &hsarv1.SignalRequest{
		TenantId:    tenantID,
		RequestId:   requestID,
		TextPayload: text,
	}

	sf, err := c.stub.ProcessSignal(ctx, req)
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
}
