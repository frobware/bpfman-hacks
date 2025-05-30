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

package httpproxy_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/frobware/bpfman-hacks/httpproxy"
)

var (
	udsTempDir   string
	udsTestSock  string
	udsTestProxy *httpproxy.Proxy
)

// TestMain sets up a single UDS server for TestServeConcurrency,
// ensuring it stays alive for parallel tests, then tears it down
// after m.Run(). If we closed the server too early (in init() or at
// the end of a parent test), any t.Parallel() subtests could run
// after the server is gone and start failing with 502s.
//
// Steps:
//  1. Create a fresh temp dir for the socket to avoid collisions.
//  2. Compute udsTestSock inside that dir.
//  3. Listen on the UDS path and start an httptest.Server on it.
//  4. Build udsTestProxy exactly as you would in production.
//  5. Call m.Run() to execute all tests (including parallel subtests).
//  6. After tests finish, close the server and remove the temp dir.
func TestMain(m *testing.M) {
	var err error
	udsTempDir, err = os.MkdirTemp("", "httpproxy-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir temp: %v\n", err)
		os.Exit(1)
	}

	udsTestSock = filepath.Join(udsTempDir, "test.sock")

	lis, err := net.Listen("unix", udsTestSock)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen unix: %v\n", err)
		os.Exit(1)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "pong")
	}))
	srv.Listener = lis
	srv.Start()

	udsTestProxy = httpproxy.NewUnix(udsTestSock, "http://unix")

	code := m.Run()
	srv.Close()
	os.RemoveAll(udsTempDir)
	os.Exit(code)
}

// TestCopyHeaders verifies that copyHeaders strips hop-by-hop and Connection-named
// headers.
func TestCopyHeaders(t *testing.T) {
	src := http.Header{
		"Connection":        {"Keep-Alive, X-Custom-Hop"},
		"Keep-Alive":        {"timeout=5"},
		"X-Custom-Hop":      {"foo"},
		"Transfer-Encoding": {"chunked"},
		"X-Preserved":       {"bar"},
	}
	dst := http.Header{}
	httpproxy.CopyHeaders(dst, src)

	if dst.Get("X-Preserved") != "bar" {
		t.Errorf("expected X-Preserved preserved, got %q", dst.Get("X-Preserved"))
	}
	if dst.Get("Keep-Alive") != "" {
		t.Error("expected Keep-Alive stripped")
	}
	if dst.Get("X-Custom-Hop") != "" {
		t.Error("expected X-Custom-Hop stripped")
	}
}

// TestServeHTTP_TCP exercises the proxy over a TCP httptest.Server.
func TestServeHTTP_TCP(t *testing.T) {
	// Backend returns the path and echo header.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/foo" {
			t.Errorf("expected path /foo, got %s", r.URL.Path)
		}
		w.Header().Set("X-Test", "value")
		fmt.Fprint(w, "tcp-OK")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/foo?x=1", nil)
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "tcp-OK" {
		t.Errorf("unexpected body %q", rr.Body.String())
	}
	if rr.Header().Get("X-Test") != "value" {
		t.Error("expected X-Test header forwarded")
	}
}

// TestServeHTTP_UDS exercises the proxy over a Unix domain socket.
func TestServeHTTP_UDS(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("failed to listen on unix: %v", err)
	}
	defer lis.Close()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		fmt.Fprint(w, "udsOK")
	}))
	srv.Listener = lis
	srv.Start()
	defer srv.Close()

	px := httpproxy.NewUnix(sock, "http://unix")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/anything", nil)
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "udsOK" {
		t.Errorf("unexpected body %q", rr.Body.String())
	}
	if rr.Header().Get("Content-Length") != "5" {
		t.Error("expected Content-Length preserved")
	}
}

