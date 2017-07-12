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

package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/eventlog"
	logpb "github.com/luci/luci-go/common/eventlog/proto"
)

// fakeLogger implements syncLogger by storing method arguments for later inspection.
type fakeLogger struct {
	events []*eventlog.ChromeInfraLogEvent
}

func (fl *fakeLogger) LogSync(ctx context.Context, events ...*eventlog.ChromeInfraLogEvent) error {
	fl.events = append(fl.events, events...)
	return nil
}

func defaultEvent() *eventlog.ChromeInfraLogEvent {
	point := logpb.ChromeInfraEvent_POINT
	return &eventlog.ChromeInfraLogEvent{
		LogEvent: &logpb.LogRequestLite_LogEventLite{
			EventTimeMs: proto.Int64(10),
		},
		InfraEvent: &logpb.ChromeInfraEvent{
			TimestampKind: &point,
		},
	}
}

func (fl *fakeLogger) NewLogEvent(ctx context.Context, eventTime eventlog.TypedTime) *eventlog.ChromeInfraLogEvent {
	return defaultEvent()
}

func fakeGetEnvValue(key string) *string {
	if key == "" {
		return nil
	}

	ret := fmt.Sprintf("%s value", key)
	return &ret
}

func TestLogSync(t *testing.T) {
	flogger := &fakeLogger{}
	eventlogger := &IsolateEventLogger{client: flogger, getEnvironmentValue: fakeGetEnvValue}
	ctx := context.Background()
	op := logpb.IsolateClientEvent_ARCHIVE.Enum()
	const startUsec = 100
	const endUsec = 200
	start := time.Unix(0, startUsec*1000)
	end := time.Unix(0, endUsec*1000)

	details := &logpb.IsolateClientEvent_ArchiveDetails{
		HitCount:    proto.Int64(1),
		MissCount:   proto.Int64(2),
		HitBytes:    proto.Int64(4),
		MissBytes:   proto.Int64(8),
		IsolateHash: []string{"hash brown"},
	}

	wantEvent := defaultEvent()
	wantEvent.InfraEvent.IsolateClientEvent = &logpb.IsolateClientEvent{
		Binary: &logpb.Binary{
			Name:          proto.String("isolate"),
			VersionNumber: proto.String(version),
		},
		Operation:      op,
		ArchiveDetails: details,
		Master:         proto.String("BUILDBOT_MASTERNAME value"),
		Builder:        proto.String("BUILDBOT_BUILDERNAME value"),
		BuildId:        proto.String("BUILDBOT_BUILDNUMBER value"),
		Slave:          proto.String("BUILDBOT_SLAVENAME value"),
		StartTsUsec:    proto.Int64(startUsec),
		EndTsUsec:      proto.Int64(endUsec),
	}

	err := eventlogger.logStats(ctx, op, start, end, details)
	if err != nil {
		t.Fatalf("IsolateEventLogger.logStats: got err %v; want %v", err, nil)
	}
	if got, want := flogger.events, []*eventlog.ChromeInfraLogEvent{wantEvent}; !reflect.DeepEqual(got, want) {
		t.Errorf("IsolateEventLogger.logStats: got %v; want %v", got, want)
	}
}
