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
	"testing"
	"time"

	"cloud.google.com/go/errorreporting/apiv1beta1/errorreportingpb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCloudErrorReporter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svcCtx := &ServiceContext{
		Project: "proj",
		Service: "service",
		Version: "ver",
	}
	reqCtx := &RequestContext{
		HTTPMethod: "GET",
		URL:        "http://example.com",
		UserAgent:  "user-agent",
		RemoteIP:   "remote-ip",
		TraceID:    "trace-id",
	}

	testTime := time.Date(2044, time.April, 4, 4, 4, 0, 0, time.UTC)

	t.Run("Reports one", func(t *testing.T) {
		client := &mockedClient{}
		rep := newCloudErrorReporter(ctx, client)
		rep.ReportError(&ErrorReport{
			ServiceContext: svcCtx,
			RequestContext: reqCtx,
			Timestamp:      testTime,
			User:           "user",
			Message:        "Boom",
			Stack:          "stack",
		})
		rep.Close(ctx)
		assert.That(t, client.closed, should.BeTrue)

		// A request looks fine.
		assert.That(t, client.reqs[0], should.Match(&errorreportingpb.ReportErrorEventRequest{
			ProjectName: "projects/proj",
			Event: &errorreportingpb.ReportedErrorEvent{
				EventTime: timestamppb.New(testTime),
				ServiceContext: &errorreportingpb.ServiceContext{
					Service: "service",
					Version: "ver",
				},
				Message: "Boom (Log Trace ID: trace-id)\nstack",
				Context: &errorreportingpb.ErrorContext{
					HttpRequest: &errorreportingpb.HttpRequestContext{
						Method:    "GET",
						Url:       "http://example.com",
						UserAgent: "user-agent",
						RemoteIp:  "remote-ip",
					},
					User: "user",
				},
			},
		}))
	})

	t.Run("Reports many", func(t *testing.T) {
		client := &mockedClient{}
		rep := newCloudErrorReporter(ctx, client)
		for i := range maxPendingReports {
			rep.ReportError(&ErrorReport{
				ServiceContext: svcCtx,
				RequestContext: reqCtx,
				Timestamp:      testTime,
				Message:        fmt.Sprintf("%d", i),
				Stack:          "stack",
			})
		}
		rep.Close(ctx)
		assert.That(t, client.closed, should.BeTrue)

		// Got all reports, no dups.
		assert.Loosely(t, client.reqs, should.HaveLength(maxPendingReports))
		seen := stringset.New(0)
		for _, req := range client.reqs {
			seen.Add(req.Event.Message)
		}
		assert.That(t, seen.Len(), should.Equal(maxPendingReports))
	})

	t.Run("Drops when blocked", func(t *testing.T) {
		unblock := make(chan struct{})
		ready := make(chan struct{})

		client := &mockedClient{
			cb: func() {
				ready <- struct{}{}
				<-unblock
			},
		}

		rep := newCloudErrorReporter(ctx, client)
		submit := func() {
			rep.ReportError(&ErrorReport{
				ServiceContext: svcCtx,
				RequestContext: reqCtx,
				Message:        "msg",
				Stack:          "stack",
			})
		}

		// First fill in all executors.
		for range uploaderGoroutines {
			submit()
			<-ready
		}

		// Now all reports are being buffered with an excess dropped.
		for range maxPendingReports + 10 {
			submit()
		}
		// Indeed excess was dropped.
		assert.That(t, rep.dropped, should.Equal(int64(10)))

		// Let it all exit.
		close(unblock)
		go func() {
			for range ready {
			}
		}()
		rep.Close(ctx)
		assert.That(t, client.closed, should.BeTrue)

		// Drops any new reports now.
		submit()
		assert.That(t, rep.dropped, should.Equal(int64(11)))
	})
}

type mockedClient struct {
	m      sync.Mutex
	reqs   []*errorreportingpb.ReportErrorEventRequest
	closed bool
	cb     func()
}

func (mc *mockedClient) ReportErrorEvent(ctx context.Context, req *errorreportingpb.ReportErrorEventRequest, opts ...gax.CallOption) (*errorreportingpb.ReportErrorEventResponse, error) {
	mc.m.Lock()
	mc.reqs = append(mc.reqs, req)
	mc.m.Unlock()
	if mc.cb != nil {
		mc.cb()
	}
	return nil, nil
}

func (mc *mockedClient) Close() error {
	mc.m.Lock()
	mc.closed = true
	mc.m.Unlock()
	return nil
}
