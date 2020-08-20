// Copyright 2017 The LUCI Authors.
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

package cloud

import (
	"fmt"
	"os"
	"regexp"
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	cloudLogging "cloud.google.com/go/logging"

	"golang.org/x/net/context"
)

var (
	// LogIDRegexp is a regular expression that can be used to validate a Log ID.
	//
	// From: https://godoc.org/cloud.google.com/go/logging#Client.Logger
	LogIDRegexp = regexp.MustCompile(`^[A-Za-z0-9/_\-\.]{0,512}$`)

	// TraceIDLogLabel is the label used to express a request's trace ID in
	// Stackdriver LogEntry messages. Attaching a consistent trace ID label to
	// different logs allows them to be associated with the same trace.
	TraceIDLogLabel = "appengine.googleapis.com/trace_id"

	consoleLoggerConfig = gologger.LoggerConfig{
		Out:    os.Stderr,
		Format: gologger.StdFormat,
	}
)

// InsertIDGenerator generates logging Insert IDs from a base key and an
// atomically-incrementing number. It is safe for concurrent usage.
//
// Insert IDs must be unique, so it's critical that Base is unique for any given
// InsertIDGenerator.
type InsertIDGenerator struct {
	Counter int64
	Base    string
}

// Next returns the next InsertID in the InsertIDGenerator sequence.
//
// As a convenience, a nil InsertIDGenerator will return an empty string.
func (gen *InsertIDGenerator) Next() string {
	if gen == nil {
		return ""
	}

	nextValue := atomic.AddInt64(&gen.Counter, 1)
	if nextValue == 0 {
		// (This should be impossible)
		panic("insert ID generator counter wrapped")
	}
	return fmt.Sprintf("%s_%d", gen.Base, nextValue-1)
}

// LogSeverityTracker tracks the highest observed log severity. It is safe for
// concurrent access.
type LogSeverityTracker struct {
	observed        bool
	highestSeverity cloudLogging.Severity
	lock            sync.Mutex
}

// HighestSeverity returns the highest logging severity that is been observed.
//
// If no logging severity has been explicitly observed, Default severity will
// be returned.
func (lst *LogSeverityTracker) HighestSeverity() cloudLogging.Severity {
	lst.lock.Lock()
	defer lst.lock.Unlock()

	if !lst.observed {
		return cloudLogging.Default
	}
	return lst.highestSeverity
}

// Observe updates the LogSeverityTracker's highest observed log severity.
//
// Observe is safe for concurrent usage.
func (lst *LogSeverityTracker) Observe(s cloudLogging.Severity) {
	lst.lock.Lock()
	defer lst.lock.Unlock()

	if !lst.observed || s > lst.highestSeverity {
		lst.highestSeverity = s
		lst.observed = true
	}
}

// NewConsoleLogger instantiates a new Logger instance that logs to STDERR.
//
// STDERR is independently captured in Flex environments.
func NewConsoleLogger(c context.Context) logging.Logger {
	return consoleLoggerConfig.NewLogger(c)
}

// LoggerConfig configures a Logger instance.
type LoggerConfig struct {
	// SeverityTracker, if not nil, will be updated with the severity of log
	// messages sent through the generated Logger(s).
	SeverityTracker *LogSeverityTracker
	// InsertIDGenerator, if not nil, will be used to generate InsertIDs for
	// generated logs.
	InsertIDGenerator *InsertIDGenerator

	// Trace, if not empty, is the trace to associated with each log entry.
	Trace string

	// LogToSTDERR, if true, indicates that log messages should be tee'd to
	// a simple STDERR logger prior to being written.
	LogToSTDERR bool

	// The given labels will be applied to each log entry. This allows to reuse
	// Logger instance between requests, even if they have different default
	// labels.
	Labels map[string]string
}

// WithLogger installs an instance of a logging.Logger that forwards logs to an
// underlying Stackdriver Logger into the supplied Context.
//
// An alternative is to construct Logger per request, setting request labels via
// CommonLabels(...) option. But it is more heavy solution, and at the time of
// writing it leads to memory leaks:
// https://github.com/GoogleCloudPlatform/google-cloud-go/issues/720#issuecomment-346199870
func WithLogger(c context.Context, l *cloudLogging.Logger, cfg *LoggerConfig) context.Context {
	if cfg == nil {
		cfg = &LoggerConfig{}
	}

	return logging.SetFactory(c, func(c context.Context) logging.Logger {
		bl := boundLogger{
			Context:      c,
			LoggerConfig: *cfg,
			cl:           l,
		}
		if cfg.LogToSTDERR {
			bl.stderrLogger = NewConsoleLogger(c)
		}
		return &bl
	})
}

type boundLogger struct {
	context.Context
	LoggerConfig
	stderrLogger logging.Logger
	cl           *cloudLogging.Logger
}

func (bl *boundLogger) Debugf(format string, args ...interface{}) {
	bl.LogCall(logging.Debug, 1, format, args)
}

func (bl *boundLogger) Infof(format string, args ...interface{}) {
	bl.LogCall(logging.Info, 1, format, args)
}

func (bl *boundLogger) Warningf(format string, args ...interface{}) {
	bl.LogCall(logging.Warning, 1, format, args)
}

func (bl *boundLogger) Errorf(format string, args ...interface{}) {
	bl.LogCall(logging.Error, 1, format, args)
}

func (bl *boundLogger) LogCall(lvl logging.Level, calldepth int, format string, args []interface{}) {
	if bl.stderrLogger != nil {
		bl.stderrLogger.LogCall(lvl, calldepth+1, format, args)
	}

	severity := severityForLevel(lvl)
	if bl.SeverityTracker != nil {
		bl.SeverityTracker.Observe(severity)
	}

	fields := logging.GetFields(bl)
	line := fmt.Sprintf(format, args...)
	if len(fields) > 0 {
		line = fmt.Sprintf("%s :: %s", line, fields.String())
	}

	// Per docs, 'Log' takes ownership of Labels map, so make a copy.
	var labels map[string]string
	if len(bl.Labels) != 0 {
		labels = make(map[string]string, len(bl.Labels))
		for k, v := range bl.Labels {
			labels[k] = v
		}
	}

	// Generate a LogEntry for the supplied parameters.
	bl.cl.Log(cloudLogging.Entry{
		Severity: severity,
		Payload:  line,
		Labels:   labels,
		InsertID: bl.InsertIDGenerator.Next(),
		Trace:    bl.Trace,
	})
}

func severityForLevel(l logging.Level) cloudLogging.Severity {
	switch l {
	case logging.Debug:
		return cloudLogging.Debug
	case logging.Info:
		return cloudLogging.Info
	case logging.Warning:
		return cloudLogging.Warning
	case logging.Error:
		return cloudLogging.Error
	default:
		return cloudLogging.Default
	}
}
