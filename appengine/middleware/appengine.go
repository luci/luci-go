// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

var devAppserverBypassFn = func(c context.Context) bool {
	return info.Get(c).IsDevAppServer()
}

// RequireCron ensures that this handler was run from the appengine 'cron'
// service. Otherwise it aborts the request with a StatusForbidden.
//
// This middleware has no effect when using 'BaseTest' or when running under
// dev_appserver.py
func RequireCron(h Handler) Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if !devAppserverBypassFn(c) {
			if r.Header.Get("X-Appengine-Cron") != "true" {
				rw.WriteHeader(http.StatusForbidden)
				logging.Errorf(c, "request not made from cron")
				fmt.Fprint(rw, "error: must be run from cron")
				return
			}
		}
		h(c, rw, r, p)
	}
}

// RequireTaskQueue ensures that this handler was run from the specified
// appengine 'taskqueue' queue. Otherwise it aborts the request with
// a StatusForbidden.
//
// if `queue` is the empty string, than this simply checks that this handler was
// run from ANY appengine taskqueue.
//
// This middleware has no effect when using 'BaseTest' or when running under
// dev_appserver.py
func RequireTaskQueue(queue string, h Handler) Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if !devAppserverBypassFn(c) {
			qName := r.Header.Get("X-AppEngine-QueueName")
			if qName == "" || (queue != "" && queue != qName) {
				rw.WriteHeader(http.StatusForbidden)
				logging.Errorf(c, "request made from wrong taskqueue: %q v %q", qName, queue)
				fmt.Fprintf(rw, "error: must be run from the correct taskqueue")
				return
			}
		}
		h(c, rw, r, p)
	}
}
