// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package xsrf

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/server/secrets/testsecrets"

	. "github.com/smartystreets/goconvey/convey"
)

func TestXsrf(t *testing.T) {
	Convey("Token + Check", t, func() {
		c := makeContext()
		tok, err := Token(c)
		So(err, ShouldBeNil)
		So(Check(c, tok), ShouldBeNil)
		So(Check(c, tok+"abc"), ShouldNotBeNil)
	})

	Convey("TokenField works", t, func() {
		c := makeContext()
		So(TokenField(c), ShouldResemble,
			template.HTML("<input type=\"hidden\" name=\"xsrf_token\" "+
				"value=\"AXsiX2kiOiIxNDQyMjcwNTIwMDAwIn1ceiDv1yfNK9OHcdb209l3fM4p_gn-Uaembaa8gr3WXg\">"))
	})

	Convey("Middleware works", t, func() {
		c := makeContext()
		tok, _ := Token(c)

		h := WithTokenCheck(func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
			rw.Write([]byte("hi"))
		})

		// Has token -> works.
		rec := httptest.NewRecorder()
		req := makeRequest(tok)
		h(c, rec, req, nil)
		So(rec.Code, ShouldEqual, 200)

		// No token.
		rec = httptest.NewRecorder()
		req = makeRequest("")
		h(c, rec, req, nil)
		So(rec.Code, ShouldEqual, 403)

		// Bad token.
		rec = httptest.NewRecorder()
		req = makeRequest("blah")
		h(c, rec, req, nil)
		So(rec.Code, ShouldEqual, 403)
	})
}

func makeContext() context.Context {
	c := testsecrets.Use(context.Background())
	c, _ = testclock.UseTime(c, time.Unix(1442270520, 0))
	return c
}

func makeRequest(tok string) *http.Request {
	body := url.Values{}
	if tok != "" {
		body.Add("xsrf_token", tok)
	}
	req, _ := http.NewRequest("POST", "https://example.com", strings.NewReader(body.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return req
}
