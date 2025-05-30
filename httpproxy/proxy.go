/*
MIT License

Copyright (c) 2025 Andrew McDermott

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package httpproxy provides a generic reverse proxy
// implementation that forwards HTTP requests to a specific upstream
// endpoint over a custom dialer (e.g., Unix domain socket or TCP). It
// strips hop-by-hop headers as defined in RFC 7230 §6.1 and any
// headers named in the Connection header, then relays all other
// headers and the request/response bodies transparently.
//
// A reverse proxy differs from a forward proxy in that it sits in
// front of backend servers and presents a unified interface to
// clients. Clients send requests to the proxy, which routes them to
// the real server (the "upstream") and returns the responses. This
// helps hide backend topology (e.g., Unix sockets) from clients.
package httpproxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// hopByHop lists HTTP header fields that a proxy or gateway must
// remove before forwarding a message. As per RFC 7230 §6.1, a proxy
// MUST strip any headers named in the Connection field and all
// standard hop-by-hop headers, regardless of whether they appear in
// Connection. These fields (case-insensitive) are:
var hopByHop = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

// Proxy forwards HTTP requests to an upstream endpoint over a custom
// dialer. It strips hop-by-hop headers and relays all other headers
// and body seamlessly. The Transport field is configured with
// sensible defaults but is public should you require changes once the
// proxy has been constructed.
type Proxy struct {
	// UpstreamBase is the base URL for upstream requests (e.g.
	// "http://unix"). The proxy appends the incoming request's
	// RequestURI to this base to construct the full upstream URL.
	UpstreamBase string

	// Transport performs the actual HTTP round trips to the
	// upstream server. It is configured with sensible defaults
	// for connection pooling and timeouts but can be modified
	// after proxy construction if needed.
	Transport http.RoundTripper

	// OnRequest, if non-nil, is called before forwarding each
	// request to the upstream server. It can be used for logging,
	// authentication, or modifying request headers.
	OnRequest func(*http.Request)

	// OnResponse, if non-nil, is called after receiving each
	// response from the upstream server but before forwarding it
	// to the client. It can be used for logging, metrics
	// collection, or modifying response headers.
	OnResponse func(*http.Response)

	// OnError, if non-nil, is called when an error occurs during
	// request processing (e.g., io.Copy failures). It can be used
	// for logging or error handling.
	OnError func(error)
}

// newProxy constructs a Proxy that dials via dialContext and forwards
// to upstream (scheme://host). For Unix sockets, upstream is
// "http://unix".
func newProxy(dialContext func(ctx context.Context, network, addr string) (net.Conn, error), upstream string) *Proxy {
	transport := &http.Transport{
		DialContext: dialContext,
		// Allow HTTP/1.1 keep-alive.
		DisableKeepAlives:   false,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     1000,
		IdleConnTimeout:     90 * time.Second,
		// Fail fast if the server doesn't respond.
		ResponseHeaderTimeout: 5 * time.Second,
	}

	return &Proxy{
		UpstreamBase: upstream,
		Transport:    transport,
	}
}

// copyHeaders strips standard and Connection-named hop-by-hop
// headers, then copies the rest.
func copyHeaders(dst, src http.Header) {
	// Dynamic set from Connection header.
	dynamic := map[string]bool{}

	if conns, ok := src["Connection"]; ok {
		for _, v := range conns {
			for _, tok := range strings.Split(v, ",") {
				t := strings.ToLower(strings.TrimSpace(tok))
				if t != "" {
					dynamic[t] = true
				}
			}
		}
	}

	for k, vs := range src {
		key := strings.ToLower(k)
		if hopByHop[key] || dynamic[key] {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// ServeHTTP implements http.Handler: it forwards r to the upstream
// and relays response headers/body back to w.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upstreamURL := p.UpstreamBase + r.URL.RequestURI()
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("request creation failed: %v", err), http.StatusInternalServerError)
		return
	}

	copyHeaders(req.Header, r.Header)

	if p.OnRequest != nil {
		p.OnRequest(req)
	}

	resp, err := p.Transport.RoundTrip(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if p.OnResponse != nil {
		p.OnResponse(resp)
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil && p.OnError != nil {
		p.OnError(err)
	}
}

// Option is a functional option for configuring a Proxy.
type Option func(*Proxy)

// WithOnRequest sets a callback that is called before each request is forwarded.
func WithOnRequest(fn func(*http.Request)) Option {
	return func(p *Proxy) {
		p.OnRequest = fn
	}
}

// WithOnResponse sets a callback that is called after each response is received.
func WithOnResponse(fn func(*http.Response)) Option {
	return func(p *Proxy) {
		p.OnResponse = fn
	}
}

// WithOnError sets a callback that is called when an error occurs during request processing.
func WithOnError(fn func(error)) Option {
	return func(p *Proxy) {
		p.OnError = fn
	}
}

// NewUnix creates a reverse proxy that dials a Unix-domain socket at
// socketPath. The upstreamBase must be a valid URL with a hostname
// (for example "http://unix") so that http.NewRequest can parse a
// scheme and authority (i.e., a host). The custom DialContext ignores
// the hostname and always connects over the UDS path.
//
// Example:
//
// This will translate an incoming GET /metrics into an outgoing
// request to "http://unix/metrics", then DialContext will open
// the socket at socketPath.
//
//	proxy := httpproxy.NewUnix("/var/run/app.sock", "http://unix",
//		httpproxy.WithOnError(func(err error) { log.Error(err, "proxy error") }),
//	)
func NewUnix(socketPath, upstream string, opts ...Option) *Proxy {
	proxy := newProxy(
		func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
		upstream,
	)

	for _, opt := range opts {
		opt(proxy)
	}

	return proxy
}

// NewTCP creates a reverse proxy that dials a TCP backend at addr
// (e.g. "host:port"). The upstreamBase must include a valid URL
// scheme and authority (for example "http://host:port") so that
// http.NewRequest can build a proper URL. The custom DialContext will
// use the network and addr you provided.
//
// Example:
//
//	// Incoming GET /foo becomes an outgoing request to
//	// "http://backend:9090/foo" over TCP.
//	proxy := httpproxy.NewTCP("backend:9090", "http://backend:9090")
func NewTCP(addr, upstream string, opts ...Option) *Proxy {
	proxy := newProxy(
		func(_ context.Context, network, _ string) (net.Conn, error) {
			return net.Dial(network, addr)
		},
		upstream,
	)

	for _, opt := range opts {
		opt(proxy)
	}

	return proxy
}
