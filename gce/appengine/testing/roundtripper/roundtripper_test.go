// Copyright 2018 The LUCI Authors.
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

package roundtripper

import (
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	. "github.com/smartystreets/goconvey/convey"
)

func TestJSONRoundTripper(t *testing.T) {
	t.Parallel()

	Convey("RoundTrip", t, func() {
		rt := &JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		srv := compute.NewInstancesService(gce)
		call := srv.Insert("project", "zone", &compute.Instance{Name: "name"})

		Convey("ok", func() {
			rt.Handler = func(req interface{}) (int, interface{}) {
				inst, ok := req.(*compute.Instance)
				So(ok, ShouldBeTrue)
				So(inst.Name, ShouldEqual, "name")
				return http.StatusOK, &compute.Operation{
					ClientOperationId: "id",
				}
			}
			rt.Type = reflect.TypeOf(compute.Instance{})
			rsp, err := call.Do()
			So(err, ShouldBeNil)
			So(rsp, ShouldNotBeNil)
			So(rsp.ClientOperationId, ShouldEqual, "id")
		})

		Convey("error", func() {
			rt.Handler = func(_ interface{}) (int, interface{}) {
				return http.StatusNotFound, nil
			}
			rt.Type = reflect.TypeOf(compute.Instance{})
			rsp, err := call.Do()
			So(err.(*googleapi.Error).Code, ShouldEqual, http.StatusNotFound)
			So(rsp, ShouldBeNil)
		})
	})
}

func TestStringRoundTripper(t *testing.T) {
	t.Parallel()

	Convey("RoundTrip", t, func() {
		rt := &StringRoundTripper{}
		cli := &http.Client{Transport: rt}
		rt.Handler = func(req *http.Request) (int, string) {
			So(req, ShouldNotBeNil)
			return http.StatusOK, "test"
		}
		rsp, err := cli.Get("https://example.com")
		So(err, ShouldBeNil)
		So(rsp, ShouldNotBeNil)
		So(rsp.StatusCode, ShouldEqual, http.StatusOK)
		b, err := ioutil.ReadAll(rsp.Body)
		So(err, ShouldBeNil)
		So(string(b), ShouldEqual, "test")
	})
}
