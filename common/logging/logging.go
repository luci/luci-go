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

// Package logging defines Logger interface and context.Context helpers to
// put\get logger from context.Context.
//
// Unfortunately standard library doesn't define any Logger interface (only
// struct). And even worse: GAE logger is exposing different set of methods.
// Some additional layer is needed to unify the logging. Package logging is
// intended to be used from packages that support both local and GAE
// environments. Such packages should not use global logger but must accept
// instances of Logger interface (or even more generally context.Context) as
// parameters. Then callers can pass appropriate Logger implementation (or
// inject appropriate logger into context.Context) depending on where the code
// is running.
package logging

import "context"

// Logger interface is ultimately implemented by underlying logging libraries
// (like go-logging or GAE logging). It is the least common denominator among
// logger implementations.
//
// Logger instance is bound to some particular LogContext that defines logging
// level and extra message fields.
//
// Implementations register factories that produce Loggers (using 'SetFactory'
// function), and top level functions (like 'Infof') use them to grab instances
// of Logger bound to passed contexts. That's how they know what logging level
// to use and what extra fields to add.
type Logger interface {
	// Debugf formats its arguments according to the format, analogous to
	// fmt.Printf and records the text as a log message at Debug level.
	Debugf(format string, args ...any)

	// Infof is like Debugf, but logs at Info level.
	Infof(format string, args ...any)

	// Warningf is like Debugf, but logs at Warning level.
	Warningf(format string, args ...any)

	// Errorf is like Debugf, but logs at Error level.
	Errorf(format string, args ...any)

	// LogCall is a generic logging function. This is oriented more towards
	// utility functions than direct end-user usage.
	LogCall(l Level, calldepth int, format string, args []any)
}

// LogContext is the current logging context: the logging level, logging fields,
// etc.
//
// It is propagated through context.Context. It is primarily used by logging
// handlers. Callers usually use functions like SetLevel to modify it.
//
// Values of LogContext are immutable once constructed.
type LogContext struct {
	// Factory can instantiate loggers configured to use this context.
	Factory Factory
	// Level is the current logging level.
	Level Level
	// Fields is the current field put into all messages.
	Fields Fields
}

var ctxKey = "logging.LogContext"
var defaultCtx = LogContext{Level: DefaultLevel}

// readCtx returns the current context or the default context. Never nil.
func readCtx(ctx context.Context) *LogContext {
	if inCtx := ctx.Value(&ctxKey); inCtx != nil {
		return inCtx.(*LogContext)
	}
	return &defaultCtx
}

// modifyCtx calls the callback to modify the current context and stores it.
func modifyCtx(ctx context.Context, cb func(*LogContext)) context.Context {
	cur := *readCtx(ctx)
	cb(&cur)
	return context.WithValue(ctx, &ctxKey, &cur)
}
