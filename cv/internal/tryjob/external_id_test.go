// Copyright 2021 The LUCI Authors.
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

package tryjob

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestExternalID(t *testing.T) {
	t.Parallel()

	ftt.Run("ExternalID works", t, func(t *ftt.Test) {

		t.Run("BuildbucketID", func(t *ftt.Test) {
			eid, err := BuildbucketID("cr-buildbucket.appspot.com", 12)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, eid, should.Resemble(ExternalID("buildbucket/cr-buildbucket.appspot.com/12")))

			host, build, err := eid.ParseBuildbucketID()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, host, should.Match("cr-buildbucket.appspot.com"))
			assert.Loosely(t, build, should.Equal(12))

			assert.Loosely(t, eid.MustURL(), should.Match("https://cr-buildbucket.appspot.com/build/12"))
		})
		t.Run("Bad ID", func(t *ftt.Test) {
			e := ExternalID("blah")
			_, _, err := e.ParseBuildbucketID()
			assert.Loosely(t, err, should.ErrLike("not a valid BuildbucketID"))

			_, err = e.URL()
			assert.Loosely(t, err, should.ErrLike("invalid ExternalID"))
		})
	})

	ftt.Run("Resolve works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		host := "example.com"

		t.Run("None exist", func(t *ftt.Test) {
			ids := []ExternalID{
				MustBuildbucketID(host, 101),
				MustBuildbucketID(host, 102),
				MustBuildbucketID(host, 103),
			}
			// None of ids[:] are created.

			tjIDs, err := Resolve(ctx, ids...)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tjIDs, should.HaveLength(len(ids)))
			for _, tjID := range tjIDs {
				assert.Loosely(t, tjID, should.BeZero)
			}
			tjs, err := ResolveToTryjobs(ctx, ids...)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tjs, should.HaveLength(len(ids)))
			for _, tj := range tjs {
				assert.Loosely(t, tj, should.BeNil)
			}
		})
		t.Run("Some exist", func(t *ftt.Test) {
			ids := []ExternalID{
				MustBuildbucketID(host, 201),
				MustBuildbucketID(host, 202),
				MustBuildbucketID(host, 203),
			}
			// ids[0] is not created.
			ids[1].MustCreateIfNotExists(ctx)
			ids[2].MustCreateIfNotExists(ctx)

			tjIDs, err := Resolve(ctx, ids...)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tjIDs, should.HaveLength(len(ids)))
			for i, tjID := range tjIDs {
				if i == 0 {
					assert.Loosely(t, tjID, should.BeZero)
				} else {
					assert.Loosely(t, tjID, should.NotEqual(0))
				}
			}

			tjs, err := ResolveToTryjobs(ctx, ids...)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tjs, should.HaveLength(len(ids)))
			for i, tj := range tjs {
				if i == 0 {
					assert.Loosely(t, tj, should.BeNil)
				} else {
					assert.Loosely(t, tj, should.NotBeNil)
					assert.Loosely(t, tj.ExternalID, should.Equal(ids[i]))
				}
			}
		})

		t.Run("All exist", func(t *ftt.Test) {
			ids := []ExternalID{
				MustBuildbucketID(host, 301),
				MustBuildbucketID(host, 302),
				MustBuildbucketID(host, 303),
			}
			ids[0].MustCreateIfNotExists(ctx)
			ids[1].MustCreateIfNotExists(ctx)
			ids[2].MustCreateIfNotExists(ctx)

			tjIDs, err := Resolve(ctx, ids...)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tjIDs, should.HaveLength(len(ids)))
			for _, tjID := range tjIDs {
				assert.Loosely(t, tjID, should.NotEqual(0))
			}

			tjs, err := ResolveToTryjobs(ctx, ids...)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tjs, should.HaveLength(len(ids)))
			for i, tj := range tjs {
				assert.Loosely(t, tj, should.NotBeNil)
				assert.Loosely(t, tj.ExternalID, should.Equal(ids[i]))
			}
		})
	})
}
