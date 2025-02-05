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

package sdlogger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func use(ctx context.Context, out io.Writer, proto LogEntry) context.Context {
	return logging.SetFactory(ctx, Factory(&Sink{Out: out}, proto, nil))
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

	ftt.Run("Basic", t, func(t *ftt.Test) {
		c = use(c, buf, LogEntry{TraceID: "hi"})
		logging.Infof(c, "test context")
		assert.Loosely(t, read(buf), should.Resemble(&LogEntry{
			Message:   "test context",
			Severity:  InfoSeverity,
			Timestamp: Timestamp{Seconds: 1454472306, Nanos: 7},
			TraceID:   "hi", // copied from the prototype
		}))
	})

	ftt.Run("Simple fields", t, func(t *ftt.Test) {
		c = use(c, buf, LogEntry{})
		logging.NewFields(map[string]any{"foo": "bar"}).Infof(c, "test field")
		e := read(buf)
		assert.Loosely(t, e.Fields["foo"], should.Equal("bar"))
		assert.Loosely(t, e.Message, should.Equal(`test field :: {"foo":"bar"}`))
	})

	ftt.Run("Error field", t, func(t *ftt.Test) {
		c = use(c, buf, LogEntry{})
		c = logging.SetField(c, "foo", "bar")
		logging.WithError(fmt.Errorf("boom")).Infof(c, "boom")
		e := read(buf)
		assert.Loosely(t, e.Fields["foo"], should.Equal("bar"))             // still works
		assert.Loosely(t, e.Fields[logging.ErrorKey], should.Equal("boom")) // also works
		assert.Loosely(t, e.Message, should.Equal(`boom :: {"error":"boom", "foo":"bar"}`))
	})
}
