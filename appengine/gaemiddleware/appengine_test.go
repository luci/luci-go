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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/router"
)

func init() {
	// disable this so that we can actually check the logic in these middlewares
	devAppserverBypassFn = func(context.Context) bool { return false }
}

func TestRequireCron(t *testing.T) {
	t.Parallel()

	ftt.Run("Test RequireCron", t, func(t *ftt.Test) {
		hit := false
		f := func(c *router.Context) {
			hit = true
			c.Writer.Write([]byte("ok"))
		}

		t.Run("from non-cron fails", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Writer:  rec,
				Request: (&http.Request{}).WithContext(gaetesting.TestingContext()),
			}
			router.RunMiddleware(c, router.NewMiddlewareChain(RequireCron), f)
			assert.Loosely(t, hit, should.BeFalse)
			assert.Loosely(t, rec.Body.String(), should.Equal("error: must be run from cron\n"))
			assert.Loosely(t, rec.Code, should.Equal(http.StatusForbidden))
		})

		t.Run("from cron succeeds", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Writer: rec,
				Request: (&http.Request{
					Header: http.Header{
						http.CanonicalHeaderKey("x-appengine-cron"): []string{"true"},
					},
				}).WithContext(gaetesting.TestingContext()),
			}
			router.RunMiddleware(c, router.NewMiddlewareChain(RequireCron), f)
			assert.Loosely(t, hit, should.BeTrue)
			assert.Loosely(t, rec.Body.String(), should.Equal("ok"))
			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
		})
	})
}

func TestRequireTQ(t *testing.T) {
	t.Parallel()

	ftt.Run("Test RequireTQ", t, func(t *ftt.Test) {
		hit := false
		f := func(c *router.Context) {
			hit = true
			c.Writer.Write([]byte("ok"))
		}

		t.Run("from non-tq fails (wat)", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Writer:  rec,
				Request: (&http.Request{}).WithContext(gaetesting.TestingContext()),
			}
			router.RunMiddleware(c, router.NewMiddlewareChain(RequireTaskQueue("wat")), f)
			assert.Loosely(t, hit, should.BeFalse)
			assert.Loosely(t, rec.Body.String(), should.Equal("error: must be run from the correct taskqueue\n"))
			assert.Loosely(t, rec.Code, should.Equal(http.StatusForbidden))
		})

		t.Run("from non-tq fails", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Writer:  rec,
				Request: (&http.Request{}).WithContext(gaetesting.TestingContext()),
			}
			router.RunMiddleware(c, router.NewMiddlewareChain(RequireTaskQueue("")), f)
			assert.Loosely(t, hit, should.BeFalse)
			assert.Loosely(t, rec.Body.String(), should.Equal("error: must be run from the correct taskqueue\n"))
			assert.Loosely(t, rec.Code, should.Equal(http.StatusForbidden))
		})

		t.Run("from wrong tq fails (wat)", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Writer: rec,
				Request: (&http.Request{
					Header: http.Header{
						http.CanonicalHeaderKey("x-appengine-queuename"): []string{"else"},
					},
				}).WithContext(gaetesting.TestingContext()),
			}
			router.RunMiddleware(c, router.NewMiddlewareChain(RequireTaskQueue("wat")), f)
			assert.Loosely(t, hit, should.BeFalse)
			assert.Loosely(t, rec.Body.String(), should.Equal("error: must be run from the correct taskqueue\n"))
			assert.Loosely(t, rec.Code, should.Equal(http.StatusForbidden))
		})

		t.Run("from right tq succeeds (wat)", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Writer: rec,
				Request: (&http.Request{
					Header: http.Header{
						http.CanonicalHeaderKey("x-appengine-queuename"): []string{"wat"},
					},
				}).WithContext(gaetesting.TestingContext()),
			}
			router.RunMiddleware(c, router.NewMiddlewareChain(RequireTaskQueue("wat")), f)
			assert.Loosely(t, hit, should.BeTrue)
			assert.Loosely(t, rec.Body.String(), should.Equal("ok"))
			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
		})

		t.Run("from any tq succeeds", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Writer: rec,
				Request: (&http.Request{
					Header: http.Header{
						http.CanonicalHeaderKey("x-appengine-queuename"): []string{"wat"},
					},
				}).WithContext(gaetesting.TestingContext()),
			}
			router.RunMiddleware(c, router.NewMiddlewareChain(RequireTaskQueue("")), f)
			assert.Loosely(t, hit, should.BeTrue)
			assert.Loosely(t, rec.Body.String(), should.Equal("ok"))
			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
		})
	})
}
