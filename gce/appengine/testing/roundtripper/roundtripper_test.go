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
	"io"
	"net/http"
	"reflect"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

func TestJSONRoundTripper(t *testing.T) {
	t.Parallel()

	ftt.Run("RoundTrip", t, func(t *ftt.Test) {
		rt := &JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		assert.Loosely(t, err, should.BeNil)
		srv := compute.NewInstancesService(gce)
		call := srv.Insert("project", "zone", &compute.Instance{Name: "name"})

		t.Run("ok", func(t *ftt.Test) {
			rt.Handler = func(req any) (int, any) {
				inst, ok := req.(*compute.Instance)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, inst.Name, should.Equal("name"))
				return http.StatusOK, &compute.Operation{
					ClientOperationId: "id",
				}
			}
			rt.Type = reflect.TypeOf(compute.Instance{})
			rsp, err := call.Do()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp, should.NotBeNil)
			assert.Loosely(t, rsp.ClientOperationId, should.Equal("id"))
		})

		t.Run("error", func(t *ftt.Test) {
			rt.Handler = func(_ any) (int, any) {
				return http.StatusNotFound, nil
			}
			rt.Type = reflect.TypeOf(compute.Instance{})
			rsp, err := call.Do()
			assert.Loosely(t, err.(*googleapi.Error).Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp, should.BeNil)
		})
	})
}

func TestStringRoundTripper(t *testing.T) {
	t.Parallel()

	ftt.Run("RoundTrip", t, func(t *ftt.Test) {
		rt := &StringRoundTripper{}
		cli := &http.Client{Transport: rt}
		rt.Handler = func(req *http.Request) (int, string) {
			assert.Loosely(t, req, should.NotBeNil)
			return http.StatusOK, "test"
		}
		rsp, err := cli.Get("https://example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rsp, should.NotBeNil)
		assert.Loosely(t, rsp.StatusCode, should.Equal(http.StatusOK))
		b, err := io.ReadAll(rsp.Body)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(b), should.Equal("test"))
	})
}
