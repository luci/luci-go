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
	End(err error)
	// Attribute annotates the span with an attribute.
	Attribute(key string, val interface{})
}

// StartSpan adds a span to the trace with the given name.
//
// This span is put as the current in the context (so nested spans can discover
// it and use it as a parent). The span must be closed with End when done.
func StartSpan(ctx context.Context, name string) (context.Context, Span) {
	if backend == nil {
		return ctx, NullSpan{}
	}
	return backend.StartSpan(ctx, name)
}

////////////////////////////////////////////////////////////////////////////////
// Implementation API.

var backend Backend

// Backend knows how to collect and upload spans.
type Backend interface {
	// StartSpan implements public StartSpan function.
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// SetBackend installs the process-global implementation of the span collector.
//
// May be called at most once (preferably before first StartSpan call). Panics
// otherwise.
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
func (NullSpan) Attribute(key string, val interface{}) {}
