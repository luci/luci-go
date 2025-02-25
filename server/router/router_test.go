// Copyright 2016 The LUCI Authors.
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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRouter(t *testing.T) {
	t.Parallel()

	outputKey := "output"
	client := &http.Client{}
	appendValue := func(c context.Context, key string, val string) context.Context {
		var current []string
		if v := c.Value(key); v != nil {
			current = v.([]string)
		}
		return context.WithValue(c, key, append(current, val))
	}
	a := func(c *Context, next Handler) {
		c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "a:before"))
		next(c)
		c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "a:after"))
	}
	b := func(c *Context, next Handler) {
		c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "b:before"))
		next(c)
		c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "b:after"))
	}
	c := func(c *Context, next Handler) {
		c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "c"))
		next(c)
	}
	d := func(c *Context, next Handler) {
		next(c)
		c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "d"))
	}
	stop := func(_ *Context, _ Handler) {}
	handler := func(c *Context) {
		c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "handler"))
	}

	ftt.Run("Router", t, func(t *ftt.Test) {
		r := New()

		t.Run("New", func(t *ftt.Test) {
			t.Run("Should initialize non-nil httprouter.Router", func(t *ftt.Test) {
				assert.Loosely(t, r.hrouter, should.NotBeNil)
			})
		})

		t.Run("Use", func(t *ftt.Test) {
			t.Run("Should append middleware", func(t *ftt.Test) {
				assert.Loosely(t, len(r.middleware), should.BeZero)
				r.Use(NewMiddlewareChain(a, b))
				assert.Loosely(t, len(r.middleware), should.Equal(2))
			})
		})

		t.Run("Subrouter", func(t *ftt.Test) {
			t.Run("Should create new router with values from original router", func(t *ftt.Test) {
				r.BasePath = "/foo"
				r.Use(NewMiddlewareChain(a, b))
				r2 := r.Subrouter("bar")
				assert.Loosely(t, r.hrouter, should.Equal(r2.hrouter))
				assert.Loosely(t, r2.BasePath, should.Equal("/foo/bar"))
				assert.Loosely(t, r.middleware, should.Match(r2.middleware))
			})
		})

		t.Run("Handle", func(t *ftt.Test) {
			t.Run("Should not modify existing empty r.middleware slice", func(t *ftt.Test) {
				assert.Loosely(t, len(r.middleware), should.BeZero)
				r.Handle("GET", "/bar", NewMiddlewareChain(b, c), handler)
				assert.Loosely(t, len(r.middleware), should.BeZero)
			})

			t.Run("Should not modify existing r.middleware slice", func(t *ftt.Test) {
				r.Use(NewMiddlewareChain(a))
				assert.Loosely(t, len(r.middleware), should.Equal(1))
				r.Handle("GET", "/bar", NewMiddlewareChain(b, c), handler)
				assert.Loosely(t, len(r.middleware), should.Equal(1))
			})
		})

		t.Run("run", func(t *ftt.Test) {
			ctx := &Context{
				Request: &http.Request{},
			}

			t.Run("Should execute handler when using nil middlewares", func(t *ftt.Test) {
				run(ctx, nil, nil, handler)
				assert.Loosely(t, ctx.Request.Context().Value(outputKey), should.Match([]string{"handler"}))
			})

			t.Run("Should execute middlewares and handler in order", func(t *ftt.Test) {
				m := NewMiddlewareChain(a, b, c)
				n := NewMiddlewareChain(d)
				run(ctx, m, n, handler)
				assert.Loosely(t, ctx.Request.Context().Value(outputKey), should.Match(
					[]string{"a:before", "b:before", "c", "handler", "d", "b:after", "a:after"},
				))
			})

			t.Run("Should not execute upcoming middleware/handlers if next is not called", func(t *ftt.Test) {
				mc := NewMiddlewareChain(a, stop, b)
				run(ctx, mc, NewMiddlewareChain(), handler)
				assert.Loosely(t, ctx.Request.Context().Value(outputKey), should.Match([]string{"a:before", "a:after"}))
			})

			t.Run("Should execute next middleware when it encounters nil middleware", func(t *ftt.Test) {
				t.Run("At start of first chain", func(t *ftt.Test) {
					run(ctx, NewMiddlewareChain(nil, a), NewMiddlewareChain(b), handler)
					assert.Loosely(t, ctx.Request.Context().Value(outputKey), should.Match([]string{"a:before", "b:before", "handler", "b:after", "a:after"}))
				})
				t.Run("At start of second chain", func(t *ftt.Test) {
					run(ctx, NewMiddlewareChain(a), NewMiddlewareChain(nil, b), handler)
					assert.Loosely(t, ctx.Request.Context().Value(outputKey), should.Match([]string{"a:before", "b:before", "handler", "b:after", "a:after"}))
				})
				t.Run("At end of first chain", func(t *ftt.Test) {
					run(ctx, NewMiddlewareChain(a, nil), NewMiddlewareChain(b), handler)
					assert.Loosely(t, ctx.Request.Context().Value(outputKey), should.Match([]string{"a:before", "b:before", "handler", "b:after", "a:after"}))
				})
				t.Run("At end of second chain", func(t *ftt.Test) {
					run(ctx, NewMiddlewareChain(a), NewMiddlewareChain(b, nil), handler)
					assert.Loosely(t, ctx.Request.Context().Value(outputKey), should.Match([]string{"a:before", "b:before", "handler", "b:after", "a:after"}))
				})
			})
		})

		t.Run("ServeHTTP", func(t *ftt.Test) {
			ts := httptest.NewServer(r)
			a := func(c *Context, next Handler) {
				c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "a:before"))
				next(c)
				c.Request = c.Request.WithContext(appendValue(c.Request.Context(), outputKey, "a:after"))
				io.WriteString(c.Writer, strings.Join(c.Request.Context().Value(outputKey).([]string), ","))
			}

			t.Run("Should execute middleware registered in Use and Handle in order", func(t *ftt.Test) {
				r.Use(NewMiddlewareChain(a))
				r.GET("/ab", NewMiddlewareChain(b), handler)
				res, err := client.Get(ts.URL + "/ab")
				assert.Loosely(t, err, should.BeNil)
				defer res.Body.Close()
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				p, err := io.ReadAll(res.Body)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(p), should.Equal(strings.Join(
					[]string{"a:before", "b:before", "handler", "b:after", "a:after"},
					",",
				)))
			})

			t.Run("Should return method not allowed for existing path but wrong method in request", func(t *ftt.Test) {
				r.POST("/comment", NewMiddlewareChain(), handler)
				res, err := client.Get(ts.URL + "/comment")
				assert.Loosely(t, err, should.BeNil)
				defer res.Body.Close()
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusMethodNotAllowed))
			})

			t.Run("Should return expected response from handler", func(t *ftt.Test) {
				handler := func(c *Context) {
					c.Writer.Write([]byte("Hello, " + c.Params[0].Value))
				}
				r.GET("/hello/:name", NewMiddlewareChain(c, d), handler)
				res, err := client.Get(ts.URL + "/hello/世界")
				assert.Loosely(t, err, should.BeNil)
				defer res.Body.Close()
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				p, err := io.ReadAll(res.Body)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(p), should.Equal("Hello, 世界"))
			})

			t.Cleanup(func() {
				ts.Close()
			})
		})

		t.Run("makeBasePath", func(t *ftt.Test) {
			cases := []struct{ base, relative, result string }{
				{"/", "", "/"},
				{"", "", "/"},
				{"foo", "", "/foo"},
				{"foo/", "", "/foo/"},
				{"foo", "/", "/foo/"},
				{"foo", "bar", "/foo/bar"},
				{"foo/", "bar", "/foo/bar"},
				{"foo", "bar/", "/foo/bar/"},
				{"/foo", "/bar", "/foo/bar"},
				{"/foo/", "/bar", "/foo/bar"},
				{"foo//", "///bar", "/foo/bar"},
				{"foo", "bar///baz/qux", "/foo/bar/baz/qux"},
				{"//foo//", "///bar///baz/qux/", "/foo/bar/baz/qux/"},
			}
			for _, c := range cases {
				assert.Loosely(t, makeBasePath(c.base, c.relative), should.Equal(c.result))
			}
		})
	})
}