// TestServeHTTP_Concurrency tests the proxy's ability to handle
// multiple concurrent requests safely. It validates that the proxy
// can serve many simultaneous requests without data races, connection
// leaks, or response corruption. The test runs with escalating
// connection counts to verify both basic functionality and high-load
// scenarios. Each subtest runs in parallel to stress-test the proxy's
// thread safety and connection pooling behaviour.
func TestServeHTTP_Concurrency(t *testing.T) {
	for _, tc := range []int{100, 200, 500, 5000, 10000} {
		tc := tc
		t.Run(fmt.Sprintf("%d-conns", tc), func(t *testing.T) {
			t.Parallel()

			var wg sync.WaitGroup
			errs := make(chan error, tc)

			// Launch tc concurrent goroutines, each making one request.
			for i := range tc {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					rr := httptest.NewRecorder()
					req := httptest.NewRequest("GET", "/ping", nil)
					udsTestProxy.ServeHTTP(rr, req)

					body := rr.Body.String()
					if rr.Code != http.StatusOK {
						errs <- fmt.Errorf(
							"worker %d: status %d; proxy-body=%q",
							id, rr.Code, body,
						)
						return
					}
					if body != "pong" {
						errs <- fmt.Errorf(
							"worker %d: unexpected body %q",
							id, body,
						)
					}
				}(i)
			}

			// Wait for all requests to complete and check for errors.
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
		})
	}
}

// TestServeHTTP_DialFailure tests behaviour when the upstream connection
// fails.
func TestServeHTTP_DialFailure(t *testing.T) {
	px := httpproxy.NewUnix("/nonexistent/socket", "http://unix")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "proxy error") {
		t.Error("expected proxy error message in response body")
	}
}

// TestServeHTTP_TCPDialFailure tests TCP connection failure.
func TestServeHTTP_TCPDialFailure(t *testing.T) {
	px := httpproxy.NewTCP("127.0.0.1:0", "http://127.0.0.1:0")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway, got %d", rr.Code)
	}
}

// TestServeHTTP_UpstreamError tests when the upstream returns an error
// status.
func TestServeHTTP_UpstreamError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/error", nil)
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "internal server error") {
		t.Error("expected upstream error message forwarded")
	}
}

// TestServeHTTP_RequestCreationFailure tests when http.NewRequestWithContext fails
// due to malformed URL construction.
func TestServeHTTP_RequestCreationFailure(t *testing.T) {
	// Create a proxy with an invalid upstream base that will cause URL construction to fail
	px := &httpproxy.Proxy{
		UpstreamBase: "://invalid-scheme", // Invalid URL scheme
		Transport:    http.DefaultTransport,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "request creation failed") {
		t.Error("expected request creation failed message in response body")
	}
}

// TestServeHTTP_OnRequestHook tests the OnRequest hook functionality.
func TestServeHTTP_OnRequestHook(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hook-Added") != "test-value" {
			t.Error("expected X-Hook-Added header from OnRequest hook")
		}
		fmt.Fprint(w, "hook-OK")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	var hookCalled int32
	px := httpproxy.NewTCP(addr, "http://"+addr,
		httpproxy.WithOnRequest(func(req *http.Request) {
			atomic.AddInt32(&hookCalled, 1)
			req.Header.Set("X-Hook-Added", "test-value")
		}),
	)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/hook-test", nil)
	px.ServeHTTP(rr, req)

	if atomic.LoadInt32(&hookCalled) != 1 {
		t.Errorf("expected OnRequest hook called once, got %d", hookCalled)
	}
	if rr.Body.String() != "hook-OK" {
		t.Errorf("unexpected response body %q", rr.Body.String())
	}
}

// TestServeHTTP_OnResponseHook tests the OnResponse hook
// functionality.
func TestServeHTTP_OnResponseHook(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Header", "backend-value")
		fmt.Fprint(w, "response-OK")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	var hookCalled int32
	var capturedStatus int
	px := httpproxy.NewTCP(addr, "http://"+addr,
		httpproxy.WithOnResponse(func(resp *http.Response) {
			atomic.AddInt32(&hookCalled, 1)
			capturedStatus = resp.StatusCode
			resp.Header.Set("X-Hook-Modified", "modified-value")
		}),
	)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/response-test", nil)
	px.ServeHTTP(rr, req)

	if atomic.LoadInt32(&hookCalled) != 1 {
		t.Errorf("expected OnResponse hook called once, got %d", hookCalled)
	}
	if capturedStatus != http.StatusOK {
		t.Errorf("expected hook to capture status 200, got %d", capturedStatus)
	}
	if rr.Header().Get("X-Hook-Modified") != "modified-value" {
		t.Error("expected X-Hook-Modified header from OnResponse hook")
	}
	if rr.Header().Get("X-Backend-Header") != "backend-value" {
		t.Error("expected X-Backend-Header preserved from backend")
	}
}

