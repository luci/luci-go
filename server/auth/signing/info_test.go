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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
)

func TestFetchServiceInfo(t *testing.T) {
	t.Parallel()

	const testURL = "https://test.example.com"

	ftt.Run("With empty cache", t, func(t *ftt.Test) {
		ctx := caching.WithEmptyProcessCache(context.Background())
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			infoCacheNamespace: cachingtest.NewBlobCache(),
		})

		t.Run("Works", func(t *ftt.Test) {
			calls := 0
			ctx := internal.WithTestTransport(ctx, func(r *http.Request, body string) (int, string) {
				assert.Loosely(t, r.URL.String(), should.Equal(testURL))
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info1, should.Match(expected))
			assert.Loosely(t, calls, should.Equal(1))

			// The in-process cache works.
			info2, err := FetchServiceInfo(ctx, testURL)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info2, should.Equal(info1)) // the exact same object
			assert.Loosely(t, calls, should.Equal(1))     // no new calls

			// The global cache works too.
			ctx = caching.WithEmptyProcessCache(ctx)
			info3, err := FetchServiceInfo(ctx, testURL)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info3, should.NotEqual(info1)) // a new deserialized object
			assert.Loosely(t, info3, should.Match(expected)) // still has the correct value
			assert.Loosely(t, calls, should.Equal(1))        // no new calls
		})

		t.Run("Error", func(t *ftt.Test) {
			ctx := internal.WithTestTransport(ctx, func(r *http.Request, body string) (int, string) {
				return 500, "error"
			})
			info, err := FetchServiceInfo(ctx, testURL)
			assert.Loosely(t, info, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestFetchLUCIServiceIdentity(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := caching.WithEmptyProcessCache(context.Background())
		ctx = internal.WithTestTransport(ctx, func(r *http.Request, body string) (code int, response string) {
			if r.URL.String() != "https://blah/auth/api/v1/server/info" {
				return 400, "Wrong URL"
			}
			return 200, `{"app_id": "blah-app-id"}`
		})

		id, err := FetchLUCIServiceIdentity(ctx, "https://blah")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, id, should.Equal(identity.Identity("service:blah-app-id")))

		_, err = FetchLUCIServiceIdentity(ctx, "http://blah")
		assert.Loosely(t, err, should.ErrLike("not an https:// URL"))

		_, err = FetchLUCIServiceIdentity(ctx, "https://blah/blah")
		assert.Loosely(t, err, should.ErrLike("not a root URL"))

		_, err = FetchLUCIServiceIdentity(ctx, "https://")
		assert.Loosely(t, err, should.ErrLike("not a root URL"))
	})
}
