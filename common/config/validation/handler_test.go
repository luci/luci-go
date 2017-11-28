// Copyright 2017 The LUCI Authors.
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

package validation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInstallHandlers(t *testing.T) {
	metaCall := func(r *router.Router, v *Validator) *httptest.ResponseRecorder {
		req, err := http.NewRequest("GET", metadataPath, nil)
		So(err, ShouldBeNil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}
	valCall := func(r *router.Router, v *Validator, configSet, path, content string) *httptest.ResponseRecorder {
		respBodyJSON, err := json.Marshal(map[string]interface{}{"config_set": configSet, "path": path, "content": []byte(content)})
		So(err, ShouldBeNil)
		req, err := http.NewRequest("POST", validationPath, strings.NewReader(string(respBodyJSON)))
		So(err, ShouldBeNil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	Convey("Initialization of validation routes and handlers", t, func() {
		v := &Validator{Func: mockValidationFunction}
		r := router.New()
		InstallHandlers(r, router.NewMiddlewareChain(), v)

		Convey("Basic metadataHandler call", func() {
			rr := metaCall(r, v)
			So(rr.Code, ShouldEqual, 200)
			var resp map[string]interface{}
			json.NewDecoder(rr.Body).Decode(&resp)
			So(len(resp), ShouldEqual, 2)
		})

		Convey("metadataHandler call with patterns", func() {
			cp, err := pattern.Parse("configSet")
			So(err, ShouldBeNil)
			pp, err := pattern.Parse("path")
			So(err, ShouldBeNil)
			v.ConfigPatterns = append(v.ConfigPatterns, &ConfigPattern{ConfigSet: cp, Path: pp})
			rr := metaCall(r, v)
			So(rr.Code, ShouldEqual, 200)
		})

		Convey("Basic validationHandler call", func() {
			rr := valCall(r, v, "dead", "beef", "")
			So(rr.Code, ShouldEqual, 200)
			var resp map[string][]map[string]string
			json.NewDecoder(rr.Body).Decode(&resp)
			So(len(resp["messages"]), ShouldEqual, 1)
			So(len(resp["messages"][0]), ShouldEqual, 2)
			So(resp["messages"][0]["text"], ShouldEqual, "deadbeef")
			So(resp["messages"][0]["severity"], ShouldEqual, "ERROR")
		})

		Convey("validationHandler call with no configSet or path", func() {
			rr := valCall(r, v, "", "", "")
			So(rr.Code, ShouldEqual, 400)
		})

		Convey("validationHandler call with no path", func() {
			rr := valCall(r, v, "dead", "", "")
			So(rr.Code, ShouldEqual, 400)
		})
	})
}

func mockValidationFunction(configSet, path, content string, ctx *Context) {
	ctx.errors = append(ctx.errors, errors.New("deadbeef"))
}