// TestServeHTTP_LargeRequestBody tests the handling of large request bodies.
func TestServeHTTP_LargeRequestBody(t *testing.T) {
	const bodySize = 100 * 1024 * 1024 // 100MB
	expectedBody := bytes.Repeat([]byte("A"), bodySize)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("backend failed to read body: %v", err)
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		if len(receivedBody) != bodySize {
			t.Errorf("expected body size %d, got %d", bodySize, len(receivedBody))
		}
		if !bytes.Equal(receivedBody, expectedBody) {
			t.Error("received body doesn't match expected body")
		}
		fmt.Fprint(w, "large-body-OK")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/large", bytes.NewReader(expectedBody))
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "large-body-OK" {
		t.Errorf("unexpected response body %q", rr.Body.String())
	}
}

// TestServeHTTP_LargeResponseBody tests the handling of large response
// bodies.
func TestServeHTTP_LargeResponseBody(t *testing.T) {
	const bodySize = 200 * 1024 * 1024 // 200MB
	expectedBody := bytes.Repeat([]byte("B"), bodySize)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", bodySize))
		if _, err := w.Write(expectedBody); err != nil {
			panic(err) // Test server should never fail to write.
		}
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/large-response", nil)
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if len(rr.Body.Bytes()) != bodySize {
		t.Errorf("expected response body size %d, got %d", bodySize, len(rr.Body.Bytes()))
	}
	if !bytes.Equal(rr.Body.Bytes(), expectedBody) {
		t.Error("response body doesn't match expected body")
	}
}

// TestCopyHeaders_MalformedConnection tests edge cases in Connection
// header parsing.
func TestCopyHeaders_MalformedConnection(t *testing.T) {
	tests := []struct {
		name string
		src  http.Header
		want map[string]string
	}{
		{
			name: "empty connection value",
			src: http.Header{
				"Connection": {""},
				"X-Test":     {"should-be-preserved"},
			},
			want: map[string]string{
				"X-Test": "should-be-preserved",
			},
		},
		{
			name: "connection with spaces and commas",
			src: http.Header{
				"Connection":  {" Keep-Alive , X-Custom , "},
				"Keep-Alive":  {"should-be-stripped"},
				"X-Custom":    {"should-be-stripped"},
				"X-Preserved": {"should-be-preserved"},
			},
			want: map[string]string{
				"X-Preserved": "should-be-preserved",
			},
		},
		{
			name: "multiple connection headers",
			src: http.Header{
				"Connection": {"Keep-Alive", "X-Remove"},
				"Keep-Alive": {"stripped"},
				"X-Remove":   {"stripped"},
				"X-Keep":     {"preserved"},
			},
			want: map[string]string{
				"X-Keep": "preserved",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := http.Header{}
			httpproxy.CopyHeaders(dst, tt.src)

			for key, expectedValue := range tt.want {
				if got := dst.Get(key); got != expectedValue {
					t.Errorf("expected %s=%s, got %s", key, expectedValue, got)
				}
			}

			// Verify that stripped headers are actually gone.
			for key := range tt.src {
				if _, shouldExist := tt.want[key]; !shouldExist && dst.Get(key) != "" {
					t.Errorf("header %s should have been stripped but found: %s", key, dst.Get(key))
				}
			}
		})
	}
}

// TestServeHTTP_MultipleHeaderValues tests the handling of headers with
// multiple values.
func TestServeHTTP_MultipleHeaderValues(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that multiple X-Test values were preserved.
		values := r.Header["X-Test"]
		if len(values) != 2 || values[0] != "value1" || values[1] != "value2" {
			t.Errorf("expected X-Test=[value1, value2], got %v", values)
		}

		// Echo back multiple values.
		w.Header()["X-Response"] = []string{"resp1", "resp2"}
		fmt.Fprint(w, "multi-header-OK")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/multi-header", nil)
	req.Header["X-Test"] = []string{"value1", "value2"}
	px.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	responseValues := rr.Header()["X-Response"]
	if len(responseValues) != 2 || responseValues[0] != "resp1" || responseValues[1] != "resp2" {
		t.Errorf("expected X-Response=[resp1, resp2], got %v", responseValues)
	}
}

