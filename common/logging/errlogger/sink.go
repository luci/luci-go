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

package errlogger

import (
	"context"
	"fmt"
	"sync"
	"time"

	vkit "cloud.google.com/go/errorreporting/apiv1beta1"
	"cloud.google.com/go/errorreporting/apiv1beta1/errorreportingpb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging"
)

const (
	uploaderGoroutines = 8
	warnThottling      = 5 * time.Second
	maxPendingReports  = 500
	flushTimeout       = 5 * time.Second
)

// CloudErrorReporter implements Sink by uploading reports to Cloud Error
// Reporting.
//
// Construct it with NewCloudErrorReporter. Close it with Close.
type CloudErrorReporter struct {
	client errorReporterClient
	ctx    context.Context

	chM     sync.Mutex
	ch      chan *ErrorReport // nil if already stopped
	dropped int64             // how many reports were dropped due to overflow

	warnM        sync.Mutex
	lastWarnMsg  string    // used to skip excessive warnings
	lastWarnTime time.Time // used to skip excessive warnings

	doneCh chan struct{} // closed when all goroutines have exited
}

type errorReporterClient interface {
	ReportErrorEvent(ctx context.Context, req *errorreportingpb.ReportErrorEventRequest, opts ...gax.CallOption) (*errorreportingpb.ReportErrorEventResponse, error)
	Close() error
}

// NewCloudErrorReporter constructs a CloudErrorReporter sink.
//
// It must be closed via Close() when no longer used. The context is used to do
// the initial setup and, for logging Cloud Error Reporter's own errors, and for
// running internal uploader goroutines.
func NewCloudErrorReporter(ctx context.Context, opts ...option.ClientOption) (*CloudErrorReporter, error) {
	client, err := vkit.NewReportErrorsClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return newCloudErrorReporter(ctx, client), nil
}

// newCloudErrorReporter is used in tests to mock the client.
func newCloudErrorReporter(ctx context.Context, client errorReporterClient) *CloudErrorReporter {
	ch := make(chan *ErrorReport, maxPendingReports)

	rep := &CloudErrorReporter{
		client: client,
		ctx:    ctx,
		ch:     ch,
		doneCh: make(chan struct{}),
	}

	var wg sync.WaitGroup
	wg.Add(uploaderGoroutines)
	for range uploaderGoroutines {
		go func() {
			defer wg.Done()
			for report := range ch {
				if err := rep.reportOne(report); err != nil {
					rep.throttledWarn(fmt.Sprintf("Cloud Error Reporter submission error: %s", err))
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(rep.doneCh)
	}()

	return rep
}

// ReportError asynchronously uploads the error report.
func (r *CloudErrorReporter) ReportError(rep *ErrorReport) {
	r.chM.Lock()
	defer r.chM.Unlock()
	if r.ch == nil {
		r.throttledWarn("Not reporting an error, since CloudErrorReporter is already stopped")
		r.dropped++
	} else {
		select {
		case r.ch <- rep:
		default:
			r.throttledWarn("Too many pending unreported errors")
			r.dropped++
		}
	}
}

// Close flushes pending errors and stops the sink.
func (r *CloudErrorReporter) Close(ctx context.Context) {
	r.chM.Lock()
	ch := r.ch
	r.ch = nil
	r.chM.Unlock()
	if ch == nil {
		return // already stopped
	}

	ctx, cancel := context.WithTimeout(ctx, flushTimeout)
	defer cancel()

	// Notify all goroutines to stop.
	close(ch)

	// Wait for all of them to stop or for a timeout.
	select {
	case <-r.doneCh:
		// Can safely close the client now, since there will be no calls anymore.
		_ = r.client.Close()
	case <-ctx.Done():
		logging.Warningf(r.ctx, "Timeout waiting for Cloud Error Reporter flush, giving up")
	}
}

// reportOne submits one error report.
func (r *CloudErrorReporter) reportOne(rep *ErrorReport) error {
	msg := rep.Message
	if rep.RequestContext != nil && rep.RequestContext.TraceID != "" {
		msg += fmt.Sprintf(" (Log Trace ID: %s)", rep.RequestContext.TraceID)
	}
	// As long as rep.Stack is well-formed stack trace, Cloud Error Reporting will
	// recognize it and extract source location from it. No need to do it
	// manually.
	msg += "\n" + rep.Stack

	var httpReq *errorreportingpb.HttpRequestContext
	if rep.RequestContext != nil {
		httpReq = &errorreportingpb.HttpRequestContext{
			Method:    rep.RequestContext.HTTPMethod,
			Url:       rep.RequestContext.URL,
			UserAgent: rep.RequestContext.UserAgent,
			RemoteIp:  rep.RequestContext.RemoteIP,
		}
	}

	_, err := r.client.ReportErrorEvent(r.ctx, &errorreportingpb.ReportErrorEventRequest{
		ProjectName: "projects/" + rep.ServiceContext.Project,
		Event: &errorreportingpb.ReportedErrorEvent{
			EventTime: timestamppb.New(rep.Timestamp),
			ServiceContext: &errorreportingpb.ServiceContext{
				Service: rep.ServiceContext.Service,
				Version: rep.ServiceContext.Version,
			},
			Message: msg,
			Context: &errorreportingpb.ErrorContext{
				HttpRequest: httpReq,
				User:        rep.User,
			},
		},
	})
	return err
}

// throttledWarn logs a warning, throttling them to once per 5 sec.
//
// Note that this must be a warning (not an error) to avoid accidental recursion
// (since error log lines eventually end up in CloudErrorReporter again).
func (r *CloudErrorReporter) throttledWarn(msg string) {
	r.warnM.Lock()
	defer r.warnM.Unlock()

	shouldWarn := msg != r.lastWarnMsg ||
		r.lastWarnTime.IsZero() ||
		time.Since(r.lastWarnTime) > warnThottling

	if shouldWarn {
		logging.Warningf(r.ctx, "%s", msg)
		r.lastWarnMsg = msg
		r.lastWarnTime = time.Now()
	}
}
