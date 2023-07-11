// Copyright 2021 The LUCI Authors.
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

// Package tracetest contains tracing system implementation useful in tests.
package tracetest

import (
	"context"
	"sync/atomic"

	"go.chromium.org/luci/common/trace"
)

var (
	enabled    int64
	spanCtxKey = "tracetest.SpanContext"
)

// Enable installs tracetest fake tracing system globally in the process.
func Enable() {
	if atomic.AddInt64(&enabled, 1) == 1 {
		trace.SetBackend(testingBackend{})
	}
}

// WithSpanContext overrides the span context string in the context.
//
// The following property holds:
//
//	trace.SpanContext(tracetest.WithSpanContext(ctx, "abc")) == "abc".
func WithSpanContext(ctx context.Context, spanCtx string) context.Context {
	return context.WithValue(ctx, &spanCtxKey, spanCtx)
}

type testingBackend struct{}

func (testingBackend) StartSpan(ctx context.Context, name string, kind trace.SpanKind) (context.Context, trace.Span) {
	return ctx, trace.NullSpan{}
}

func (testingBackend) SpanContext(ctx context.Context) string {
	val, _ := ctx.Value(&spanCtxKey).(string)
	return val
}
