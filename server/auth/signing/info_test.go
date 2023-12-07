// Copyright 2016 The LUCI Authors.
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

package signing

import (
	"context"
	"net/http"
	"testing"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchServiceInfo(t *testing.T) {
	t.Parallel()

	const testURL = "https://test.example.com"

	Convey("With empty cache", t, func() {
		ctx := caching.WithEmptyProcessCache(context.Background())
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			infoCacheNamespace: cachingtest.NewBlobCache(),
		})

		Convey("Works", func() {
			calls := 0
			ctx := internal.WithTestTransport(ctx, func(r *http.Request, body string) (int, string) {
				So(r.URL.String(), ShouldEqual, testURL)
				calls++
				return 200, `{
					"app_id": "some-app-id",
					"app_runtime": "go",
					"app_runtime_version": "go1.5.1",
					"app_version": "1234-abcdef",
					"service_account_name": "some-app-id@appspot.gserviceaccount.com"
				}`
			})

			expected := &ServiceInfo{
				AppID:              "some-app-id",
				AppRuntime:         "go",
				AppRuntimeVersion:  "go1.5.1",
				AppVersion:         "1234-abcdef",
				ServiceAccountName: "some-app-id@appspot.gserviceaccount.com",
			}

			info1, err := FetchServiceInfo(ctx, testURL)
			So(err, ShouldBeNil)
			So(info1, ShouldResemble, expected)
			So(calls, ShouldEqual, 1)

			// The in-process cache works.
			info2, err := FetchServiceInfo(ctx, testURL)
			So(err, ShouldBeNil)
			So(info2, ShouldEqual, info1) // the exact same object
			So(calls, ShouldEqual, 1)     // no new calls

			// The global cache works too.
			ctx = caching.WithEmptyProcessCache(ctx)
			info3, err := FetchServiceInfo(ctx, testURL)
			So(err, ShouldBeNil)
			So(info3, ShouldNotPointTo, info1)  // a new deserialized object
			So(info3, ShouldResemble, expected) // still has the correct value
			So(calls, ShouldEqual, 1)           // no new calls
		})

		Convey("Error", func() {
			ctx := internal.WithTestTransport(ctx, func(r *http.Request, body string) (int, string) {
				return 500, "error"
			})
			info, err := FetchServiceInfo(ctx, testURL)
			So(info, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestFetchLUCIServiceIdentity(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := caching.WithEmptyProcessCache(context.Background())
		ctx = internal.WithTestTransport(ctx, func(r *http.Request, body string) (code int, response string) {
			if r.URL.String() != "https://blah/auth/api/v1/server/info" {
				return 400, "Wrong URL"
			}
			return 200, `{"app_id": "blah-app-id"}`
		})

		id, err := FetchLUCIServiceIdentity(ctx, "https://blah")
		So(err, ShouldBeNil)
		So(id, ShouldEqual, identity.Identity("service:blah-app-id"))

		_, err = FetchLUCIServiceIdentity(ctx, "http://blah")
		So(err, ShouldErrLike, "not an https:// URL")

		_, err = FetchLUCIServiceIdentity(ctx, "https://blah/blah")
		So(err, ShouldErrLike, "not a root URL")

		_, err = FetchLUCIServiceIdentity(ctx, "https://")
		So(err, ShouldErrLike, "not a root URL")
	})
}
