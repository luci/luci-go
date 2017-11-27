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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/data/text/pattern"
)

func TestInstallHandlers(t *testing.T) {
	metaCall := func(v *Validator) (*httptest.ResponseRecorder, error) {
		req, err := http.NewRequest("GET", metadataPath, nil)
		if err != nil {
			return nil, err
		}
		rr := httptest.NewRecorder()
		r := router.New()

		InstallHandlers(r, router.NewMiddlewareChain(), v)
		r.ServeHTTP(rr, req)
		return rr, nil
	}

	valCall := func(v *Validator, configSet, path, content string) (*httptest.ResponseRecorder, error) {
		respBodyJSON, err := json.Marshal(map[string]string{"config_set": configSet, "path": path, "content": content})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest("POST", validationPath, strings.NewReader(string(respBodyJSON)))
		if err != nil {
			return nil, err
		}
		rr := httptest.NewRecorder()
		r := router.New()
		InstallHandlers(r, router.NewMiddlewareChain(), v)
		r.ServeHTTP(rr, req)
		return rr, nil
	}

	v := &Validator{Function: mockValidationFunction}
	Convey("Basic metadataHandler call", t, func() {
		rr, err := metaCall(v)
		So(err, ShouldBeNil)
		So(rr.Code, ShouldEqual, 200)
		resp := map[string]interface{}{}
		json.NewDecoder(rr.Body).Decode(&resp)
		So(len(resp), ShouldEqual, 2)
	})

	Convey("metadataHandler call with patterns", t, func() {
		cp, err := pattern.Parse("configSet")
		So(err, ShouldBeNil)
		pp, err := pattern.Parse("path")
		So(err, ShouldBeNil)
		v.ConfigPatterns = append(v.ConfigPatterns, &ConfigPattern{ConfigSet: cp, Path: pp})
		rr, err := metaCall(v)
		So(rr.Code, ShouldEqual, 200)
	})

	Convey("Basic validationHandler call", t, func() {
		rr, err := valCall(v, "dead", "beef", "")
		So(err, ShouldBeNil)
		So(rr.Code, ShouldEqual, 200)
		resp := map[string][]map[string]string{}
		json.NewDecoder(rr.Body).Decode(&resp)
		So(len(resp["messages"]), ShouldEqual, 1)
		So(len(resp["messages"][0]), ShouldEqual, 2)
		So(resp["messages"][0]["text"], ShouldEqual, "deadbeef")
		So(resp["messages"][0]["severity"], ShouldEqual, "ERROR")
	})

	Convey("validationHandler call with no configSet or path", t, func() {
		rr, err := valCall(v, "", "", "")
		So(err, ShouldBeNil)
		So(rr.Code, ShouldEqual, 400)
	})

	Convey("validationHandler call with no path", t, func() {
		rr, err := valCall(v, "dead", "", "")
		So(err, ShouldBeNil)
		So(rr.Code, ShouldEqual, 400)
	})
}

func mockValidationFunction(configSet, path, content string, ctx *Context) {
	ctx.errors = append(ctx.errors, errors.New("deadbeef"))
}
