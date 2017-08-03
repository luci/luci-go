// Copyright 2017 The LUCI Authors.
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

package coordinatorTest

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/gae/service/info"
	tq "github.com/luci/gae/service/taskqueue"

	"golang.org/x/net/context"
)

func drainTaskQueues(c context.Context, h http.Handler) {
	now := clock.Now(c)

	logging.Debugf(c, "Running task(s) for namespace %q...", info.GetNamespace(c))
	tst := tq.GetTestable(c)
	for queue, tasks := range tst.GetScheduledTasks() {
		for taskName, task := range tasks {
			if task.ETA.After(now) {
				continue
			}

			logging.Debugf(c, "Running queue %q, task %q...", queue, taskName)
			fakeTaskQueueHTTP(c, h, task)
			if err := tq.Delete(c, queue, task); err != nil {
				panic(fmt.Errorf("could not delete task %q: %s", taskName, err))
			}
		}
	}
}

func fakeTaskQueueHTTP(c context.Context, h http.Handler, task *tq.Task) {
	var rw fakeResponseWriter
	req := http.Request{
		Method: task.Method,
		URL: &url.URL{
			Scheme: "fake",
			Host:   "localhost",
			Path:   task.Path,
		},
		Body: ioutil.NopCloser(bytes.NewReader(task.Payload)),
	}

	h.ServeHTTP(&rw, &req)
}

type fakeResponseWriter struct {
	code int
	h    http.Header
	buf  bytes.Buffer
}

func (rw *fakeResponseWriter) Header() http.Header {
	if rw.h == nil {
		rw.h = http.Header{}
	}
	return rw.h
}

func (rw *fakeResponseWriter) Write(p []byte) (int, error) { return rw.buf.Write(p) }
func (rw *fakeResponseWriter) WriteHeader(code int)        { rw.code = code }
