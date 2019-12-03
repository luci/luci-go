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
	"fmt"

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

func (ocTraceBackend) StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	ctx, span := octrace.StartSpan(ctx, name)
	if span.IsRecordingEvents() {
		return ctx, ocSpan{span}
	}
	return ctx, trace.NullSpan{}
}

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
