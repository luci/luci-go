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

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Basic", t, func() {
		c = use(c, buf, LogEntry{TraceID: "hi"})
		logging.Infof(c, "test context")
		So(read(buf), ShouldResemble, &LogEntry{
			Message:   "test context",
			Severity:  InfoSeverity,
			Timestamp: Timestamp{Seconds: 1454472306, Nanos: 7},
			TraceID:   "hi", // copied from the prototype
		})
	})

	Convey("Simple fields", t, func() {
		c = use(c, buf, LogEntry{})
		logging.NewFields(map[string]any{"foo": "bar"}).Infof(c, "test field")
		e := read(buf)
		So(e.Fields["foo"], ShouldEqual, "bar")
		So(e.Message, ShouldEqual, `test field :: {"foo":"bar"}`)
	})

	Convey("Error field", t, func() {
		c = use(c, buf, LogEntry{})
		c = logging.SetField(c, "foo", "bar")
		logging.WithError(fmt.Errorf("boom")).Infof(c, "boom")
		e := read(buf)
		So(e.Fields["foo"], ShouldEqual, "bar")             // still works
		So(e.Fields[logging.ErrorKey], ShouldEqual, "boom") // also works
		So(e.Message, ShouldEqual, `boom :: {"error":"boom", "foo":"bar"}`)
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

	Convey("errStackRe regex match", t, func() {
		errStr := "original error: rpc error: code = Internal desc = internal: attaching a status: rpc error: code = FailedPrecondition desc = internal"
		stackStr := `goroutine 27693:
#0 go.chromium.org/luci/grpc/appstatus/status.go:59 - appstatus.Attach()
  reason: attaching a status
  tag["application-specific response status"]: &status.Status{s:(*status.Status)(0xc002885e60)}
`
		msg := errStr + "\n\n" + stackStr
		match := errStackRe.FindStringSubmatch(msg)
		So(match, ShouldNotBeNil)
		So(match[1], ShouldEqual, errStr)
		So(match[2], ShouldEqual, stackStr)
	})

	Convey("end to end", t, func() {
		c := context.Background()
		c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
		buf := bytes.NewBuffer([]byte{})

		Convey("logging error with full stack", func() {
			fakeErrSink := newFakeCloudErrorsSink(buf)
			c = useLog(c, fakeErrSink, LogEntry{TraceID: "trace123"})

			errors.Log(c, errors.New("test error"))

			// assert errorreporting.entry has the stack from errors.renderStack().
			So(fakeErrSink.errRptEntry.Error.Error(), ShouldEqual, "original error: test error (Log Trace ID: trace123)")
			stackMatch, err := regexp.MatchString(`goroutine \d+:\n.*sdlogger.TestErrorReporting.func*`, string(fakeErrSink.errRptEntry.Stack))
			So(err, ShouldBeNil)
			So(stackMatch, ShouldBeTrue)

			// assert outputted LogEntry.message
			logOutput := read(buf)
			logMsgMatch, err := regexp.MatchString(`original error: test error\n\ngoroutine \d+:\n.*sdlogger.TestErrorReporting.func*`, logOutput.Message)
			So(err, ShouldBeNil)
			So(logMsgMatch, ShouldBeTrue)
		})

		Convey("logging error without stack", func() {
			fakeErrSink := newFakeCloudErrorsSink(buf)
			c = useLog(c, fakeErrSink, LogEntry{TraceID: "trace123"})

			logging.Errorf(c, "test error")

			So(fakeErrSink.errRptEntry.Error.Error(), ShouldEqual, "test error (Log Trace ID: trace123)")
			So(fakeErrSink.errRptEntry.Stack, ShouldNotBeNil)
			So(read(buf), ShouldResemble, &LogEntry{
				Message:   "test error",
				Severity:  ErrorSeverity,
				Timestamp: Timestamp{Seconds: 1454472306, Nanos: 7},
				TraceID:   "trace123",
			})
		})

		Convey("logging non-error", func() {
			fakeErrSink := newFakeCloudErrorsSink(buf)
			c = useLog(c, fakeErrSink, LogEntry{TraceID: "trace123"})

			logging.Infof(c, "info")

			So(fakeErrSink.errRptEntry, ShouldBeNil)
			So(read(buf), ShouldResemble, &LogEntry{
				Message:   "info",
				Severity:  InfoSeverity,
				Timestamp: Timestamp{Seconds: 1454472306, Nanos: 7},
				TraceID:   "trace123",
			})
		})
	})
}

func TestCleanupStack(t *testing.T) {
	t.Parallel()

	call := func(s string) string {
		return string(cleanupStack([]byte(s)))
	}

	Convey("Works", t, func() {
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

		So(call(stack), ShouldEqual, expected)
	})

	Convey("Skips unexpected stuff", t, func() {
		So(call(""), ShouldEqual, "")
		So(call("abc"), ShouldEqual, "abc")
		So(call("abc\ndef"), ShouldEqual, "abc\ndef")
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
