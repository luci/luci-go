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
	"net/http"
	"reflect"
	"testing"

	"google.golang.org/api/compute/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestJSONRoundTripper(t *testing.T) {
	t.Parallel()

	Convey("RoundTrip", t, func() {
		rt := &JSONRoundTripper{
			Handler: func(req interface{}) interface{} {
				inst, ok := req.(*compute.Instance)
				So(ok, ShouldBeTrue)
				So(inst.Name, ShouldEqual, "name")
				return &compute.Operation{
					ClientOperationId: "id",
				}
			},
			Type: reflect.TypeOf(compute.Instance{}),
		}
		gce, err := compute.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		srv := compute.NewInstancesService(gce)
		call := srv.Insert("project", "zone", &compute.Instance{Name: "name"})
		rsp, err := call.Do()
		So(err, ShouldBeNil)
		So(rsp, ShouldNotBeNil)
		So(rsp.ClientOperationId, ShouldEqual, "id")
	})
}
