// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package metric

import (
	"io"
	"net/http"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/iotools"
)

// instrumentedHTTPRoundTripper reports tsmon metrics about the requests that
// are made using it.  It implements http.RoundTripper.
type instrumentedHTTPRoundTripper struct {
	ctx  context.Context
	base http.RoundTripper
	name string
}

// RoundTrip implements http.RoundTripper.
func (t *instrumentedHTTPRoundTripper) RoundTrip(origReq *http.Request) (*http.Response, error) {
	req := *origReq

	// If a request body was provided, wrap it in a CountingReader so we can see
	// how big it was after it's been sent.
	var requestBodyReader *iotools.CountingReader
	if req.Body != nil {
		requestBodyReader = &iotools.CountingReader{Reader: req.Body}
		req.Body = struct {
			io.Reader
			io.Closer
		}{requestBodyReader, req.Body}
	}

	start := clock.Now(t.ctx)
	resp, err := t.base.RoundTrip(&req)
	duration := clock.Now(t.ctx).Sub(start)

	var requestBytes int64
	if requestBodyReader != nil {
		requestBytes = requestBodyReader.Count
	}

	var code int
	var responseBytes int64
	if resp != nil {
		code = resp.StatusCode
		responseBytes = resp.ContentLength
	}

	UpdateHTTPMetrics(t.ctx, req.URL.Host, t.name, code, duration, requestBytes, responseBytes)

	return resp, err
}

// InstrumentTransport returns a transport that sends HTTP client metrics via
// the given context.
func InstrumentTransport(ctx context.Context, base http.RoundTripper, name string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &instrumentedHTTPRoundTripper{ctx, base, name}
}
