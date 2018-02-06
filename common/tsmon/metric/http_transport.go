// Copyright 2016 The LUCI Authors.
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

package metric

import (
	"io"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
)

// instrumentedHTTPRoundTripper reports tsmon metrics about the requests that
// are made using it.  It implements http.RoundTripper.
type instrumentedHTTPRoundTripper struct {
	ctx    context.Context
	base   http.RoundTripper
	client string // ends up in 'client' field of the metrics
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

	UpdateHTTPMetrics(t.ctx, req.URL.Host, t.client, code, duration, requestBytes, responseBytes)

	return resp, err
}

// InstrumentTransport returns a transport that sends HTTP client metrics via
// the given context.
//
// If the context has no tsmon initialized (no metrics store installed), returns
// the original transport unchanged.
func InstrumentTransport(ctx context.Context, base http.RoundTripper, client string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if store.IsNilStore(tsmon.GetState(ctx).S) {
		return base
	}
	return &instrumentedHTTPRoundTripper{ctx, base, client}
}

func init() {
	// We use init hook to break module dependency cycle: auth can't import tsmon
	// module directly.
	auth.SetMonitoringInstrumentation(InstrumentTransport)
}
