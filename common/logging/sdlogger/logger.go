// Copyright 2018 The LUCI Authors.
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

// Package sdlogger is a logger that formats entries as JSON understood by
// Stackdriver log collectors.
//
// The JSON format is defined by the Google Cloud Logging plugin for fluentd,
// which is mainly used by Google Kubernetes Engine + Stackdriver stack.
// See https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud
//
// Though not properly documented, GAE environment also understands subset of
// this format.
package sdlogger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// Severity is a string denoting the logging level of the entry.
//
// Must be one of the predefined constants, otherwise Stackdriver may not
// recognize it.
type Severity string

// All allowed severities. Using other values results in undefined behavior.
const (
	UnknownSeverity Severity = ""
	DebugSeverity   Severity = "DEBUG"
	InfoSeverity    Severity = "INFO"
	WarningSeverity Severity = "WARNING"
	ErrorSeverity   Severity = "ERROR"
)

// LevelToSeverity converts logging level to the corresponding severity.
func LevelToSeverity(l logging.Level) Severity {
	switch l {
	case logging.Debug:
		return DebugSeverity
	case logging.Info:
		return InfoSeverity
	case logging.Warning:
		return WarningSeverity
	case logging.Error:
		return ErrorSeverity
	default:
		return UnknownSeverity
	}
}

// LogEntry defines a structure of JSON log entries recognized by google-fluentd
// plugin on GKE.
//
// See https://cloud.google.com/logging/docs/agent/configuration#process-payload
// for a list of possible magical fields. All other fields are exposed as part
// of jsonPayload in Stackdriver logs.
type LogEntry struct {
	// Severity is a string denoting the logging level of the entry.
	Severity Severity `json:"severity"`
	// Message is a single line human readable string of the log message.
	Message string `json:"message,omitempty"`
	// Timestamp is the unix timestamp of when the entry was produced.
	Timestamp Timestamp `json:"timestamp,omitempty"`
	// TraceID is 32-byte hex string with Stackdriver trace ID.
	TraceID string `json:"logging.googleapis.com/trace,omitempty"`
	// TraceSampled is true if this trace will be uploaded to Stackdriver.
	TraceSampled bool `json:"logging.googleapis.com/trace_sampled,omitempty"`
	// SpanID is a 16-byte hex string with Stackdriver span ID.
	SpanID string `json:"logging.googleapis.com/spanId,omitempty"`
	// Operation is used to group log lines from a single request together.
	Operation *Operation `json:"logging.googleapis.com/operation,omitempty"`
	// RequestInfo is information about the handled HTTP request.
	RequestInfo *RequestInfo `json:"httpRequest,omitempty"`
	// Fields are extra structured data that may be tagged in the log entry.
	Fields logging.Fields `json:"fields,omitempty"`
}

// Timestamp represents a unix timestamp.
type Timestamp struct {
	Seconds int64 `json:"seconds"`
	Nanos   int64 `json:"nanos"`
}

// ToTimestamp converts time.Time to Timestamp.
func ToTimestamp(t time.Time) Timestamp {
	ts := t.UnixNano()
	return Timestamp{
		Seconds: ts / 1e9,
		Nanos:   ts % 1e9,
	}
}

// Operation is used to group log lines from a single request together.
type Operation struct {
	ID string `json:"id"`
}

// RequestInfo contains information about handled HTTP request.
//
// See following definition for all fields we can possibly expose here:
// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#HttpRequest
type RequestInfo struct {
	Method       string `json:"requestMethod"` // e.g. "GET"
	URL          string `json:"requestUrl"`    // e.g. "http://host/path"
	Status       int    `json:"status"`        // e.g. 200
	RequestSize  string `json:"requestSize"`   // e.g. "123123"
	ResponseSize string `json:"responseSize"`  // e.g. "2324"
	UserAgent    string `json:"userAgent"`     // e.g. "Mozilla/4.0 ..."
	RemoteIP     string `json:"remoteIp"`      // e.g. "192.168.1.1"
	Latency      string `json:"latency"`       // e.g. "3.5s"
}

// LogEntryMutator may mutate log entries before they are written.
type LogEntryMutator func(context.Context, *LogEntry)