// BenchmarkProxy_TCP benchmarks TCP proxy performance.
func BenchmarkProxy_TCP(b *testing.B) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "bench")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/bench", nil)
			px.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				b.Errorf("unexpected status %d", rr.Code)
			}
		}
	})
}

// BenchmarkProxy_UDS benchmarks Unix domain socket proxy performance.
func BenchmarkProxy_UDS(b *testing.B) {
	sock := filepath.Join(b.TempDir(), "bench.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		b.Fatalf("failed to listen on unix: %v", err)
	}
	defer lis.Close()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "bench")
	}))
	srv.Listener = lis
	srv.Start()
	defer srv.Close()

	px := httpproxy.NewUnix(sock, "http://unix")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/bench", nil)
			px.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				b.Errorf("unexpected status %d", rr.Code)
			}
		}
	})
}

// BenchmarkCopyHeaders benchmarks header copying performance.
func BenchmarkCopyHeaders(b *testing.B) {
	src := http.Header{
		"Content-Type":    {"application/json"},
		"Content-Length":  {"1234"},
		"Authorization":   {"Bearer token"},
		"X-Request-ID":    {"req-123"},
		"X-Forwarded-For": {"192.168.1.1"},
		"User-Agent":      {"test/1.0"},
		"Accept":          {"application/json"},
		"Cache-Control":   {"no-cache"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst := http.Header{}
		httpproxy.CopyHeaders(dst, src)
	}
}

// BenchmarkCopyHeaders_WithConnectionHeader benchmarks header copying
// with Connection header parsing.
func BenchmarkCopyHeaders_WithConnectionHeader(b *testing.B) {
	src := http.Header{
		"Connection":      {"Keep-Alive, X-Custom-Header"},
		"Keep-Alive":      {"timeout=5"},
		"X-Custom-Header": {"value"},
		"Content-Type":    {"application/json"},
		"Content-Length":  {"1234"},
		"Authorization":   {"Bearer token"},
		"X-Request-ID":    {"req-123"},
		"X-Forwarded-For": {"192.168.1.1"},
		"User-Agent":      {"test/1.0"},
		"Accept":          {"application/json"},
		"Cache-Control":   {"no-cache"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst := http.Header{}
		httpproxy.CopyHeaders(dst, src)
	}
}

// failingResponseWriter is a mock http.ResponseWriter that fails on Write
// to simulate io.Copy errors.
type failingResponseWriter struct {
	header http.Header
	code   int
}

func (f *failingResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *failingResponseWriter) WriteHeader(code int) {
	f.code = code
}

func (f *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated write failure")
}

// TestOnError tests that the OnError hook is called when io.Copy
// fails.
func TestOnError(t *testing.T) {
	var capturedError error
	var errorCallCount int

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "test response body")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr,
		httpproxy.WithOnError(func(err error) {
			capturedError = err
			errorCallCount++
		}),
	)

	failingWriter := &failingResponseWriter{}
	req := httptest.NewRequest("GET", "/test", nil)

	px.ServeHTTP(failingWriter, req)

	if errorCallCount != 1 {
		t.Errorf("expected OnError to be called 1 time, got %d", errorCallCount)
	}

	if capturedError == nil {
		t.Error("expected OnError to capture an error, got nil")
	} else if capturedError.Error() != "simulated write failure" {
		t.Errorf("expected error \"simulated write failure\", got \"%s\"", capturedError.Error())
	}

	if failingWriter.code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, failingWriter.code)
	}

	if got := failingWriter.Header().Get("Content-Type"); got != "text/plain" {
		t.Errorf("expected Content-Type \"text/plain\", got \"%s\"", got)
	}
}

