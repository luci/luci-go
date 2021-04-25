// Copyright 2021 The LUCI Authors.
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

package router

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
)

// WithContextTimeout returns a middleware that adds a timeout to the context.
func WithContextTimeout(timeout time.Duration) Middleware {
	return func(c *Context, next Handler) {
		var cancel context.CancelFunc
		c.Context, cancel = clock.WithTimeout(c.Context, timeout)
		defer cancel()
		next(ctx)
	}
}

// WithPanicCatcher is a middleware that catches panics, dumps stack trace to
// logging and returns HTTP 500.
func WithPanicCatcher(c *Context, next Handler) {
	ctx := c.Context
	w := c.Writer
	uri := c.Request.RequestURI
	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		// Log the reason before the stack in case appengine cuts entire log
		// message due to size limitations.
		logging.Fields{
			"panic.error": p.Reason,
		}.Errorf(ctx, "Caught panic during handling of %q: %s\n%s", uri, p.Reason, p.Stack)

		// Note: it may be too late to send HTTP 500 if `next` already sent
		// headers. But there's nothing else we can do at this point anyway.
		http.Error(w, "Internal Server Error. See logs.", http.StatusInternalServerError)
	})
	next(c)
}
