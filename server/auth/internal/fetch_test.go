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

package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	RegisterClientFactory(func(ctx context.Context, scopes []string) (*http.Client, error) {
		return http.DefaultClient, nil
	})
}

func TestFetch(t *testing.T) {
	ftt.Run("with test context", t, func(c *ftt.Test) {
		body := ""
		status := http.StatusOK
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(body))
		}))
		defer ts.Close()

		ctx := context.Background()

		c.Run("fetch works", func(c *ftt.Test) {
			var val struct {
				A string `json:"a"`
			}
			body = `{"a": "hello"}`
			req := Request{
				Method: "GET",
				URL:    ts.URL,
				Out:    &val,
			}
			assert.Loosely(c, req.Do(ctx), should.BeNil)
			assert.Loosely(c, val.A, should.Equal("hello"))
		})

		c.Run("handles bad status code", func(c *ftt.Test) {
			var val struct{}
			status = http.StatusNotFound
			req := Request{
				Method: "GET",
				URL:    ts.URL,
				Out:    &val,
			}
			assert.Loosely(c, req.Do(ctx), should.ErrLike("HTTP code (404)"))
		})

		c.Run("handles bad body", func(c *ftt.Test) {
			var val struct{}
			body = "not json"
			req := Request{
				Method: "GET",
				URL:    ts.URL,
				Out:    &val,
			}
			assert.Loosely(c, req.Do(ctx), should.ErrLike("can't deserialize JSON"))
		})

		c.Run("handles connection error", func(c *ftt.Test) {
			var val struct{}
			req := Request{
				Method: "GET",
				URL:    "http://localhost:12345678",
				Out:    &val,
			}
			assert.Loosely(c, req.Do(ctx), should.ErrLike("dial tcp"))
		})
	})
}
