// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lhttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func httpReqGen(method, url string, body []byte) RequestGen {
	return func() (*http.Request, error) {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		return http.NewRequest("GET", url, bodyReader)
	}
}

func TestNewRequestGET(t *testing.T) {
	ftt.Run(`HTTP GET requests should be handled correctly.`, t, func(c *ftt.Test) {
		ctx := context.Background()

		// First call returns HTTP 500, second succeeds.
		serverCalls := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serverCalls++
			content, err := io.ReadAll(r.Body)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, content, should.Match([]byte{}))
			if serverCalls == 1 {
				w.WriteHeader(500)
			} else {
				fmt.Fprintf(w, "Hello, client\n")
			}
		}))
		defer ts.Close()

		httpReq := httpReqGen("GET", ts.URL, nil)

		clientCalls := 0
		clientReq := NewRequest(ctx, http.DefaultClient, fast, httpReq, func(resp *http.Response) error {
			clientCalls++
			content, err := io.ReadAll(resp.Body)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, string(content), should.Match("Hello, client\n"))
			assert.Loosely(c, resp.Body.Close(), should.BeNil)
			return nil
		}, nil)

		status, err := clientReq()
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, status, should.Match(200))
		assert.Loosely(c, serverCalls, should.Match(2))
		assert.Loosely(c, clientCalls, should.Match(1))
	})
}

func TestNewRequestPOST(t *testing.T) {
	ftt.Run(`HTTP POST requests should be handled correctly.`, t, func(c *ftt.Test) {
		ctx := context.Background()

		// First call returns HTTP 500, second succeeds.
		serverCalls := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serverCalls++
			content, err := io.ReadAll(r.Body)
			assert.Loosely(c, err, should.BeNil)
			// The same data is sent twice.
			assert.Loosely(c, string(content), should.Match("foo bar"))
			if serverCalls == 1 {
				w.WriteHeader(500)
			} else {
				fmt.Fprintf(w, "Hello, client\n")
			}
		}))
		defer ts.Close()

		httpReq := httpReqGen("POST", ts.URL, []byte("foo bar"))

		clientCalls := 0
		clientReq := NewRequest(ctx, http.DefaultClient, fast, httpReq, func(resp *http.Response) error {
			clientCalls++
			content, err := io.ReadAll(resp.Body)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, string(content), should.Match("Hello, client\n"))
			assert.Loosely(c, resp.Body.Close(), should.BeNil)
			return nil
		}, nil)

		status, err := clientReq()
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, status, should.Match(200))
		assert.Loosely(c, serverCalls, should.Match(2))
		assert.Loosely(c, clientCalls, should.Match(1))
	})
}

func TestNewRequestGETFail(t *testing.T) {
	ftt.Run(`HTTP GET requests should handle failure successfully.`, t, func(t *ftt.Test) {
		ctx := context.Background()

		serverCalls := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serverCalls++
			w.WriteHeader(500)
		}))
		defer ts.Close()

		httpReq := httpReqGen("GET", ts.URL, nil)

		clientReq := NewRequest(ctx, http.DefaultClient, fast, httpReq, func(resp *http.Response) error {
			t.Fail()
			return nil
		}, nil)

		status, err := clientReq()
		assert.Loosely(t, err.Error(), should.Match("gave up after 4 attempts: http request failed: Internal Server Error (HTTP 500)"))
		assert.Loosely(t, status, should.Match(500))
	})
}

