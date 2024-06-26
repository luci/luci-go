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

package templates

import (
	"fmt"
	"net/http"

	"go.chromium.org/luci/server/router"
)

// WithTemplates is middleware that lazily loads template bundle and injects it
// into the context.
//
// Wrapper reply with HTTP 500 if templates can not be loaded. Inner handler
// receives context with all templates successfully loaded.
func WithTemplates(b *Bundle) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		// Note: Use(...) calls EnsureLoaded and initializes b.err inside.
		c.Request = c.Request.WithContext(Use(c.Request.Context(), b, &Extra{
			Request: c.Request,
			Params:  c.Params,
		}))
		if b.err != nil {
			http.Error(c.Writer, fmt.Sprintf("Can't load HTML templates.\n%s", b.err), http.StatusInternalServerError)
			return
		}
		next(c)
	}
}
