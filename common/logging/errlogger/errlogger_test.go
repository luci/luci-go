// Copyright 2025 The LUCI Authors.
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

package errlogger

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestErrLogger(t *testing.T) {
	t.Parallel()

	svcCtx := &ServiceContext{
		Project: "proj",
		Service: "service",
		Version: "ver",
	}
	reqCtx := &RequestContext{
		HTTPMethod: "GET",
		URL:        "http://example.com",
		UserAgent:  "user-agent",
		RemoteIP:   "remote-ip",
		TraceID:    "trace-id",
	}

	t.Run("Skips non-errors", func(t *testing.T) {
		ctx := logging.SetFactory(context.Background(), Factory(&Config{
			Sink: callbackSink(func(rep *ErrorReport) {
				t.Errorf("Unexpectedly called: %v", rep)
			}),
			ServiceContext: svcCtx,
		}, reqCtx))
		logging.Debugf(ctx, "Debug")
		logging.Infof(ctx, "Info")
		logging.Warningf(ctx, "Warning")
	})

	t.Run("Captures errors", func(t *testing.T) {
		var lastReport *ErrorReport
		ctx := logging.SetFactory(context.Background(), Factory(&Config{
			Sink: callbackSink(func(rep *ErrorReport) {
				lastReport = rep
			}),
			ServiceContext: svcCtx,
			UserResolver: func(ctx context.Context) string {
				return "resolved-user"
			},
		}, reqCtx))

		testTime := time.Date(2044, time.April, 4, 4, 4, 0, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, testTime)

		logging.Errorf(ctx, "Boom")

		assert.That(t, lastReport, should.Match(&ErrorReport{
			ServiceContext: svcCtx,
			RequestContext: reqCtx,
			User:           "resolved-user",
			Timestamp:      testTime,
			Message:        "Boom",
			Stack:          lastReport.Stack,
		}))

		// The stack is present.
		assert.That(t, lastReport.Stack, should.NotEqual(""))
	})

	t.Run("Panic catcher e2e", func(t *testing.T) {
		var lastReport *ErrorReport
		ctx := logging.SetFactory(context.Background(), Factory(&Config{
			Sink: callbackSink(func(rep *ErrorReport) {
				lastReport = rep
			}),
			ServiceContext: svcCtx,
		}, reqCtx))

		paniccatcher.Do(func() {
			panic("BOOM")
		}, func(p *paniccatcher.Panic) {
			p.Log(ctx, "Crashed")
		})

		assert.That(t, lastReport.Message, should.Equal("Crashed"))
		assert.That(t, lastReport.Stack, should.MatchRegexp(`panic\(.*\)`))
	})

	t.Run("errors.Log e2e", func(t *testing.T) {
		var lastReport *ErrorReport
		ctx := logging.SetFactory(context.Background(), Factory(&Config{
			Sink: callbackSink(func(rep *ErrorReport) {
				lastReport = rep
			}),
			ServiceContext: svcCtx,
		}, reqCtx))

		errors.Log(ctx, errors.Reason("Error reason").Err())

		assert.That(t, lastReport.Message, should.Equal("original error: Error reason"))
		assert.That(t, lastReport.Stack, should.MatchRegexp(`errlogger_test.go\:116`))
	})
}

type callbackSink func(rep *ErrorReport)

func (cbs callbackSink) ReportError(rep *ErrorReport) { cbs(rep) }

func TestCleanupStack(t *testing.T) {
	t.Parallel()

	t.Run("Works", func(t *testing.T) {
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

		assert.That(t, cleanupStack([]byte(stack)), should.Equal(expected))
	})

	t.Run("Skips unexpected stuff", func(t *testing.T) {
		assert.That(t, cleanupStack([]byte("")), should.Equal(""))
		assert.That(t, cleanupStack([]byte("abc")), should.Equal("abc"))
		assert.That(t, cleanupStack([]byte("abc\ndef")), should.Equal("abc\ndef"))
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
		out := cleanupStack([]byte(s))
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
