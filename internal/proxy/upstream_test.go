package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hsar-org/hsar/internal/proxy"
)

func TestUpstreamReturns502WhenUnreachable(t *testing.T) {
	u, err := proxy.NewUpstream("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[]}`))
	u.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestUpstreamNonStreamingPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	}))
	defer upstream.Close()

	u, err := proxy.NewUpstream(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	proxySrv := httptest.NewServer(u)
	defer proxySrv.Close()

	resp, err := http.Post(proxySrv.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello") {
		t.Fatalf("body = %s", body)
	}
}

func TestUpstreamStreamingFirstByteBeforeBodyComplete(t *testing.T) {
	var secondChunk atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		_, _ = w.Write([]byte("data: {\"chunk\":1}\n\n"))
		flusher.Flush()

		time.Sleep(400 * time.Millisecond)
		secondChunk.Store(true)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	u, err := proxy.NewUpstream(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxySrv := httptest.NewServer(u)
	defer proxySrv.Close()

	start := time.Now()
	resp, err := http.Post(
		proxySrv.URL+"/v1/chat/completions",
		"application/json",
		strings.NewReader(`{"stream":true,"messages":[]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	firstByte := time.Since(start)

	if n == 0 {
		t.Fatal("expected first chunk bytes")
	}
	if firstByte > 250*time.Millisecond {
		t.Fatalf("first byte took %v; proxy likely buffered full body", firstByte)
	}
	if secondChunk.Load() {
		t.Fatal("second chunk should not have been sent before first read")
	}

	rest, _ := io.ReadAll(resp.Body)
	all := string(buf[:n]) + string(rest)
	if !strings.Contains(all, "chunk") {
		t.Fatalf("body = %q", all)
	}
}