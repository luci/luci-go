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

package xsrf

import (
	"context"
	"encoding/base64"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
)

func TestXsrf(t *testing.T) {
	ftt.Run("Token + Check", t, func(t *ftt.Test) {
		c := makeContext()
		tok, err := Token(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, Check(c, tok), should.BeNil)
		assert.Loosely(t, Check(c, tok+"abc"), should.ErrLike(base64.CorruptInputError(76)))
	})

	ftt.Run("TokenField works", t, func(t *ftt.Test) {
		c := makeContext()
		assert.Loosely(t, TokenField(c), should.Resemble(
			template.HTML("<input type=\"hidden\" name=\"xsrf_token\" "+
				"value=\"AXsiX2kiOiIxNDQyMjcwNTIwMDAwIn1ceiDv1yfNK9OHcdb209l3fM4p_gn-Uaembaa8gr3WXg\">")))
	})

	ftt.Run("Middleware works", t, func(t *ftt.Test) {
		c := makeContext()
		tok, _ := Token(c)

		h := func(c *router.Context) {
			c.Writer.Write([]byte("hi"))
		}
		mc := router.NewMiddlewareChain(WithTokenCheck)

		// Has token -> works.
		rec := httptest.NewRecorder()
		req := makeRequest(c, tok)
		router.RunMiddleware(&router.Context{
			Writer:  rec,
			Request: req,
		}, mc, h)
		assert.Loosely(t, rec.Code, should.Equal(200))

		// No token.
		rec = httptest.NewRecorder()
		req = makeRequest(c, "")
		router.RunMiddleware(&router.Context{
			Writer:  rec,
			Request: req,
		}, mc, h)
		assert.Loosely(t, rec.Code, should.Equal(403))

		// Bad token.
		rec = httptest.NewRecorder()
		req = makeRequest(c, "blah")
		router.RunMiddleware(&router.Context{
			Writer:  rec,
			Request: req,
		}, mc, h)
		assert.Loosely(t, rec.Code, should.Equal(403))
	})
}

func makeContext() context.Context {
	c := secrets.Use(context.Background(), &testsecrets.Store{})
	c, _ = testclock.UseTime(c, time.Unix(1442270520, 0))
	return c
}

func makeRequest(ctx context.Context, tok string) *http.Request {
	body := url.Values{}
	if tok != "" {
		body.Add("xsrf_token", tok)
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://example.com", strings.NewReader(body.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return req
}