// TestOnError_NotCalled tests that OnError is not called when no
// error occurs.
func TestOnError_NotCalled(t *testing.T) {
	var errorCallCount int

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "success")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr,
		httpproxy.WithOnError(func(err error) {
			errorCallCount++
		}),
	)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)

	px.ServeHTTP(rr, req)

	if errorCallCount != 0 {
		t.Errorf("expected OnError to not be called, but it was called %d times", errorCallCount)
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if body := rr.Body.String(); body != "success" {
		t.Errorf("expected body \"success\", got \"%s\"", body)
	}
}

// TestOnError_Nil tests that nil OnError hook does not cause panic.
func TestOnError_Nil(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "test")
	}))
	defer backend.Close()

	addr := backend.Listener.Addr().String()
	px := httpproxy.NewTCP(addr, "http://"+addr)

	failingWriter := &failingResponseWriter{}
	req := httptest.NewRequest("GET", "/test", nil)

	px.ServeHTTP(failingWriter, req)

	if failingWriter.code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, failingWriter.code)
	}
}

// TestWithOptions tests the WithXXX functional options pattern for
// both TCP and Unix proxies.
func TestWithOptions(t *testing.T) {
	tests := []struct {
		name       string
		setupProxy func(t *testing.T) *httpproxy.Proxy
	}{
		{
			name: "TCP",
			setupProxy: func(t *testing.T) *httpproxy.Proxy {
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, "options-test")
				}))
				t.Cleanup(backend.Close)

				addr := backend.Listener.Addr().String()
				return httpproxy.NewTCP(addr, "http://"+addr)
			},
		},
		{
			name: "Unix",
			setupProxy: func(t *testing.T) *httpproxy.Proxy {
				sock := filepath.Join(t.TempDir(), "test.sock")
				lis, err := net.Listen("unix", sock)
				if err != nil {
					t.Fatalf("failed to listen on unix: %v", err)
				}
				t.Cleanup(func() { lis.Close() })

				srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, "options-test")
				}))
				srv.Listener = lis
				srv.Start()
				t.Cleanup(srv.Close)

				// Use functional options to ensure NewUnixProxy options loop is covered
				return httpproxy.NewUnix(sock, "http://unix",
					httpproxy.WithOnRequest(func(req *http.Request) {}),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var onRequestCalled, onResponseCalled, onErrorCalled bool
			var capturedRequest *http.Request
			var capturedResponse *http.Response
			var capturedError error

			px := tt.setupProxy(t)
			px.OnRequest = func(req *http.Request) {
				onRequestCalled = true
				capturedRequest = req
			}
			px.OnResponse = func(resp *http.Response) {
				onResponseCalled = true
				capturedResponse = resp
			}
			px.OnError = func(err error) {
				onErrorCalled = true
				capturedError = err
			}

			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/options", nil)
			px.ServeHTTP(rr, req)

			if !onRequestCalled {
				t.Error("expected OnRequest to be called")
			}
			if capturedRequest == nil {
				t.Error("expected OnRequest to capture request")
			}

			if !onResponseCalled {
				t.Error("expected OnResponse to be called")
			}
			if capturedResponse == nil {
				t.Error("expected OnResponse to capture response")
			}

			if onErrorCalled {
				t.Error("expected OnError not to be called")
			}
			if capturedError != nil {
				t.Errorf("expected no error, got %v", capturedError)
			}

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
			}
			if body := rr.Body.String(); body != "options-test" {
				t.Errorf("expected body \"options-test\", got \"%s\"", body)
			}
		})
	}

	t.Run("functional_options", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "functional-options-test")
		}))
		defer backend.Close()

		addr := backend.Listener.Addr().String()
		var onRequestCalled, onResponseCalled, onErrorCalled bool

		px := httpproxy.NewTCP(addr, "http://"+addr,
			httpproxy.WithOnRequest(func(req *http.Request) {
				onRequestCalled = true
			}),
			httpproxy.WithOnResponse(func(resp *http.Response) {
				onResponseCalled = true
			}),
			httpproxy.WithOnError(func(err error) {
				onErrorCalled = true
			}),
		)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/functional-options", nil)
		px.ServeHTTP(rr, req)

		if !onRequestCalled {
			t.Error("expected OnRequest to be called")
		}
		if !onResponseCalled {
			t.Error("expected OnResponse to be called")
		}
		if onErrorCalled {
			t.Error("expected OnError not to be called")
		}
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})
}
