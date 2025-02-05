// Copyright 2025 The LUCI Authors.
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

// Package errlogger implements a logger that logs errors to Cloud Error
// Reporting service.
package errlogger

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// ServiceContext is information about the running service.
type ServiceContext struct {
	// Project is the cloud project name that hosts the service.
	Project string
	// Service is identifies of the service within the project.
	Service string
	// Version is the service's version string.
	Version string
}

// RequestContext is information about the incoming request.
type RequestContext struct {
	// HTTPMethod is e.g. "GET" or "POST".
	HTTPMethod string
	// URL is a request's URL string.
	URL string
	// UserAgent is taken from the request headers.
	UserAgent string
	// RemoteIP is the callers IP address.
	RemoteIP string
	// TraceID is a trace ID associated with this request.
	TraceID string
}

// ErrorReport is submitted to the sink.
type ErrorReport struct {
	// ServiceContext identifies the service, always set.
	ServiceContext *ServiceContext
	// RequestContext identifies the request, if any. Can be nil.
	RequestContext *RequestContext
	// User is a token used to count "affected users" in the aggregated reports.
	User string
	// Timestamp is when the error happened.
	Timestamp time.Time
	// Message is a human-readable error message.
	Message string
	// Stack is a stack trace to associated with the error message, always set.
	Stack string
}

// Sink uploads error reports to the error aggregation service.
//
// Always best effort. Should handle errors internally. Should not block (e.g.
// by dropping excessive reports).
type Sink interface {
	// ReportError asynchronously uploads the error report.
	ReportError(rep *ErrorReport)
}

// Config holds configuration for the error reporting logger.
type Config struct {
	// Sink is where to send reports. Required.
	Sink Sink
	// ServiceContext is information about the running service. Required.
	ServiceContext *ServiceContext
	// UserResolver returns a token identifying an effected user. Optional.
	UserResolver func(ctx context.Context) string
}

// Factory returns a factory that produces error loggers scoped to a single
// request (if any).
func Factory(cfg *Config, req *RequestContext) logging.Factory {
	if cfg.Sink == nil {
		panic("cfg.Sink is required")
	}
	if cfg.ServiceContext == nil {
		panic("cfg.ServiceContext is required")
	}
	return func(ctx context.Context, lc *logging.LogContext) logging.Logger {
		return errLogger(func(fm string, args []any) {
			var user string
			if cfg.UserResolver != nil {
				user = cfg.UserResolver(ctx)
			}
			cfg.Sink.ReportError(&ErrorReport{
				ServiceContext: cfg.ServiceContext,
				RequestContext: req,
				User:           user,
				Timestamp:      clock.Now(ctx),
				Message:        fmt.Sprintf(fm, args...),
				// TODO: Use the stack from logging.LogContext if present.
				Stack: captureStack(),
			})
		})
	}
}

// captureStack captures the current stack trace, cleaning it up by removing
// stack frames belonging to the logging library itself.
func captureStack() string {
	var buf [16 * 1024]byte
	stack := buf[0:runtime.Stack(buf[:], false)]
	return cleanupStack(stack)
}

// cleanupStack removes internal logging stack frames from the stack trace.
//
// That way the data reported to Cloud Error Reporting is cleaner.
func cleanupStack(s []byte) string {
	// `s` has form:
	//
	//  <first line with some overall message>\n
	//  <symbolname>(<args>)\n
	//  \t<filepath> <offset>\n
	//  <symbolname>(<args>)\n
	//  \t<filepath> <offset>\n
	//  ...
	//
	// We need to preserve the first line, but remove all head entries (pairs of
	// lines) for symbols in go.chromium.org/luci/common/logging package.
	const skipPrefix = "go.chromium.org/luci/common/logging"

	readLine := func() (line []byte) {
		if idx := bytes.IndexByte(s, '\n'); idx != -1 {
			line, s = s[:idx+1], s[idx+1:]
		}
		return line
	}

	firstLine := readLine()
	if firstLine == nil {
		return string(s)
	}

	// Write the first line to the output as is.
	out := strings.Builder{}
	out.Grow(len(firstLine) + len(s))
	_, _ = out.Write(firstLine)

	// Skip pairs of lines we want filtered out.
	for len(s) > 0 {
		// Read <symbolname>(<args>)\n.
		if sym := readLine(); !bytes.HasPrefix(sym, []byte(skipPrefix)) {
			// Something unexpected (or just EOF). Dump it all to the output.
			_, _ = out.Write(sym)
			break
		}
		// Skip the following line with "\t<filepath> <offset>\n" as well.
		_ = readLine()
	}

	// Dump the rest as is.
	_, _ = out.Write(s)
	return out.String()
}

// errLogger processes all error-level messages by calling the callback.
type errLogger func(fmt string, args []any)

func (t errLogger) Debugf(fmt string, args ...any)   {}
func (t errLogger) Infof(fmt string, args ...any)    {}
func (t errLogger) Warningf(fmt string, args ...any) {}

func (t errLogger) Errorf(fmt string, args ...any) {
	t(fmt, args)
}

func (t errLogger) LogCall(level logging.Level, calldepth int, fmt string, args []any) {
	if level >= logging.Error {
		t(fmt, args)
	}
}
