// Copyright 2015 The LUCI Authors.
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

package gaemiddleware

import (
	"net/http"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

var devAppserverBypassFn = info.IsDevAppServer

// RequireCron ensures that the request is from the appengine 'cron' service.
//
// It checks the presence of a magical header that can be set only by GAE.
// If the header is not there, it aborts the request with StatusForbidden.
//
// This middleware has no effect when using 'BaseTest' or when running under
// dev_appserver.py
func RequireCron(c *router.Context, next router.Handler) {
	if !devAppserverBypassFn(c.Context) {
		if c.Request.Header.Get("X-Appengine-Cron") != "true" {
			logging.Errorf(c.Context, "request not made from cron")
			http.Error(c.Writer, "error: must be run from cron", http.StatusForbidden)
			return
		}
	}
	next(c)
}

// RequireTaskQueue ensures that the request is from the specified task queue.
//
// It checks the presence of a magical header that can be set only by GAE.
// If the header is not there, it aborts the request with StatusForbidden.
//
// if 'queue' is the empty string, than this simply checks that this handler was
// run from ANY appengine taskqueue.
//
// This middleware has no effect when using 'BaseTest' or when running under
// dev_appserver.py
func RequireTaskQueue(queue string) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		if !devAppserverBypassFn(c.Context) {
			qName := c.Request.Header.Get("X-AppEngine-QueueName")
			if qName == "" || (queue != "" && queue != qName) {
				logging.Errorf(c.Context, "request made from wrong taskqueue: %q v %q", qName, queue)
				http.Error(c.Writer, "error: must be run from the correct taskqueue", http.StatusForbidden)
				return
			}
		}
		next(c)
	}
}
