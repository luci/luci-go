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
	"regexp"
	"testing"

	"cloud.google.com/go/errorreporting"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
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

type fakeCloudErrorsSink struct {
	CloudErrorsSink
	errRptEntry *errorreporting.Entry
}

func (f *fakeCloudErrorsSink) Write(l *LogEntry) {
	if l.Severity == ErrorSeverity {
		errRptEntry := prepErrorReportingEntry(l, nil)
		f.errRptEntry = &errRptEntry
	}
	f.Out.Write(l)
}

func newFakeCloudErrorsSink(out io.Writer) *fakeCloudErrorsSink {
	return &fakeCloudErrorsSink{CloudErrorsSink: CloudErrorsSink{Out: &Sink{Out: out}}}
}

func useLog(ctx context.Context, fakeSink *fakeCloudErrorsSink, proto LogEntry) context.Context {
	return logging.SetFactory(ctx, Factory(fakeSink, proto, nil))
}

func TestErrorReporting(t *testing.T) {
	t.Parallel()

	ftt.Run("errStackRe regex match", t, func(t *ftt.Test) {
		errStr := "original error: rpc error: code = Internal desc = internal: attaching a status: rpc error: code = FailedPrecondition desc = internal"
		stackStr := `goroutine 27693:
#0 go.chromium.org/luci/grpc/appstatus/status.go:59 - appstatus.Attach()
  reason: attaching a status
  tag["application-specific response status"]: &status.Status{s:(*status.Status)(0xc002885e60)}
`
		msg := errStr + "\n\n" + stackStr
		match := errStackRe.FindStringSubmatch(msg)
		assert.Loosely(t, match, should.NotBeNil)
		assert.Loosely(t, match[1], should.Equal(errStr))
		assert.Loosely(t, match[2], should.Equal(stackStr))
	})

	ftt.Run("end to end", t, func(t *ftt.Test) {
		c := context.Background()
		c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
		buf := bytes.NewBuffer([]byte{})

		t.Run("logging error with full stack", func(t *ftt.Test) {
			fakeErrSink := newFakeCloudErrorsSink(buf)
			c = useLog(c, fakeErrSink, LogEntry{TraceID: "trace123"})

			errors.Log(c, errors.New("test error"))

			// assert errorreporting.entry has the stack from errors.renderStack().
			assert.Loosely(t, fakeErrSink.errRptEntry.Error.Error(), should.Equal("original error: test error (Log Trace ID: trace123)"))
			stackMatch, err := regexp.MatchString(`goroutine \d+:\n.*sdlogger.TestErrorReporting.func*`, string(fakeErrSink.errRptEntry.Stack))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, stackMatch, should.BeTrue)

			// assert outputted LogEntry.message
			logOutput := read(buf)
			logMsgMatch, err := regexp.MatchString(`original error: test error\n\ngoroutine \d+:\n.*sdlogger.TestErrorReporting.func*`, logOutput.Message)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logMsgMatch, should.BeTrue)
		})

		t.Run("logging error without stack", func(t *ftt.Test) {
			fakeErrSink := newFakeCloudErrorsSink(buf)
			c = useLog(c, fakeErrSink, LogEntry{TraceID: "trace123"})

			logging.Errorf(c, "test error")

			assert.Loosely(t, fakeErrSink.errRptEntry.Error.Error(), should.Equal("test error (Log Trace ID: trace123)"))
			assert.Loosely(t, fakeErrSink.errRptEntry.Stack, should.NotBeNil)
			assert.Loosely(t, read(buf), should.Resemble(&LogEntry{
				Message:   "test error",
				Severity:  ErrorSeverity,
				Timestamp: Timestamp{Seconds: 1454472306, Nanos: 7},
				TraceID:   "trace123",
			}))
		})

		t.Run("logging non-error", func(t *ftt.Test) {
			fakeErrSink := newFakeCloudErrorsSink(buf)
			c = useLog(c, fakeErrSink, LogEntry{TraceID: "trace123"})

			logging.Infof(c, "info")

			assert.Loosely(t, fakeErrSink.errRptEntry, should.BeNil)
			assert.Loosely(t, read(buf), should.Resemble(&LogEntry{
				Message:   "info",
				Severity:  InfoSeverity,
				Timestamp: Timestamp{Seconds: 1454472306, Nanos: 7},
				TraceID:   "trace123",
			}))
		})
	})
}

