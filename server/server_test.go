// Copyright 2019 The LUCI Authors.
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

package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gkelogger"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		stdoutLogs := logsRecorder{}
		stderrLogs := logsRecorder{}

		srv := New(Options{
			Prod:      true,
			HTTPAddr:  "main_addr",
			AdminAddr: "admin_addr",

			testCtx:    ctx,
			testSeed:   1,
			testStdout: &stdoutLogs,
			testStderr: &stderrLogs,

			// Bind to auto-assigned ports.
			testListeners: map[string]net.Listener{
				"main_addr":  setupListener(),
				"admin_addr": setupListener(),
			},
		})

		mainPort := srv.opts.testListeners["main_addr"].Addr().(*net.TCPAddr).Port
		mainAddr := fmt.Sprintf("http://127.0.0.1:%d", mainPort)

		srv.Routes.GET("/test", router.MiddlewareChain{}, func(c *router.Context) {
			logging.Infof(c.Context, "Info log")
			tc.Add(time.Second)
			logging.Warningf(c.Context, "Warn log")
			c.Writer.WriteHeader(404)
			c.Writer.Write([]byte("Hello, world"))
		})

		// Run the serving loop in parallel.
		done := make(chan error)
		go func() { done <- srv.ListenAndServe() }()

		// Call "/test" to verify the server can serve successfully.
		resp, err := httpGet(mainAddr+"/test", map[string]string{
			"User-Agent":            "Test-user-agent",
			"X-Cloud-Trace-Context": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/00001;trace=TRUE",
			"X-Forwarded-For":       "1.1.1.1,2.2.2.2,3.3.3.3",
		})
		So(err, ShouldBeNil)
		So(resp, ShouldEqual, "Hello, world")

		// Stderr log captures details about the request.
		So(stderrLogs.Last(1), ShouldResemble, []gkelogger.LogEntry{
			{
				Severity: "warning",
				Time:     "1454472307.7",
				TraceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				RequestInfo: &gkelogger.RequestInfo{
					Method:       "GET",
					URL:          mainAddr + "/test",
					Status:       404,
					RequestSize:  "0",
					ResponseSize: "12", // len("Hello, world")
					UserAgent:    "Test-user-agent",
					RemoteIP:     "2.2.2.2",
					Latency:      "1.000000s",
				},
			},
		})
		// Stdout log captures individual log lines.
		So(stdoutLogs.Last(2), ShouldResemble, []gkelogger.LogEntry{
			{
				Severity: "info",
				Message:  "Info log",
				Time:     "1454472306.7",
				TraceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Operation: &gkelogger.Operation{
					ID: "9566c74d10037c4d7bbb0407d1e2c649",
				},
			},
			{
				Severity: "warning",
				Message:  "Warn log",
				Time:     "1454472307.7",
				TraceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Operation: &gkelogger.Operation{
					ID: "9566c74d10037c4d7bbb0407d1e2c649",
				},
			},
		})

		// Make sure Shutdown works.
		srv.Shutdown()
		So(<-done, ShouldBeNil)
	})
}

func setupListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	return l
}

func httpGet(addr string, headers map[string]string) (resp string, err error) {
	req, err := http.NewRequest("GET", addr, nil)
	if err != nil {
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	blob, err := ioutil.ReadAll(res.Body)
	return string(blob), err
}

type logsRecorder struct {
	m    sync.Mutex
	logs []gkelogger.LogEntry
}

func (r *logsRecorder) Write(e *gkelogger.LogEntry) {
	r.m.Lock()
	r.logs = append(r.logs, *e)
	r.m.Unlock()
}

func (r *logsRecorder) Last(n int) []gkelogger.LogEntry {
	entries := make([]gkelogger.LogEntry, n)
	r.m.Lock()
	copy(entries, r.logs[len(r.logs)-n:])
	r.m.Unlock()
	return entries
}
