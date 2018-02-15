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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"

	. "github.com/smartystreets/goconvey/convey"
)

func read(b *bytes.Buffer) *LogEntry {
	var result LogEntry
	if err := json.NewDecoder(b).Decode(&result); err != nil {
		panic(fmt.Errorf("could not decode '%s': %s", b.String(), err))
	}
	return &result
}

func TestLogger(t *testing.T) {
	c := context.Background()
	c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
	Convey("Basic Debug", t, func() {
		buf := bytes.NewBuffer([]byte{})
		l := jsonLogger{ctx: c, out: buf}
		l.Debugf("test %s", logging.Debug)
		So(read(buf), ShouldResemble, &LogEntry{
			Message:  "test debug",
			Severity: "debug",
			Time:     clock.Now(c).Format(time.RFC3339Nano),
		})
	})
	Convey("Basic in context", t, func() {
		buf := bytes.NewBuffer([]byte{})
		c = Use(c, buf)
		logging.Infof(c, "test context")
		So(read(buf), ShouldResemble, &LogEntry{
			Message:  "test context",
			Severity: "info",
			Time:     clock.Now(c).Format(time.RFC3339Nano),
		})
	})
	Convey("Basic in Fields", t, func() {
		buf := bytes.NewBuffer([]byte{})
		c = Use(c, buf)
		logging.NewFields(map[string]interface{}{"foo": "bar"}).Infof(c, "test field")
		e := read(buf)
		So(e.Fields["foo"], ShouldEqual, "bar")
	})
}
