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

package middleware

import (
	"time"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/server/router"
)

// WithContextTimeout returns a middleware that adds a timeout to the context.
func WithContextTimeout(timeout time.Duration) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		ctx, cancel := clock.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		next(c)
	}
}
