// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workflow

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"
)

func TestRelocatableStorage(t *testing.T) {
	ftt.Run("Test RelocatableStorage", t, func(t *ftt.Test) {
		ctx := context.Background()
		ap := actions.NewActionProcessor()
		pm1 := testutils.NewMockPackageManage("/storage/one")
		pm2 := testutils.NewMockPackageManage("/storage/two")

		makeAction := func() *core.Action {
			return &core.Action{
				Name: "pkg",
				Deps: []*core.Action{
					{Name: "dep", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Command{
					Command: &core.ActionCommand{
						Args: []string{"bin", actions.DepRef("dep")},
					},
				},
			}
		}

		pkg1, err := ap.Process("", pm1, makeAction())
		assert.Loosely(t, err, should.BeNil)
		pkg2, err := ap.Process("", pm2, makeAction())
		assert.Loosely(t, err, should.BeNil)

		// DerivationID differs because local storage paths differ.
		assert.Loosely(t, pkg1.DerivationID, should.NotEqual(pkg2.DerivationID))
		assert.Loosely(t, pkg1.ActionID, should.Equal(pkg2.ActionID))

		var fetchedID1, fetchedID2 string
		var uploadedIDs1, uploadedIDs2 []string

		s1 := NewRelocatableStorage(ap, &testStorage{
			fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
				fetchedID1 = id
				return "hash1", nil
			},
			upload: func(_ context.Context, _ actions.Package, ids []string) (string, error) {
				uploadedIDs1 = ids
				return "hash1", nil
			},
		})
		s2 := NewRelocatableStorage(ap, &testStorage{
			fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
				fetchedID2 = id
				return "hash2", nil
			},
			upload: func(_ context.Context, _ actions.Package, ids []string) (string, error) {
				uploadedIDs2 = ids
				return "hash2", nil
			},
		})

		_, err = s1.Fetch(ctx, pkg1, "")
		assert.Loosely(t, err, should.BeNil)
		_, err = s2.Fetch(ctx, pkg2, "")
		assert.Loosely(t, err, should.BeNil)

		// Relocatable ID is reproducible across different storage paths.
		assert.Loosely(t, fetchedID1, should.NotBeEmpty)
		assert.Loosely(t, fetchedID1, should.Equal(fetchedID2))
		assert.Loosely(t, fetchedID1, should.NotEqual(pkg1.DerivationID))
		assert.Loosely(t, fetchedID2, should.NotEqual(pkg2.DerivationID))

		_, err = s1.Upload(ctx, pkg1, []string{"existing_id"})
		assert.Loosely(t, err, should.BeNil)
		_, err = s2.Upload(ctx, pkg2, []string{"existing_id"})
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, uploadedIDs1, should.Match([]string{"existing_id", fetchedID1}))
		assert.Loosely(t, uploadedIDs2, should.Match(uploadedIDs1))
	})
}

func TestReadOnlyStorage(t *testing.T) {
	ftt.Run("Test ReadOnlyStorage", t, func(t *ftt.Test) {
		ctx := context.Background()
		uploadCalled := false
		s := NewReadOnlyStorage(&testStorage{
			fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
				return "fetched:" + id, nil
			},
			upload: func(_ context.Context, _ actions.Package, _ []string) (string, error) {
				uploadCalled = true
				return "uploaded", nil
			},
		})

		hash, err := s.Fetch(ctx, actions.Package{}, "id1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hash, should.Equal("fetched:id1"))

		hash, err = s.Upload(ctx, actions.Package{}, []string{"id1"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hash, should.BeEmpty)
		assert.Loosely(t, uploadCalled, should.BeFalse)
	})
}
