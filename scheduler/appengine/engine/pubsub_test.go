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

package engine

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigureTopic(t *testing.T) {
	Convey("configureTopic works", t, func(ctx C) {
		c := newTestContext(epoch)

		calls := []struct {
			Call     string
			Code     int
			Request  string
			Response string
		}{
			// First round.
			{
				Call:     "PUT /v1/projects/abc/topics/def",
				Code:     200,
				Response: `{}`,
			},
			{
				Call:     "PUT /v1/projects/abc/subscription/def",
				Code:     200,
				Response: `{}`,
			},
			{
				Call:     "GET /v1/projects/abc/topics/def:getIamPolicy",
				Code:     200,
				Response: `{"etag": "some_etag"}`,
			},
			{
				Call: "POST /v1/projects/abc/topics/def:setIamPolicy",
				Code: 200,
				Request: `{
					"policy": {
						"bindings": [
							{
								"role": "roles/pubsub.publisher",
								"members": ["user:some@publisher.com"]
							}
						],
						"etag": "some_etag"
					}
				}`,
				Response: `{}`,
			},

			// Repeat to test idempotency.
			{
				Call:     "PUT /v1/projects/abc/topics/def",
				Code:     409,
				Response: `{}`,
			},
			{
				Call:     "PUT /v1/projects/abc/subscription/def",
				Code:     409,
				Response: `{}`,
			},
			{
				Call: "GET /v1/projects/abc/topics/def:getIamPolicy",
				Code: 200,
				Response: `{
					"bindings": [
						{
							"role": "roles/pubsub.publisher",
							"members": ["user:some@publisher.com"]
						}
					],
					"etag": "some_etag"
				}`,
			},
		}
		idx := 0

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if idx == len(calls) {
				ctx.Printf("Unexpected URL call '%s %s'", r.Method, r.URL.Path)
				ctx.So(idx, ShouldNotEqual, len(calls))
			}
			call := calls[idx]
			idx++
			ctx.So(r.Method+" "+r.URL.Path, ShouldEqual, call.Call)
			if call.Request != "" {
				blob, err := ioutil.ReadAll(r.Body)
				ctx.So(err, ShouldBeNil)
				expected := make(map[string]interface{})
				received := make(map[string]interface{})
				ctx.So(json.Unmarshal([]byte(call.Request), &expected), ShouldBeNil)
				ctx.So(json.Unmarshal([]byte(blob), &received), ShouldBeNil)
				ctx.So(received, ShouldResemble, expected)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(call.Code)
			w.Write([]byte(call.Response))
		}))
		defer ts.Close()

		err := configureTopic(
			c,
			"projects/abc/topics/def",
			"projects/abc/subscription/def",
			"http://push_url",
			"some@publisher.com",
			ts.URL)
		So(err, ShouldBeNil)

		// Repeat to test idempotency.
		err = configureTopic(
			c,
			"projects/abc/topics/def",
			"projects/abc/subscription/def",
			"http://push_url",
			"some@publisher.com",
			ts.URL)
		So(err, ShouldBeNil)

		// All expected calls are made.
		So(idx, ShouldEqual, len(calls))
	})
}