// LogEntryWriter knows how to write LogEntries to some output.
type LogEntryWriter interface {
	// Write appends a log entry to the output.
	Write(*LogEntry)
}

// Sink takes care of JSON-serializing log entries and synchronizing writes
// to an io.Writer (usually stdout or stderr).
//
// Implements LogEntryWriter.
//
// Sink can either be used directly for fine-grain control of what is getting
// logged, or via a logging.Logger for interoperability with most of LUCI code.
//
// There should be at most one Sink instance assigned to a given io.Writer,
// shared by all Logger instances writing to it. Violating this requirement may
// cause malformed log lines.
type Sink struct {
	Out io.Writer

	l sync.Mutex
}

// Write appends a log entry to the output by JSON-serializing and writing it.
//
// Panics if the LogEntry can't be serialized (this should not be possible).
// Ignores errors from io.Writer.
func (s *Sink) Write(l *LogEntry) {
	buf, err := json.Marshal(l)
	if err != nil {
		panic(err)
	}
	s.l.Lock()
	defer s.l.Unlock()
	_, _ = s.Out.Write(buf)
	_, _ = s.Out.Write([]byte("\n"))
}

// Factory returns a factory of logger.Logger instances that log to the given
// LogEntryWriter (usually &Sink{...}) using the LogEntry (if any) as a
// prototype for log entries.
//
// For each log message, makes a copy of 'prototype', overwrites its Severity,
// Message, Time and Fields (keeping other fields as they are), calls 'mut'
// callback (which is allowed to mutate the log entry), and writes the result to
// LogEntryWriter. This allows to "prepopulate" fields like TraceID or
// OperationID in all log entries emitted by logging.Logger.
//
// Such factory can be installed in the context via logging.SetFactory.
func Factory(w LogEntryWriter, prototype LogEntry, mut LogEntryMutator) func(context.Context, *logging.LogContext) logging.Logger {
	return func(ctx context.Context, lc *logging.LogContext) logging.Logger {
		return &jsonLogger{
			ctx:       ctx,
			lc:        lc,
			w:         w,
			prototype: prototype,
			mut:       mut,
		}
	}
}

type jsonLogger struct {
	ctx       context.Context
	lc        *logging.LogContext
	w         LogEntryWriter
	prototype LogEntry
	mut       LogEntryMutator
}

func (l *jsonLogger) Debugf(format string, args ...any) {
	l.LogCall(logging.Debug, 1, format, args)
}

func (l *jsonLogger) Infof(format string, args ...any) {
	l.LogCall(logging.Info, 1, format, args)
}

func (l *jsonLogger) Warningf(format string, args ...any) {
	l.LogCall(logging.Warning, 1, format, args)
}

func (l *jsonLogger) Errorf(format string, args ...any) {
	l.LogCall(logging.Error, 1, format, args)
}

func (l *jsonLogger) LogCall(lvl logging.Level, calldepth int, format string, args []any) {
	if lvl < l.lc.Level {
		return
	}

	var msg strings.Builder
	_, _ = fmt.Fprintf(&msg, format, args...)

	fields := l.lc.Fields
	if len(fields) > 0 {
		// logging.ErrorKey usually points to a value that implements 'error'
		// interface, which is not JSON-serializable. Convert it to a string. Note
		// that mutating the result of logging.GetFields in place is not allowed, so
		// we'll make a copy.
		if err, ok := fields[logging.ErrorKey].(error); ok {
			fields = logging.NewFields(fields)
			fields[logging.ErrorKey] = err.Error()
		}
		msg.WriteString(" :: ")
		msg.WriteString(fields.String())
	}

	if stack := l.lc.StackTrace.ForTextLog(); stack != "" {
		msg.WriteString("\n\n")
		msg.WriteString(stack)
	}

	e := l.prototype
	e.Severity = LevelToSeverity(lvl)
	e.Message = msg.String()
	e.Timestamp = ToTimestamp(clock.Now(l.ctx))
	e.Fields = fields

	if l.mut != nil {
		l.mut(l.ctx, &e)
	}

	l.w.Write(&e)
}