func TestCleanupStack(t *testing.T) {
	t.Parallel()

	call := func(s string) string {
		return string(cleanupStack([]byte(s)))
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		stack := `goroutine 19 [running]:
go.chromium.org/luci/common/logging/sdlogger.prepErrorReportingEntry(0xc0001abea0, 0x0)
	zzz/go.chromium.org/luci/common/logging/sdlogger/logger.go:210 +0x1d9
go.chromium.org/luci/common/logging/sdlogger.(*fakeCloudErrorsSink).Write(0xc0001913b0, 0xc0001abea0)
	zzz/infra/go/src/go.chromium.org/luci/common/logging/sdlogger/logger_test.go:91 +0x65
go.chromium.org/luci/common/logging/sdlogger.(*jsonLogger).LogCall(0xc0001da0c0, 0x3, 0x0?, {0x1683678, 0xa}, {0x0, 0x0, 0x0})
	zzz/infra/go/src/go.chromium.org/luci/common/logging/sdlogger/logger.go:311 +0x2f2
go.chromium.org/luci/common/logging.Errorf({0x176d680?, 0xc0001913e0?}, {0x1683678, 0xa}, {0x0, 0x0, 0x0})
	zzz/infra/go/src/go.chromium.org/luci/common/logging/exported.go:50 +0x71
go.chromium.org/luci/common/some/package.SomeCall.func2.2()
	zzz/infra/go/src/go.chromium.org/luci/common/some/package/file.go:150 +0x166
`

		expected := `goroutine 19 [running]:
go.chromium.org/luci/common/some/package.SomeCall.func2.2()
	zzz/infra/go/src/go.chromium.org/luci/common/some/package/file.go:150 +0x166
`

		assert.Loosely(t, call(stack), should.Equal(expected))
	})

	ftt.Run("Skips unexpected stuff", t, func(t *ftt.Test) {
		assert.Loosely(t, call(""), should.BeEmpty)
		assert.Loosely(t, call("abc"), should.Equal("abc"))
		assert.Loosely(t, call("abc\ndef"), should.Equal("abc\ndef"))
	})
}

// A fuzz test to ensure cleanupStack doesn't panic.

func FuzzCleanupStack(f *testing.F) {
	stack := `goroutine 19 [running]:
go.chromium.org/luci/common/logging/sdlogger.prepErrorReportingEntry(0xc0001abea0, 0x0)
	zzz/go.chromium.org/luci/common/logging/sdlogger/logger.go:210 +0x1d9
go.chromium.org/luci/common/logging/sdlogger.(*fakeCloudErrorsSink).Write(0xc0001913b0, 0xc0001abea0)
	zzz/infra/go/src/go.chromium.org/luci/common/logging/sdlogger/logger_test.go:91 +0x65
go.chromium.org/luci/common/logging/sdlogger.(*jsonLogger).LogCall(0xc0001da0c0, 0x3, 0x0?, {0x1683678, 0xa}, {0x0, 0x0, 0x0})
	zzz/infra/go/src/go.chromium.org/luci/common/logging/sdlogger/logger.go:311 +0x2f2
go.chromium.org/luci/common/logging.Errorf({0x176d680?, 0xc0001913e0?}, {0x1683678, 0xa}, {0x0, 0x0, 0x0})
	zzz/infra/go/src/go.chromium.org/luci/common/logging/exported.go:50 +0x71
go.chromium.org/luci/common/some/package.SomeCall.func2.2()
	zzz/infra/go/src/go.chromium.org/luci/common/some/package/file.go:150 +0x166
`
	f.Add(stack)
	f.Fuzz(func(t *testing.T, s string) {
		out := string(cleanupStack([]byte(s)))
		switch {
		case len(out) > len(s):
			t.Errorf("output is larger than input:\n%q\n%q", s, out)
		case len(out) == len(s):
			if out != s {
				t.Errorf("unexpected mutation:\n%q\n%q", s, out)
			}
		}
	})
}
