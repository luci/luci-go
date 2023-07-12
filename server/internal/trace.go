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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"go.chromium.org/luci/common/trace"
)

var tracer = otel.Tracer("go.chromium.org/luci/common/trace")

// EnableOpenTelemetryTracing installs OpenTelemetry as a tracing backend for
// LUCI packages.
//
// TODO(vadimsh): Get rid of it and use OpenTelemetry API directly.
func EnableOpenTelemetryTracing() {
	trace.SetBackend(otelBackend{})
}

type otelBackend struct{}

func (otelBackend) StartSpan(ctx context.Context, name string, kind trace.SpanKind) (context.Context, trace.Span) {
	otelKind := oteltrace.SpanKindUnspecified
	switch kind {
	case trace.SpanKindInternal:
		otelKind = oteltrace.SpanKindInternal
	case trace.SpanKindClient:
		otelKind = oteltrace.SpanKindClient
	}
	ctx, span := tracer.Start(ctx, name, oteltrace.WithSpanKind(otelKind))
	if span.IsRecording() {
		return ctx, otelSpan{span}
	}
	return ctx, trace.NullSpan{}
}

////////////////////////////////////////////////////////////////////////////////

type otelSpan struct {
	span oteltrace.Span
}

// End marks the span as finished (with the given status).
func (s otelSpan) End(err error) {
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End()
}

// Attribute annotates the span with an attribute.
func (s otelSpan) Attribute(key string, val any) {
	switch val := val.(type) {
	case string:
		s.span.SetAttributes(attribute.String(key, val))
	case bool:
		s.span.SetAttributes(attribute.Bool(key, val))
	case int:
		s.span.SetAttributes(attribute.Int64(key, int64(val)))
	case int64:
		s.span.SetAttributes(attribute.Int64(key, val))
	default:
		s.span.SetAttributes(attribute.String(key, fmt.Sprintf("%#v", val)))
	}
}
