// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package info

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/server/middleware"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFetchServiceInfo(t *testing.T) {
	Convey("Works", t, func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{
				"app_id": "some-app-id",
				"app_runtime": "go",
				"app_runtime_version": "go1.5.1",
				"app_version": "1234-abcdef",
				"service_account_name": "some-app-id@appspot.gserviceaccount.com"
			}`))
		}))
		info, err := FetchServiceInfo(context.Background(), ts.URL)
		So(err, ShouldBeNil)
		So(info, ShouldResemble, &ServiceInfo{
			AppID:              "some-app-id",
			AppRuntime:         "go",
			AppRuntimeVersion:  "go1.5.1",
			AppVersion:         "1234-abcdef",
			ServiceAccountName: "some-app-id@appspot.gserviceaccount.com",
		})
	})

	Convey("Error", t, func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "fail", http.StatusInternalServerError)
		}))
		info, err := FetchServiceInfo(context.Background(), ts.URL)
		So(info, ShouldBeNil)
		So(err, ShouldNotBeNil)
	})
}

func TestInstallHandlers(t *testing.T) {
	Convey("Works", t, func() {
		c := context.Background()
		router := httprouter.New()
		returnErr := false

		InstallHandlers(router, middleware.TestingBase(c), func(context.Context) (ServiceInfo, error) {
			if returnErr {
				return ServiceInfo{}, errors.New("fail")
			}
			return ServiceInfo{
				AppID:              "some-app-id",
				AppRuntime:         "go",
				AppRuntimeVersion:  "go1.5.1",
				AppVersion:         "1234-abcdef",
				ServiceAccountName: "some-app-id@appspot.gserviceaccount.com",
			}, nil
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/auth/api/v1/server/info", nil)
		router.ServeHTTP(w, req)
		So(w.Code, ShouldEqual, 200)
		So(w.Body.String(), ShouldResemble,
			`{"app_id":"some-app-id","app_runtime":"go",`+
				`"app_runtime_version":"go1.5.1",`+
				`"app_version":"1234-abcdef","service_account_name":`+
				`"some-app-id@appspot.gserviceaccount.com"}`+"\n")

		returnErr = true
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/auth/api/v1/server/info", nil)
		router.ServeHTTP(w, req)
		So(w.Code, ShouldEqual, 500)
		So(w.Body.String(), ShouldResemble, "{\"error\":\"fail\"}\n")
	})
}
