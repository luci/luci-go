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

// Package trace provides support for collecting tracing spans.
//
// It decouples spans collection from the system that actually uploads spans.
// So it tracing is not needed in a binary, there's almost 0 overhead on it.
package trace

import (
	"context"
	"fmt"
)

// Span is in-progress trace span opened by StartSpan.
type Span interface {
	// End marks the span as finished (with the given status).
	//
	// Recognizes gRPC errors and annotates spans with their status. Also, for all
	// errors, annotates spans with the error message.
	End(err error)

	// Attribute annotates the span with an attribute.
	Attribute(key string, val any)
}

// StartSpan adds a span with the given name to the current trace.
//
// This span is set as the current in the context (so nested spans can discover
// it and use it as a parent). The span must be closed with End when done.
//
// May return NullSpan{} if tracing is disabled globally in the process or the
// current trace is not being recorded.
func StartSpan(ctx context.Context, name string) (context.Context, Span) {
	if backend == nil {
		return ctx, NullSpan{}
	}
	return backend.StartSpan(ctx, name, SpanKindInternal)
}

////////////////////////////////////////////////////////////////////////////////
// Implementation API.

var backend Backend

// SpanKind is an enum that defines a flavor of a span.
type SpanKind int

const (
	SpanKindInternal SpanKind = 0 // a span internal to the server
	SpanKindClient   SpanKind = 1 // a span with a client side of an RPC call
)

// Backend knows how to collect and upload spans.
type Backend interface {
	// StartSpan implements public StartSpan function.
	StartSpan(ctx context.Context, name string, kind SpanKind) (context.Context, Span)
}

// SetBackend installs the process-global implementation of the span collector.
//
// May be called at most once (preferably before the first StartSpan call).
// Panics otherwise.
func SetBackend(b Backend) {
	if backend != nil {
		panic(fmt.Sprintf("trace.SetBackend had already been called with %v", backend))
	}
	backend = b
}

// NullSpan implements Span by doing nothing.
type NullSpan struct{}

// End does nothing.
func (NullSpan) End(err error) {}

// Attribute does nothing.
func (NullSpan) Attribute(key string, val any) {}
