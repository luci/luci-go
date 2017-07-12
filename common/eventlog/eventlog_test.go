// Copyright 2016 The LUCI Authors.
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
