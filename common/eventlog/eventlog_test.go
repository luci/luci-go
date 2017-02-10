// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package eventlog

import (
	"reflect"
	"testing"
	"time"

	logpb "github.com/luci/luci-go/common/eventlog/proto"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

func TestGeneratesLogEvent(t *testing.T) {
	c := Client{EventSource: &logpb.InfraEventSource{}}

	var tNano int64 = 1e9
	begin := logpb.ChromeInfraEvent_BEGIN
	et := TypedTime{
		time.Unix(0, tNano),
		begin,
	}
	want := &ChromeInfraLogEvent{
		LogEvent: &logpb.LogRequestLite_LogEventLite{
			EventTimeMs: proto.Int64(tNano / 1e6),
		},
		InfraEvent: &logpb.ChromeInfraEvent{
			TimestampKind: &begin,
			EventSource:   c.EventSource,
		},
	}

	got := c.NewLogEvent(context.Background(), et)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("generated log event: got: %v; want: %v", got, want)
	}
}
