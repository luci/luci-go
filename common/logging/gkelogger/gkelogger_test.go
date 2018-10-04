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

package gkelogger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"

	. "github.com/smartystreets/goconvey/convey"
)

func read(b *bytes.Buffer) *LogEntry {
	var result LogEntry
	if err := json.NewDecoder(b).Decode(&result); err != nil {
		panic(fmt.Errorf("could not decode `%s`: %q", b.Bytes(), err))
	}
	return &result
}

func TestLogger(t *testing.T) {
	t.Parallel()
	c := context.Background()
	c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
	buf := bytes.NewBuffer([]byte{})

	Convey("Basic Debug", t, func() {
		l := jsonLogger{ctx: c, out: buf, lock: &sync.Mutex{}}
		l.Debugf("test message")
		So(read(buf), ShouldResemble, &LogEntry{
			Message:  "test message",
			Severity: "debug",
			Time:     clock.Now(c).Format(time.RFC3339Nano),
		})
	})

	Convey("Basic in context", t, func() {
		c = Use(c, buf)
		logging.Infof(c, "test context")
		So(read(buf), ShouldResemble, &LogEntry{
			Message:  "test context",
			Severity: "info",
			Time:     clock.Now(c).Format(time.RFC3339Nano),
		})
	})

	Convey("Basic in Fields", t, func() {
		c = Use(c, buf)
		logging.NewFields(map[string]interface{}{"foo": "bar"}).Infof(c, "test field")
		e := read(buf)
		So(e.Fields["foo"], ShouldEqual, "bar")
	})
}
