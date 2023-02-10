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

package internal

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"

	"go.opencensus.io/exporter/stackdriver/propagation"
	octrace "go.opencensus.io/trace"

	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/grpc/grpcutil"
)

// EnableOpenCensusTracing installs OpenCensus as a tracing backend for LUCI
// packages.
func EnableOpenCensusTracing() {
	trace.SetBackend(ocTraceBackend{})
}

type ocTraceBackend struct{}

func (ocTraceBackend) StartSpan(ctx context.Context, name string, kind trace.SpanKind) (context.Context, trace.Span) {
	var ocKind int
	switch kind {
	case trace.SpanKindInternal:
		ocKind = octrace.SpanKindUnspecified
	case trace.SpanKindClient:
		ocKind = octrace.SpanKindClient
	default:
		ocKind = octrace.SpanKindUnspecified
	}
	ctx, span := octrace.StartSpan(ctx, name, octrace.WithSpanKind(ocKind))
	if span.IsRecordingEvents() {
		return ctx, ocSpan{span}
	}
	return ctx, trace.NullSpan{}
}

var cloudTraceFormat = propagation.HTTPFormat{}

func (ocTraceBackend) PropagateSpanContext(ctx context.Context, span trace.Span, req *http.Request) *http.Request {
	// Inject the new context into the request, this also makes a shallow copy.
	req = req.WithContext(ctx)

	// We are about to set some headers, clone the headers map. No need to do
	// a deep clone, since we aren't going to add values to existing slices.
	header := make(http.Header, len(req.Header))
	for k, v := range req.Header {
		header[k] = v
	}
	req.Header = header

	// Inject X-Cloud-Trace-Context header with the encoded span context.
	cloudTraceFormat.SpanContextToRequest(span.(ocSpan).span.SpanContext(), req)
	return req
}

func (ocTraceBackend) SpanContext(ctx context.Context) string {
	if span := octrace.FromContext(ctx); span != nil {
		// Note: this is identical to what SpanContextToRequest does internally.
		sc := span.SpanContext()
		sid := binary.BigEndian.Uint64(sc.SpanID[:])
		return fmt.Sprintf("%s/%d;o=%d", hex.EncodeToString(sc.TraceID[:]), sid, int64(sc.TraceOptions))
	}
	return ""
}

////////////////////////////////////////////////////////////////////////////////

type ocSpan struct {
	span *octrace.Span
}

// End marks the span as finished (with the given status).
func (s ocSpan) End(err error) {
	if err != nil {
		s.span.SetStatus(octrace.Status{
			Code:    int32(grpcutil.Code(err)),
			Message: err.Error(),
		})
	}
	s.span.End()
}

// Attribute annotates the span with an attribute.
func (s ocSpan) Attribute(key string, val interface{}) {
	var a octrace.Attribute
	switch val := val.(type) {
	case string:
		a = octrace.StringAttribute(key, val)
	case bool:
		a = octrace.BoolAttribute(key, val)
	case int:
		a = octrace.Int64Attribute(key, int64(val))
	case int64:
		a = octrace.Int64Attribute(key, val)
	default:
		a = octrace.StringAttribute(key, fmt.Sprintf("%#v", val))
	}
	s.span.AddAttributes(a)
}

////////////////////////////////////////////////////////////////////////////////

const cloudTraceHeader = "X-Cloud-Trace-Context"

// Headers knows how to return request headers.
type Headers interface {
	Header(string) string
}

// SpanContextFromMetadata extracts a Stackdriver Trace span context from
// incoming request metadata.
func SpanContextFromMetadata(h Headers) (octrace.SpanContext, bool) {
	// Unfortunately OpenCensus Stackdriver package hardcodes *http.Request as
	// the request type even though it only really just needs one header.
	header := h.Header(cloudTraceHeader)
	if header == "" {
		return octrace.SpanContext{}, false
	}
	return cloudTraceFormat.SpanContextFromRequest(&http.Request{
		Header: http.Header{cloudTraceHeader: []string{header}},
	})
}
