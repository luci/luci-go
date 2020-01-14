// Copyright 2020 The LUCI Authors.
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
	"fmt"
	"net/http"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

// RequireHost returns Middleware that rejects a request with HTTP 400 unless
// it has an expected host (exact match).
func RequireHost(host string) router.Middleware {
	if host == "" {
		panic("host is empty")
	}

	return func(c *router.Context, next router.Handler) {
		if c.Request.Host != host {
			logging.Warningf(c.Context, "Rejected a request with host %q; expected %q", c.Request.Host, host)
			fmt.Fprintf(c.Writer, "Unexpected request host %q; expected %q", c.Request.Host, host)
			c.Writer.WriteHeader(http.StatusBadRequest)
		} else {
			next(c)
		}
	}
}
