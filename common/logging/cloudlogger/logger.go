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

	"go.chromium.org/luci/common/clock"
	rand "go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"golang.org/x/net/context"
)

// loggerIface is a testing interface for a *cloudLoging.Logger struct.
type loggerIface interface {
	Log(cloudLogging.Entry)
}

func GetFactory(main, lines loggerIface) func(context.Context) logging.Logger {
	if main == nil {
		panic("no logger specified")
	}
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
	main  loggerIface
	lines loggerIface
}

const RequestKey = "request"
const TraceIDKey = "traceID"
const TraceSize = 128

var localIPKey = "key for local IP"
var severityKey = "key for severityRecorder"

// severityRecorder lives in the context and records the highest severity
// log recorded since inception.  It is meant to be installed alongside the trace.
type severityRecorder struct {
	max logging.Level
}

func getRecorder(c context.Context) *severityRecorder {
	sr, _ := c.Value(&severityKey).(*severityRecorder)
	return sr
}

func withRecorder(c context.Context, r *severityRecorder) context.Context {
	return context.WithValue(c, &severityKey, r)
}

// Request emulates an HTTP request, and its embedded into the logging fields
// as RequestKey to signal a bundled log.
// StartTime and Lantency are required, everything else is optional.
// In addition, traceID must be specified in fields.
type Request struct {
	StartTime time.Time
	Latency   time.Duration
	// These must be private to avoid JSON serilization.
	request  *http.Request
	response *http.Response
}

// WithLocalIP records the addr into the context to be used as the Local IP address for logging.
// This can be an arbitrary string.
func WithLocalIP(c context.Context, addr string) context.Context {
	return context.WithValue(c, &localIPKey, addr)
}

func getLocalIP(c context.Context) string {
	if addr, ok := c.Value(&localIPKey).(string); ok {
		return addr
	}
	return ""
}

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
func StartTrace(c context.Context, request *http.Request) context.Context {
	return StartWithTraceID(c, request, randString(c, TraceSize))
}

// StartWithTraceID is a convenience function to set a traceID in the logging context.
func StartWithTraceID(c context.Context, request *http.Request, traceID string) context.Context {
	req := &Request{
		StartTime: clock.Now(c),
		request:   request,
	}
	c = logging.SetField(c, TraceIDKey, traceID)
	c = withRecorder(c, &severityRecorder{})
	return logging.SetField(c, RequestKey, req)
}

// EndTrace is a convenience function to write a log into the parent logger if
// the context has been previously set with a *Request and TraceID.  If
// no *Request and TraceID are present in the logging context, this outputs a
// debug log into the logger in the context and then exits.
func EndTrace(c context.Context, response *http.Response) {
	fields := logging.GetFields(c)
	if fields == nil {
		logging.Debugf(c, "Failed to end trace: no logging fields set in context.")
		return
	}
	traceID, ok := fields[TraceIDKey].(string)
	if !ok {
		logging.Debugf(c, "Failed to end trace: no trace ID in set in logging context")
		return
	}
	req, ok := fields[RequestKey].(*Request)
	if !ok {
		logging.Debugf(c, "Failed to end trace: no Request in set in logging context")
		return
	}
	sr := getRecorder(c)
	if sr == nil {
		logging.Debugf(c, "Failed to end trace: no severityRecorder install in context")
		return
	}
	// If the caller didn't set a latency, calculate it from the StartTime in req.
	if req.Latency == 0 {
		req.Latency = clock.Now(c).Sub(req.StartTime)
	}

	// Set the status code based on this order:
	// 1. If response is passed in.
	// 2. If an error is in the fields.
	// 3. Default to http.StatusOK.
	if response == nil {
		response = &http.Response{}
	}
	if response.StatusCode == 0 {
		if err, ok := fields[logging.ErrorKey].(error); ok && err != nil {
			// If there is an embedded GRPC error status code, use that.
			response.StatusCode = grpcutil.ErrStatus(err)
		} else {
			response.StatusCode = http.StatusOK
		}
	}

	// EndTrace is the only thing that can set req.response, so set it here.
	req.response = response

	// A URL is required by Stackdriver, set a fake one if none is set.
	if req.request == nil {
		req.request = &http.Request{}
	}
	if req.request.URL == nil {
		req.request.URL = &url.URL{
			Path: fmt.Sprintf("/fakeurl/%s", traceID),
		}
	}

	logging.Logf(c, sr.max, "Request completed")
}

// EndTraceWithError embeds an error into the logging context, then calls EndTrace.
// An optional response size can be set.
func EndTraceWithError(c context.Context, responseSize int64, err error) {
	EndTrace(logging.SetField(c, logging.ErrorKey, err), &http.Response{ContentLength: responseSize})
}

// severityMap maps logging severity levels to cloudLogging severity levels.
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

	// If there's an embedded timestamp, dig it out and surface it instead.
	// If this log entry is a parent log, this will eventually be ignored.
	if timestamp, ok := fields["timestamp"].(time.Time); ok {
		entry.Timestamp = timestamp
	}

	// If there's an embedded trace ID, dig it out and surface it as Trace.
	if traceID, ok := fields["traceID"].(string); ok {
		entry.Trace = traceID
	}

	// Record the severity, if there is a severity recorder installed.
	sr := getRecorder(l.ctx)
	if sr != nil && lv > sr.max {
		sr.max = lv
	}

	// Only output a "parent" log to the main logger when these criterias are fulfilled:
	// * Both loggers are in the factory, main and lines.
	// * A *Request struct is present in the logging fields.
	// * Request.request.URL exists.
	// * Request.Latency is filled in.
	// * Trace is filled in.
	// Otherwise, Stackdriver will not recognize this as a parent log.
	switch req, ok := fields[RequestKey].(*Request); {
	case ok && req.Latency > 0 && l.lines != nil && req.request != nil && req.request.URL != nil && entry.Trace != "":
		entry.Timestamp = req.StartTime
		entry.HTTPRequest = &cloudLogging.HTTPRequest{
			Latency:     req.Latency,
			Request:     req.request,
			RequestSize: req.request.ContentLength,
			RemoteIP:    req.request.RemoteAddr,
			LocalIP:     getLocalIP(l.ctx),
		}
		if req.response != nil {
			entry.HTTPRequest.Status = req.response.StatusCode
			entry.HTTPRequest.ResponseSize = req.response.ContentLength
		}
		l.main.Log(entry)
	case l.lines != nil:
		// Both loggers are filled, but the above criterias were not fulfilled,
		// that means this is a child log entry.
		l.lines.Log(entry)
	default:
		// Only one logger is filled, always log to the main logger.
		l.main.Log(entry)
	}
}
