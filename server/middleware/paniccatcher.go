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

package middleware

import (
	"net/http"

	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/runtime/paniccatcher"
	"github.com/luci/luci-go/server/router"
)

// WithPanicCatcher is a middleware that catches panics, dumps stack trace to
// logging and returns HTTP 500.
func WithPanicCatcher(c *router.Context, next router.Handler) {
	ctx := c.Context
	w := c.Writer
	req := c.Request
	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		// Log the reason before the stack in case appengine cuts entire log
		// message due to size limitations.
		log.Fields{
			"panic.error": p.Reason,
		}.Errorf(ctx, "Caught panic during handling of %q: %s\n%s", req.RequestURI, p.Reason, p.Stack)

		// Note: it may be too late to send HTTP 500 if `next` already sent
		// headers. But there's nothing else we can do at this point anyway.
		http.Error(w, "Internal Server Error. See logs.", http.StatusInternalServerError)
	})
	next(c)
}
