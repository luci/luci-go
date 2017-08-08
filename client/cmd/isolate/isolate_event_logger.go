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
	"os"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/eventlog"
	logpb "go.chromium.org/luci/common/eventlog/proto"
)

type syncLogger interface {
	LogSync(ctx context.Context, events ...*eventlog.ChromeInfraLogEvent) error
	NewLogEvent(ctx context.Context, eventTime eventlog.TypedTime) *eventlog.ChromeInfraLogEvent
}

// An IsolateEventLogger logs eventlogs which contain stats about the data uploaded to the isolate server.
type IsolateEventLogger struct {
	client syncLogger

	// getEnvironmentValue looks up the value of an environment variable.
	// Set this to override the default implementation for testing.
	getEnvironmentValue func(key string) *string
}

// NewLogger returns an IsolateEventLogger which logs to the specified endpoint.
func NewLogger(ctx context.Context, endpoint string) *IsolateEventLogger {
	l := &IsolateEventLogger{}
	if host := eventlogEndpoint(endpoint); host != "" {
		l.client = eventlog.NewClient(ctx, host)
	}
	return l
}

// logStats synchronously logs an eventlog which describes an isolate run.
func (l *IsolateEventLogger) logStats(ctx context.Context, op *logpb.IsolateClientEvent_Operation, start, end time.Time, archiveDetails *logpb.IsolateClientEvent_ArchiveDetails) error {
	if l.client == nil {
		return nil
	}
	bi := l.getBuildbotInfo()
	event := l.client.NewLogEvent(ctx, eventlog.Point())
	event.InfraEvent.IsolateClientEvent = &logpb.IsolateClientEvent{
		Binary: &logpb.Binary{
			Name:          proto.String("isolate"),
			VersionNumber: proto.String(version),
		},
		Operation:      op,
		ArchiveDetails: archiveDetails,
		Master:         bi.master,
		Builder:        bi.builder,
		BuildId:        bi.buildID,
		Slave:          bi.slave,
		StartTsUsec:    proto.Int64(int64(start.UnixNano() / 1e3)),
		EndTsUsec:      proto.Int64(int64(end.UnixNano() / 1e3)),
	}
	return l.client.LogSync(ctx, event)
}

func eventlogEndpoint(endpointFlag string) string {
	switch endpointFlag {
	case "test":
		return eventlog.TestEndpoint
	case "prod":
		return eventlog.ProdEndpoint
	default:
		return endpointFlag
	}
}

// buildbotInfo contains information about the build in which this command was run.
type buildbotInfo struct {
	// Variables which are not present in the environment are nil.
	master, builder, buildID, slave *string
}

// getBuildbotInfo poulates a buildbotInfo with information from the environment.
func (l *IsolateEventLogger) getBuildbotInfo() *buildbotInfo {
	return &buildbotInfo{
		master:  l.getEnvValue("BUILDBOT_MASTERNAME"),
		builder: l.getEnvValue("BUILDBOT_BUILDERNAME"),
		buildID: l.getEnvValue("BUILDBOT_BUILDNUMBER"),
		slave:   l.getEnvValue("BUILDBOT_SLAVENAME"),
	}
}

func (l *IsolateEventLogger) getEnvValue(key string) *string {
	if l.getEnvironmentValue != nil {
		return l.getEnvironmentValue(key)
	}
	if val, ok := os.LookupEnv(key); ok {
		return &val
	}
	return nil
}