func TestNewRequestDefaultFactory(t *testing.T) {
	// Test that the default factory (rFn == nil) only retries for transient
	// HTTP errors.
	testCases := []struct {
		statusCode int    // The status code to return (the first 2 times).
		path       string // Request path, if any.
		wantErr    bool   // Whether we want NewRequest to return an error.
		wantCalls  int    // The total number of HTTP requests expected.
	}{
		// 200, passes immediately.
		{statusCode: 200, wantErr: false, wantCalls: 1},
		// Transient HTTP error codes that will retry.
		{statusCode: 408, wantErr: false, wantCalls: 3},
		{statusCode: 500, wantErr: false, wantCalls: 3},
		{statusCode: 503, wantErr: false, wantCalls: 3},
		// Immediate failure codes.
		{statusCode: 403, wantErr: true, wantCalls: 1},
		{statusCode: 404, wantErr: true, wantCalls: 1},
	}

	ctx := context.Background()

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Status code %d, path %q", tc.statusCode, tc.path), func(t *testing.T) {
			t.Parallel()
			serverCalls := 0
			ts := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer r.Body.Close()
					serverCalls++
					if serverCalls <= 2 {
						w.WriteHeader(tc.statusCode)
					}
					fmt.Fprintf(w, "Hello World!\n")
				}))
			defer ts.Close()

			httpReq := httpReqGen("GET", ts.URL+tc.path, nil)
			req := NewRequest(ctx, http.DefaultClient, nil, httpReq, func(resp *http.Response) error {
				return resp.Body.Close()
			}, nil)

			_, err := req()
			if err == nil && tc.wantErr {
				t.Error("req returned nil error, wanted an error")
			} else if err != nil && !tc.wantErr {
				t.Errorf("req returned err %v, wanted nil", err)
			}
			if got, want := serverCalls, tc.wantCalls; got != want {
				t.Errorf("total server calls; got %d, want %d", got, want)
			}
		})
	}
}

func TestNewRequestClosesBody(t *testing.T) {
	ctx := context.Background()
	serverCalls := 0

	// Return a 500 for the first 2 requests.
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			serverCalls++
			if serverCalls <= 2 {
				w.WriteHeader(500)
			}
			fmt.Fprintf(w, "Hello World!\n")
		}))
	defer ts.Close()

	rt := &trackingRoundTripper{RoundTripper: http.DefaultTransport}
	hc := &http.Client{Transport: rt}
	httpReq := httpReqGen("GET", ts.URL, nil)

	clientCalls := 0
	var lastResp *http.Response
	req := NewRequest(ctx, hc, fast, httpReq, func(resp *http.Response) error {
		clientCalls++
		lastResp = resp
		return resp.Body.Close()
	}, nil)

	status, err := req()
	if err != nil {
		t.Fatalf("req returned err %v, want nil", err)
	}
	if got, want := status, http.StatusOK; got != want {
		t.Errorf("req returned status %d, want %d", got, want)
	}

	// We expect only one client call, but three requests through to the server.
	if got, want := clientCalls, 1; got != want {
		t.Errorf("handler callback invoked %d times, want %d", got, want)
	}
	if got, want := len(rt.Responses), 3; got != want {
		t.Errorf("len(Responses) = %d, want %d", got, want)
	}

	// Check that the last response is the one we handled, and that all the bodies
	// were closed.
	if got, want := lastResp, rt.Responses[2]; got != want {
		t.Errorf("Last Response did not match Response in handler callback.\nGot:  %v\nWant: %v", got, want)
	}
	for i, resp := range rt.Responses {
		rc := resp.Body.(*trackingReadCloser)
		if !rc.Closed {
			t.Errorf("Responses[%d].Body was not closed", i)
		}
	}
}

// trackingRoundTripper wraps an http.RoundTripper, keeping track of any
// returned Responses. Each response's Body, when set, is wrapped with a
// trackingReadCloser.
type trackingRoundTripper struct {
	http.RoundTripper

	mu        sync.Mutex
	Responses []*http.Response
}

func (t *trackingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		resp.Body = &trackingReadCloser{ReadCloser: resp.Body}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Responses = append(t.Responses, resp)
	return resp, err
}

// trackingReadCloser wraps an io.ReadCloser, keeping track of whether Closed was
// called.
type trackingReadCloser struct {
	io.ReadCloser
	Closed bool
}

func (t *trackingReadCloser) Close() error {
	t.Closed = true
	return t.ReadCloser.Close()
}

// Private details.

func fast() retry.Iterator {
	return &retry.Limited{Retries: 3}
}
