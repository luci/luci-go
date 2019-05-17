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
	"io"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"

	. "github.com/smartystreets/goconvey/convey"
)

func use(ctx context.Context, out io.Writer, proto LogEntry) context.Context {
	return logging.SetFactory(ctx, Factory(&Sink{Out: out}, proto))
}

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

	Convey("Basic", t, func() {
		c = use(c, buf, LogEntry{TraceID: "hi"})
		logging.Infof(c, "test context")
		So(read(buf), ShouldResemble, &LogEntry{
			Message:  "test context",
			Severity: "info",
			Time:     "1454472306.7",
			TraceID:  "hi", // copied from the prototype
		})
	})

	Convey("Simple fields", t, func() {
		c = use(c, buf, LogEntry{})
		logging.NewFields(map[string]interface{}{"foo": "bar"}).Infof(c, "test field")
		e := read(buf)
		So(e.Fields["foo"], ShouldEqual, "bar")
	})

	Convey("Error field", t, func() {
		c = use(c, buf, LogEntry{})
		c = logging.SetField(c, "foo", "bar")
		logging.WithError(fmt.Errorf("boom")).Infof(c, "boom")
		e := read(buf)
		So(e.Fields["foo"], ShouldEqual, "bar")             // still works
		So(e.Fields[logging.ErrorKey], ShouldEqual, "boom") // also works
	})
}
