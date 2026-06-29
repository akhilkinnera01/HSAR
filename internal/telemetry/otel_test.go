package telemetry

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

var forbiddenSpanKeys = []string{
	"gen_ai.prompt", "gen_ai.completion", "message.content", "text_payload", "messages",
}

func TestSpanAttrsAllowlist(t *testing.T) {
	attrs := SpanAttrs("req-1", "tenant-a", "enforce", true, false, true, "budget", "PASSTHROUGH", false)
	attrs = append(attrs, attribute.Int("http.status_code", 200))

	seen := map[string]bool{}
	for _, a := range attrs {
		seen[string(a.Key)] = true
		for _, forbidden := range forbiddenSpanKeys {
			if strings.EqualFold(string(a.Key), forbidden) {
				t.Errorf("forbidden span attribute %s", a.Key)
			}
		}
	}
	for _, required := range []string{"hsar.trace_id", "hsar.tenant_id", "hsar.mode", "hsar.inline", "hsar.fail_open"} {
		if !seen[required] {
			t.Errorf("missing required attribute %s", required)
		}
	}
}

func TestFailOpenReasonSpanOnlyWhenFailOpen(t *testing.T) {
	attrs := SpanAttrs("req-1", "tenant-a", "enforce", true, false, false, "", "", false)
	for _, a := range attrs {
		if string(a.Key) == "hsar.fail_open_reason" {
			t.Fatal("fail_open_reason must not be set when fail_open is false")
		}
	}
}