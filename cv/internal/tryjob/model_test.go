// Copyright 2022 The LUCI Authors.
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
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestTryjob(t *testing.T) {
	t.Parallel()

	ftt.Run("Tryjob", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		t.Run("Populate RetentionKey", func(t *ftt.Test) {
			epoch := datastore.RoundTime(ct.Clock.Now().UTC())
			tj := &Tryjob{
				ID:               1,
				EntityUpdateTime: epoch,
			}
			assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil)
			tj = &Tryjob{ID: 1}
			assert.Loosely(t, datastore.Get(ctx, tj), should.BeNil)
			assert.Loosely(t, tj.RetentionKey, should.Equal(fmt.Sprintf("01/%010d", epoch.Unix())))
		})
		t.Run("Fill in EntityUpdateTime if it's missing", func(t *ftt.Test) {
			tj := &Tryjob{ID: 1}
			assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil)
			tj = &Tryjob{ID: 1}
			assert.Loosely(t, datastore.Get(ctx, tj), should.BeNil)
			assert.Loosely(t, tj.EntityUpdateTime.IsZero(), should.BeFalse)
			assert.Loosely(t, tj.RetentionKey, should.NotBeEmpty)
		})
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	ftt.Run("Delete", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		tj := MustBuildbucketID("bb.example.com", 10).MustCreateIfNotExists(ctx)

		t.Run("Works", func(t *ftt.Test) {
			assert.Loosely(t, CondDelete(ctx, tj.ID, tj.EVersion), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, &Tryjob{ID: tj.ID}), should.ErrLike(datastore.ErrNoSuchEntity))
			assert.Loosely(t, datastore.Get(ctx, &tryjobMap{ExternalID: tj.ExternalID}), should.ErrLike(datastore.ErrNoSuchEntity))
			t.Run("Can handle deleted entity", func(t *ftt.Test) {
				// delete the same entity again
				assert.Loosely(t, CondDelete(ctx, tj.ID, tj.EVersion), should.BeNil)
			})
		})
		t.Run("Works without external ID", func(t *ftt.Test) {
			prevExternalID := tj.ExternalID
			tj.ExternalID = "" // not valid, but just for testing purpose.
			assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil)
			assert.Loosely(t, CondDelete(ctx, tj.ID, tj.EVersion), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, &Tryjob{ID: tj.ID}), should.ErrLike(datastore.ErrNoSuchEntity))
			assert.Loosely(t, datastore.Get(ctx, &tryjobMap{ExternalID: prevExternalID}), should.BeNil)
		})
		t.Run("Returns error for invalid input", func(t *ftt.Test) {
			assert.Loosely(t, CondDelete(ctx, tj.ID, 0), should.ErrLike("expected EVersion must be larger than 0"))
		})
		t.Run("Returns error if condition mismatch", func(t *ftt.Test) {
			tj.EVersion = 15
			assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil)
			assert.Loosely(t, CondDelete(ctx, tj.ID, tj.EVersion-1), should.ErrLike("request to delete tryjob"))
		})
	})
}

func TestCLPatchset(t *testing.T) {
	t.Parallel()

	ftt.Run("CLPatchset works", t, func(t *ftt.Test) {
		var cl common.CLID
		var ps int32
		t.Run("with max values", func(t *ftt.Test) {
			cl, ps = 0x7fff_ffff_ffff_ffff, 0x7fff_ffff
		})
		t.Run("with min values", func(t *ftt.Test) {
			cl, ps = 0, 0
		})
		clps := MakeCLPatchset(cl, ps)
		parsedCl, parsedPs, err := clps.Parse()
		assert.NoErr(t, err)
		assert.Loosely(t, parsedCl, should.Equal(cl))
		assert.Loosely(t, parsedPs, should.Equal(ps))
	})
	ftt.Run("CLPatchset fails", t, func(t *ftt.Test) {
		t.Run("with bad number of values", func(t *ftt.Test) {
			_, _, err := CLPatchset("1/1").Parse()
			assert.Loosely(t, err, should.ErrLike("CLPatchset in unexpected format"))
		})
		t.Run("with bad version", func(t *ftt.Test) {
			_, _, err := CLPatchset("8/8/8").Parse()
			assert.Loosely(t, err, should.ErrLike("unsupported version"))
		})
		t.Run("with bad CLID", func(t *ftt.Test) {
			_, _, err := CLPatchset("1/4d35683b24371b75c5f3fda0d48796638dc0d695/7").Parse()
			assert.Loosely(t, err, should.ErrLike("clid segment in unexpected format"))
			_, _, err = CLPatchset("1/gerrit/chromium-review.googlesource.com/3530834/7").Parse()
			assert.Loosely(t, err, should.ErrLike("unexpected format"))
		})
		t.Run("with bad patchset", func(t *ftt.Test) {
			_, _, err := CLPatchset("1/1/ps1").Parse()
			assert.Loosely(t, err, should.ErrLike("patchset segment in unexpected format"))
		})
	})
}
