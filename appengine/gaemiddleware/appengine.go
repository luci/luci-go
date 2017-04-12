// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gaemiddleware

import (
	"net/http"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/router"
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
