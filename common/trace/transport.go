// Copyright 2019 The LUCI Authors.
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

package trace

import (
	"context"
	"io"
	"net/http"
)

// tracedTransport implements http.RoundTripper.
type tracedTransport struct {
	ctx  context.Context // used as a fallback if http.Request doesn't have one
	base http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *tracedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if ctx == context.Background() {
		ctx = t.ctx
	}

	ctx, span := backend.StartSpan(ctx, "HTTP:"+req.URL.Path, SpanKindClient)
	if _, ok := span.(NullSpan); ok {
		return t.base.RoundTrip(req) // not tracing, don't bother
	}

	span.Attribute("/http/host", req.Host)
	span.Attribute("/http/method", req.Method)
	span.Attribute("/http/path", req.URL.Path)

	resp, err := t.base.RoundTrip(backend.PropagateSpanContext(ctx, span, req))
	if err != nil {
		span.End(err)
		return resp, err
	}
	span.Attribute("/http/status_code", resp.StatusCode)

	// bodyTracker closes the span on io.EOF or in Close().
	resp.Body = wrappedBody(&bodyTracker{body: resp.Body, span: span}, resp.Body)
	return resp, nil
}

// wrappedBody returns a wrapped version of the original Body and only
// implements the same combination of additional interfaces as the original.
//
// This is needed since as of Go 1.12, http.Response.Body **may** also implement
// io.Writer and we need to "propagate" this through the wrapper.
func wrappedBody(wrapper io.ReadCloser, body io.ReadCloser) io.ReadCloser {
	if wr, isWr := body.(io.Writer); isWr {
		return struct {
			io.ReadCloser
			io.Writer
		}{wrapper, wr}
	}
	return struct{ io.ReadCloser }{wrapper}
}

// bodyTracker tracks how many bytes are read and captures read errors.
//
// It closes the span on io.EOF (or any other read error) or in Close(),
// whatever happens first.
type bodyTracker struct {
	body  io.ReadCloser
	span  Span
	total int64
	ended bool
}

func (bt *bodyTracker) endSpan(err error) {
	if err == io.EOF {
		err = nil
	}
	bt.span.Attribute("/http/response/size", bt.total)
	bt.span.End(err)
	bt.ended = true
}

func (bt *bodyTracker) Read(b []byte) (int, error) {
	n, err := bt.body.Read(b)
	bt.total += int64(n)
	if err != nil && !bt.ended {
		bt.endSpan(err)
	}
	return n, err
}

func (bt *bodyTracker) Close() error {
	if !bt.ended {
		bt.endSpan(nil)
	}
	return bt.body.Close()
}
