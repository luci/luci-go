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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/server/router"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
)

func TestInstallHandlers(t *testing.T) {
	Convey("Initialization of validator, validation routes and handlers", t, func() {
		v := &Validator{Func: func(configSet, path string, content []byte, ctx *Context) {
			ctx.errors = append(ctx.errors, errors.New("deadbeef"))
		}}
		r := router.New()
		rr := httptest.NewRecorder()

		metaCall := func() *config.ServiceDynamicMetadata {
			req, err := http.NewRequest("GET", metadataPath, nil)
			So(err, ShouldBeNil)
			r.ServeHTTP(rr, req)

			var resp config.ServiceDynamicMetadata
			err = json.NewDecoder(rr.Body).Decode(&resp)
			So(err, ShouldBeNil)
			return &resp
		}
		valCall := func(configSet, path, content string, getResp bool) *config.ValidationResponseMessage {
			respBodyJSON, err := json.Marshal(map[string]interface{}{"config_set": configSet, "path": path, "content": []byte(content)})
			So(err, ShouldBeNil)
			req, err := http.NewRequest("POST", validationPath, strings.NewReader(string(respBodyJSON)))
			So(err, ShouldBeNil)
			r.ServeHTTP(rr, req)
			if getResp {
				var resp config.ValidationResponseMessage
				err = json.NewDecoder(rr.Body).Decode(&resp)
				So(err, ShouldBeNil)
				return &resp
			}
			return nil
		}

		InstallHandlers(r, router.NewMiddlewareChain(), v)

		Convey("Basic metadataHandler call", func() {
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(metaCall(), ShouldResemble, &config.ServiceDynamicMetadata{
				Version: proto.String(metaDataFormatVersion),
				Validation: &config.Validator{
					Url: proto.String(fmt.Sprintf("https://%s", metadataPath)),
				},
			})
		})

		Convey("metadataHandler call with patterns", func() {
			cp, err := pattern.Parse("configSet")
			So(err, ShouldBeNil)
			pp, err := pattern.Parse("path")
			So(err, ShouldBeNil)
			v.ConfigPatterns = append(v.ConfigPatterns, &ConfigPattern{ConfigSet: cp, Path: pp})
			meta := metaCall()
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(meta, ShouldResemble, &config.ServiceDynamicMetadata{
				Version: proto.String(metaDataFormatVersion),
				Validation: &config.Validator{
					Url: proto.String(fmt.Sprintf("https://%s", metadataPath)),
					Patterns: []*config.ConfigPattern{{
						ConfigSet: proto.String(cp.String()),
						Path:      proto.String(pp.String())},
					},
				},
			})
		})

		Convey("Basic validationHandler call", func() {
			v.Func = func(configSet, path string, content []byte, ctx *Context) {
				So(string(content), ShouldEqual, "content")
				ctx.errors = append(ctx.errors, errors.New("deadbeef"))
			}
			valResp := valCall("dead", "beef", "content", true)
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(valResp, ShouldResemble, &config.ValidationResponseMessage{
				Messages: []*config.ValidationResponseMessage_Message{{
					Text:     proto.String("deadbeef"),
					Severity: config.ValidationResponseMessage_ERROR.Enum(),
				}},
			})
		})

		Convey("validationHandler call with no configSet or path", func() {
			valCall("", "", "", false)
			So(rr.Code, ShouldEqual, http.StatusBadRequest)
			So(rr.Body.String(), ShouldEqual, "Must specify the config_set of the file to validate")
		})

		Convey("validationHandler call with no path", func() {
			valCall("dead", "", "", false)
			So(rr.Code, ShouldEqual, http.StatusBadRequest)
			So(rr.Body.String(), ShouldEqual, "Must specify the path of the file to validate")
		})
	})
}
