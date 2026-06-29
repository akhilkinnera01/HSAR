package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

type chatRequest struct {
	Stream bool `json:"stream"`
}

type Upstream struct {
	base    *url.URL
	client  *http.Client
}

func NewUpstream(baseURL string) (*Upstream, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return &Upstream{
		base: u,
		client: &http.Client{
			Timeout: 0, // streaming uses request context
		},
	}, nil
}

func (u *Upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	streaming := wantsStream(body)

	upReq, err := u.buildUpstreamRequest(r, body)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	resp, err := u.client.Do(upReq.WithContext(r.Context()))
	if err != nil {
		slog.Warn("upstream_request_failed", "error", err, "trace_id", r.Header.Get("X-Request-ID"))
		http.Error(w, "Bad Gateway: Upstream Unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)

	if streaming {
		if err := streamResponse(w, r, resp); err != nil {
			slog.Warn("upstream_stream_failed", "error", err, "trace_id", r.Header.Get("X-Request-ID"))
		}
		return
	}

	_, _ = io.Copy(w, resp.Body)
}

func (u *Upstream) buildUpstreamRequest(r *http.Request, body []byte) (*http.Request, error) {
	target := *u.base
	target.Path = strings.TrimSuffix(target.Path, "/") + "/v1/chat/completions"

	upReq, err := http.NewRequest(http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	upReq.Header.Set("Content-Type", "application/json")
	if accept := r.Header.Get("Accept"); accept != "" {
		upReq.Header.Set("Accept", accept)
	}
	return upReq, nil
}

func wantsStream(body []byte) bool {
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	return req.Stream
}

func copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	for k, vals := range resp.Header {
		if isHopByHopHeader(k) {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
}

func isHopByHopHeader(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailers", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

// FlushWriter ensures periodic flush during streaming copy.
type flushWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if err == nil && n > 0 && fw.flusher != nil {
		fw.flusher.Flush()
	}
	return n, err
}