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

package cloudlogger

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	cloudLogging "cloud.google.com/go/logging"
	clAPI "google.golang.org/genproto/googleapis/logging/v2"

	"go.chromium.org/luci/common/clock"
	rand "go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"golang.org/x/net/context"
)

func GetFactory(main, lines *cloudLogging.Logger) func(context.Context) logging.Logger {
	return func(c context.Context) logging.Logger {
		return &logger{
			ctx:   c,
			main:  main,
			lines: lines,
		}
	}
}

type logger struct {
	ctx   context.Context
	main  *cloudLogging.Logger
	lines *cloudLogging.Logger
}

const RequestKey = "request"
const TraceIDKey = "traceID"
const TraceSize = 128

var charset = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// randString creates a random string of length size using characters from charset.
func randString(c context.Context, size uint64) string {
	buf := make([]rune, size)
	for i := range buf {
		buf[i] = charset[rand.Intn(c, len(charset))]
	}
	return string(buf)
}

// StartTrace generates a random string and sets the trace ID in the logging context.
func StartTrace(c context.Context) context.Context {
	return SetTraceID(c, randString(c, TraceSize))
}

// SetTrace is a convenience function to set a traceID in the logging context.
func SetTraceID(c context.Context, traceID string) context.Context {
	return logging.SetField(c, TraceIDKey, traceID)
}

// Request emulates an HTTP request, and its embedded into the logging fields
// as RequestKey to signal a bundled log.  StartTime and Lantency are required,
// everything else is optional.  In addition, traceID must be specified in fields.
type Request struct {
	StartTime time.Time
	Latency   time.Duration
	URL       *url.URL
	Status    int
	UserAgent string
	Size      int64
	RemoteIP  string
	LocalIP   string
}

var severityMap = map[logging.Level]cloudLogging.Severity{
	logging.Debug:   cloudLogging.Debug,
	logging.Info:    cloudLogging.Info,
	logging.Warning: cloudLogging.Warning,
	logging.Error:   cloudLogging.Error,
}

func (l *logger) Debugf(format string, args ...interface{}) {
	l.LogCall(logging.Debug, 1, format, args)
}

func (l *logger) Infof(format string, args ...interface{}) {
	l.LogCall(logging.Info, 1, format, args)
}

func (l *logger) Warningf(format string, args ...interface{}) {
	l.LogCall(logging.Warning, 1, format, args)
}

func (l *logger) Errorf(format string, args ...interface{}) {
	l.LogCall(logging.Error, 1, format, args)
}

func (l *logger) LogCall(lv logging.Level, calldepth int, format string, args []interface{}) {
	severity, ok := severityMap[lv]
	if !ok {
		severity = cloudLogging.Default
	}
	fields := logging.GetFields(l.ctx)
	if fields == nil {
		fields = make(logging.Fields)
	}
	// This makes the message show up magically in the log viewer.
	fields["message"] = fmt.Sprintf(format, args...)
	if logError, ok := fields[logging.ErrorKey]; ok {
		// Go Errors don't show up as JSON, turn it into a string.
		fields[logging.ErrorKey] = fmt.Sprintf("%s", logError)
	}
	entry := cloudLogging.Entry{
		Timestamp: clock.Now(l.ctx),
		Severity:  severity,
		Payload:   fields,
	}
	// If there's an embedded operation, dig it out and surface it.
	if operation, ok := fields["operation"].(*clAPI.LogEntryOperation); ok {
		entry.Operation = operation
		if operation.First {
			defer func() {
				operation.First = false
			}()
		}
	}
	// If there's an embedded trace ID, dig it out and surface it.
	if traceID, ok := fields["traceID"].(string); ok {
		entry.Trace = traceID
	}
	// If there's an embedded timestamp, dig it out and surface it, else just
	// use our own timestamp.
	if timestamp, ok := fields["timestamp"].(time.Time); ok {
		entry.Timestamp = timestamp
	} else {
		entry.Timestamp = clock.Now(l.ctx)
	}

	if req, ok := fields[RequestKey].(Request); ok && req.Latency > 0 {
		entry.Timestamp = req.StartTime
		entry.HTTPRequest = &cloudLogging.HTTPRequest{
			Latency:      req.Latency,
			Status:       req.Status,
			ResponseSize: req.Size,
			Request:      &http.Request{URL: req.URL},
			LocalIP:      req.LocalIP,
			RemoteIP:     req.RemoteIP,
		}
		l.parent.Log(entry)
	} else {
		l.child.Log(entry)
	}
}
