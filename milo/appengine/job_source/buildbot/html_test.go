// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/net/context"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/milo/appengine/common"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	. "github.com/smartystreets/goconvey/convey"
)

func request(c context.Context, params map[string]string) *router.Context {
	p := httprouter.Params{}
	for k, v := range params {
		p = append(p, httprouter.Param{Key: k, Value: v})
	}
	r := &http.Request{URL: &url.URL{Path: "/foobar"}}
	c = common.WithRequest(c, r)
	w := httptest.NewRecorder()
	return &router.Context{
		Context: c,
		Params:  p,
		Writer:  w,
		Request: r,
	}
}

func TestHtml(t *testing.T) {
	c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	c = templates.Use(c, common.GetTemplateBundle("../../frontend/templates"))
	c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})
	putDSMasterJSON(c, &buildbotMaster{
		Name:     "fake",
		Builders: map[string]*buildbotBuilder{"fake": {}},
	}, false)

	Convey(`HTML handler tests`, t, func() {
		Convey(`Build pages`, func() {
			Convey(`Empty request`, func() {
				rc := request(c, nil)
				BuildHandler(rc)
				response := rc.Writer.(*httptest.ResponseRecorder)
				So(response.Code, ShouldEqual, http.StatusBadRequest)
			})
			Convey(`Missing builder`, func() {
				rc := request(c, map[string]string{"master": "foo"})
				BuildHandler(rc)
				response := rc.Writer.(*httptest.ResponseRecorder)
				So(response.Code, ShouldEqual, http.StatusBadRequest)
			})
			Convey(`Missing build`, func() {
				rc := request(c, map[string]string{"master": "foo", "builder": "bar"})
				BuildHandler(rc)
				response := rc.Writer.(*httptest.ResponseRecorder)
				So(response.Code, ShouldEqual, http.StatusBadRequest)
			})
			Convey(`Invalid build number`, func() {
				rc := request(c, map[string]string{"master": "foo", "builder": "bar", "build": "baz"})
				BuildHandler(rc)
				response := rc.Writer.(*httptest.ResponseRecorder)
				So(response.Code, ShouldEqual, http.StatusBadRequest)
			})
		})

		Convey(`Builder pages`, func() {
			Convey(`Empty request`, func() {
				rc := request(c, nil)
				BuilderHandler(rc)
				response := rc.Writer.(*httptest.ResponseRecorder)
				So(response.Code, ShouldEqual, http.StatusBadRequest)
			})
			Convey(`Missing builder`, func() {
				rc := request(c, map[string]string{"master": "foo"})
				BuilderHandler(rc)
				response := rc.Writer.(*httptest.ResponseRecorder)
				So(response.Code, ShouldEqual, http.StatusBadRequest)
			})
			Convey(`Builder not found`, func() {
				rc := request(c, map[string]string{"master": "fake", "builder": "newline"})
				BuilderHandler(rc)
				response := rc.Writer.(*httptest.ResponseRecorder)
				So(response.Code, ShouldEqual, http.StatusNotFound)
			})
		})
	})
}
