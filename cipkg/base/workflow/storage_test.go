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
	"errors"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"
)

func TestStableStorage(t *testing.T) {
	ftt.Run("Test StableStorage", t, func(t *ftt.Test) {
		ctx := context.Background()
		ap := actions.NewActionProcessor()
		pm1 := testutils.NewMockPackageManage("/storage/one")
		pm2 := testutils.NewMockPackageManage("/storage/two")

		makeAction := func(depArg string) *core.Action {
			return &core.Action{
				Name: "pkg",
				Deps: []*core.Action{
					{
						Name: "dep",
						Spec: &core.Action_Command{
							Command: &core.ActionCommand{Args: []string{depArg}},
						},
					},
				},
				Spec: &core.Action_Command{
					Command: &core.ActionCommand{
						Args: []string{"bin", actions.DepRef("dep")},
					},
				},
			}
		}

		makeChain := func(leafArg string) *core.Action {
			leaf := &core.Action{
				Name: "leaf",
				Spec: &core.Action_Command{
					Command: &core.ActionCommand{Args: []string{leafArg}},
				},
			}
			mid := &core.Action{
				Name: "mid",
				Deps: []*core.Action{leaf},
				Spec: &core.Action_Command{
					Command: &core.ActionCommand{Args: []string{"wrap", actions.DepRef("leaf")}},
				},
			}
			return &core.Action{
				Name: "top",
				Deps: []*core.Action{mid},
				Spec: &core.Action_Command{
					Command: &core.ActionCommand{Args: []string{"build", actions.DepRef("mid")}},
				},
			}
		}

		t.Run("reproducible across local storage paths", func(t *ftt.Test) {
			pkg1, err := ap.Process(ctx, "", pm1, makeAction("v1"))
			assert.Loosely(t, err, should.BeNil)
			pkg2, err := ap.Process(ctx, "", pm2, makeAction("v1"))
			assert.Loosely(t, err, should.BeNil)

			// DerivationID differs because local storage paths differ.
			assert.Loosely(t, pkg1.DerivationID, should.NotEqual(pkg2.DerivationID))
			assert.Loosely(t, pkg1.ActionID, should.Equal(pkg2.ActionID))

			var fetchedID1, fetchedID2 string
			var uploadedIDs1, uploadedIDs2 []string

			s1 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
					fetchedID1 = id
					return "hash1", nil
				},
				upload: func(_ context.Context, _ actions.Package, ids []string) (string, error) {
					uploadedIDs1 = ids
					return "hash1", nil
				},
			})
			s2 := NewStableStorage(ap, &testStorage{
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

		t.Run("content-addressed fallback after dependency hash recorded", func(t *ftt.Test) {
			remote := make(map[string]string)

			// Build 1: dep ("v1") uploads "dep-same-hash", then pkg uploads.
			pkgV1, err := ap.Process(ctx, "", pm1, makeAction("v1"))
			assert.Loosely(t, err, should.BeNil)
			depV1 := pkgV1.BuildDependencies[0]

			var uploadedV1 []string
			s1 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, _ string) (string, error) {
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, ids []string) (string, error) {
					h := "hash:" + p.Action.Name
					if p.Action.Name == "dep" {
						h = "dep-same-hash"
					} else {
						uploadedV1 = ids
					}
					for _, id := range ids {
						remote[id] = h
					}
					return h, nil
				},
			})

			_, err = s1.Upload(ctx, depV1, nil)
			assert.Loosely(t, err, should.BeNil)
			_, err = s1.Upload(ctx, pkgV1, nil)
			assert.Loosely(t, err, should.BeNil)
			// Both contentID and relocID are uploaded.
			assert.Loosely(t, uploadedV1, should.HaveLength(2))
			contentIDV1, relocIDV1 := uploadedV1[0], uploadedV1[1]
			assert.Loosely(t, contentIDV1, should.NotEqual(relocIDV1))

			// Build 2: dep action changes ("v2") so relocID changes, but dep produces
			// the same content hash ("dep-same-hash").
			pkgV2, err := ap.Process(ctx, "", pm2, makeAction("v2"))
			assert.Loosely(t, err, should.BeNil)
			depV2 := pkgV2.BuildDependencies[0]

			var fetchedIDs []string
			var uploadedV2 []string
			s2 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
					fetchedIDs = append(fetchedIDs, id)
					if h, ok := remote[id]; ok {
						return h, nil
					}
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, ids []string) (string, error) {
					if p.Action.Name == "dep" {
						return "dep-same-hash", nil
					}
					uploadedV2 = ids
					return "hash:pkg", nil
				},
			})

			// 1st Fetch before dep is built tries relocIDV2 and misses.
			_, err = s2.Fetch(ctx, pkgV2, "")
			assert.Loosely(t, errors.Is(err, core.ErrPackageNotExist), should.BeTrue)
			assert.Loosely(t, fetchedIDs, should.HaveLength(1))
			relocIDV2 := fetchedIDs[0]
			assert.Loosely(t, relocIDV2, should.NotEqual(relocIDV1))

			// Dep builds and uploads, recording "dep-same-hash".
			_, err = s2.Upload(ctx, depV2, nil)
			assert.Loosely(t, err, should.BeNil)

			// 2nd Fetch tries relocIDV2 (miss) and falls back to contentIDV1 (hit).
			hash, err := s2.Fetch(ctx, pkgV2, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hash, should.Equal("hash:pkg"))
			assert.Loosely(t, fetchedIDs, should.Match([]string{relocIDV2, relocIDV2, contentIDV1}))

			// Upload passes [contentIDV1, relocIDV2] so remote storage can tag the
			// existing contentIDV1 artifact with relocIDV2.
			_, err = s2.Upload(ctx, pkgV2, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, uploadedV2, should.Match([]string{contentIDV1, relocIDV2}))
		})

		t.Run("transitive propagation through unstored intermediate dependency", func(t *ftt.Test) {
			remote := make(map[string]string)

			topV1, err := ap.Process(ctx, "", pm1, makeChain("v1"))
			assert.Loosely(t, err, should.BeNil)
			midV1 := topV1.BuildDependencies[0]
			leafV1 := midV1.BuildDependencies[0]

			s1 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, _ string) (string, error) {
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, ids []string) (string, error) {
					switch p.Action.Name {
					case "leaf":
						return "leaf-fixed-hash", nil
					case "mid":
						// Unstored intermediate package returns empty hash.
						return "", nil
					default:
						for _, id := range ids {
							remote[id] = "top-hash"
						}
						return "top-hash", nil
					}
				},
			})

			_, err = s1.Upload(ctx, leafV1, nil)
			assert.Loosely(t, err, should.BeNil)
			_, err = s1.Upload(ctx, midV1, nil)
			assert.Loosely(t, err, should.BeNil)
			_, err = s1.Upload(ctx, topV1, nil)
			assert.Loosely(t, err, should.BeNil)

			topV2, err := ap.Process(ctx, "", pm2, makeChain("v2"))
			assert.Loosely(t, err, should.BeNil)
			midV2 := topV2.BuildDependencies[0]
			leafV2 := midV2.BuildDependencies[0]

			s2 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
					if h, ok := remote[id]; ok {
						return h, nil
					}
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, _ []string) (string, error) {
					if p.Action.Name == "leaf" {
						return "leaf-fixed-hash", nil
					}
					return "", nil
				},
			})

			// 1st Fetch misses relocID.
			_, err = s2.Fetch(ctx, topV2, "")
			assert.Loosely(t, errors.Is(err, core.ErrPackageNotExist), should.BeTrue)

			// leaf uploads same hash; mid uploads empty hash.
			_, err = s2.Upload(ctx, leafV2, nil)
			assert.Loosely(t, err, should.BeNil)
			_, err = s2.Upload(ctx, midV2, nil)
			assert.Loosely(t, err, should.BeNil)

			// 2nd Fetch on topV2 propagates leaf's hash through mid and hits cache.
			hash, err := s2.Fetch(ctx, topV2, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hash, should.Equal("top-hash"))
		})

		t.Run("lazily resolves dependency hash via Resolve when not in memory", func(t *ftt.Test) {
			pkgV1, err := ap.Process(ctx, "", pm1, makeAction("v1"))
			assert.Loosely(t, err, should.BeNil)

			remote := make(map[string]string)
			s1 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, _ string) (string, error) {
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, ids []string) (string, error) {
					h := "pkg-hash"
					if p.Action.Name == "dep" {
						h = "dep-hash"
					}
					for _, id := range ids {
						remote[id] = h
					}
					return h, nil
				},
			})
			_, err = s1.Upload(ctx, pkgV1.BuildDependencies[0], nil)
			assert.Loosely(t, err, should.BeNil)
			_, err = s1.Upload(ctx, pkgV1, nil)
			assert.Loosely(t, err, should.BeNil)

			// dep ("v2") was already uploaded remotely with the same content hash,
			// while pkg ("v2") has not been tagged with relocID(pkgV2) yet.
			pkgV2, err := ap.Process(ctx, "", pm2, makeAction("v2"))
			assert.Loosely(t, err, should.BeNil)
			_, err = s1.Upload(ctx, pkgV2.BuildDependencies[0], nil)
			assert.Loosely(t, err, should.BeNil)

			// Fresh ReadOnlyStorage + StableStorage (empty in-memory hashes, e.g.
			// warm local cache or cold Expand before downloading dep): Fetch(pkgV2)
			// lazily calls Resolve(depV2) and hits contentID(pkg) without calling
			// Fetch(depV2) or Upload.
			var fetchedPkgs, resolvedPkgs []string
			s2 := NewReadOnlyStorage(NewStableStorage(ap, &testStorage{
				resolve: func(_ context.Context, p actions.Package, id string) (string, error) {
					resolvedPkgs = append(resolvedPkgs, p.Action.Name)
					if h, ok := remote[id]; ok {
						return h, nil
					}
					return "", core.ErrPackageNotExist
				},
				fetch: func(_ context.Context, p actions.Package, id string) (string, error) {
					fetchedPkgs = append(fetchedPkgs, p.Action.Name)
					if h, ok := remote[id]; ok {
						return h, nil
					}
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, _ actions.Package, _ []string) (string, error) {
					return "", errors.New("unexpected upload")
				},
			}))

			hash, err := s2.Fetch(ctx, pkgV2, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hash, should.Equal("pkg-hash"))
			assert.Loosely(t, resolvedPkgs, should.Match([]string{"dep"}))
			assert.Loosely(t, fetchedPkgs, should.Match([]string{"pkg", "pkg"}))
		})

		t.Run("stops recursion at resolved dependency without resolving its children", func(t *ftt.Test) {
			topPkg, err := ap.Process(ctx, "", pm1, makeChain("v1"))
			assert.Loosely(t, err, should.BeNil)

			var resolvedPkgs []string
			s := NewStableStorage(ap, &testStorage{
				resolve: func(_ context.Context, p actions.Package, _ string) (string, error) {
					resolvedPkgs = append(resolvedPkgs, p.Action.Name)
					if p.Action.Name == "mid" {
						return "mid-hash", nil
					}
					return "", core.ErrPackageNotExist
				},
				fetch: func(_ context.Context, _ actions.Package, _ string) (string, error) {
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, _ actions.Package, _ []string) (string, error) {
					return "", nil
				},
			})

			_, err = s.Fetch(ctx, topPkg, "")
			assert.Loosely(t, errors.Is(err, core.ErrPackageNotExist), should.BeTrue)
			assert.Loosely(t, resolvedPkgs, should.Match([]string{"mid"}))
		})

		t.Run("ActionRef dependency preserves stable content ID across target renames", func(t *ftt.Test) {
			makeWithRef := func(targetName, arg string) *core.Action {
				target := &core.Action{
					Name: targetName,
					Spec: &core.Action_Command{
						Command: &core.ActionCommand{Args: []string{arg}},
					},
				}
				ref := &core.Action{
					Name: "dep",
					Deps: []*core.Action{target},
					Spec: &core.Action_Ref{Ref: &core.ActionRef{}},
				}
				return &core.Action{
					Name: "pkg",
					Deps: []*core.Action{ref},
					Spec: &core.Action_Command{
						Command: &core.ActionCommand{Args: []string{"bin", actions.DepRef("dep")}},
					},
				}
			}

			remote := make(map[string]string)
			pkgV1, err := ap.Process(ctx, "", pm1, makeWithRef("dep_git_1111", "v1"))
			assert.NoErr(t, err)
			refV1 := pkgV1.BuildDependencies[0]

			s1 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, _ string) (string, error) {
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, ids []string) (string, error) {
					if p.Action.Name == "dep" {
						return "same-dep-hash", nil
					}
					for _, id := range ids {
						remote[id] = "pkg-hash"
					}
					return "pkg-hash", nil
				},
			})
			_, err = s1.Upload(ctx, refV1, nil)
			assert.NoErr(t, err)
			_, err = s1.Upload(ctx, pkgV1, nil)
			assert.NoErr(t, err)

			// Build 2 changes target's natural name and args, but ref still has
			// Action.Name == "dep" and uploads the same content hash.
			pkgV2, err := ap.Process(ctx, "", pm2, makeWithRef("dep_git_2222", "v2"))
			assert.NoErr(t, err)
			refV2 := pkgV2.BuildDependencies[0]

			s2 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
					if h, ok := remote[id]; ok {
						return h, nil
					}
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, _ []string) (string, error) {
					if p.Action.Name == "dep" {
						return "same-dep-hash", nil
					}
					return "pkg-hash", nil
				},
			})
			_, err = s2.Upload(ctx, refV2, nil)
			assert.NoErr(t, err)

			hash, err := s2.Fetch(ctx, pkgV2, "")
			assert.NoErr(t, err)
			assert.Loosely(t, hash, should.Equal("pkg-hash"))
		})

		t.Run("ActionRef itself reuses target content hash on fallback", func(t *ftt.Test) {
			makeRef := func(arg string) *core.Action {
				target := &core.Action{
					Name: "target",
					Spec: &core.Action_Command{
						Command: &core.ActionCommand{Args: []string{arg}},
					},
				}
				return &core.Action{
					Name: "alias",
					Deps: []*core.Action{target},
					Spec: &core.Action_Ref{Ref: &core.ActionRef{}},
				}
			}

			remote := make(map[string]string)
			refV1, err := ap.Process(ctx, "", pm1, makeRef("v1"))
			assert.NoErr(t, err)
			targetV1 := refV1.BuildDependencies[0]

			s1 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, _ string) (string, error) {
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, ids []string) (string, error) {
					if p.Action.Name == "target" {
						return "same-target-hash", nil
					}
					for _, id := range ids {
						remote[id] = "alias-hash"
					}
					return "alias-hash", nil
				},
			})
			_, err = s1.Upload(ctx, targetV1, nil)
			assert.NoErr(t, err)
			_, err = s1.Upload(ctx, refV1, nil)
			assert.NoErr(t, err)

			refV2, err := ap.Process(ctx, "", pm2, makeRef("v2"))
			assert.NoErr(t, err)
			targetV2 := refV2.BuildDependencies[0]

			s2 := NewStableStorage(ap, &testStorage{
				fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
					if h, ok := remote[id]; ok {
						return h, nil
					}
					return "", core.ErrPackageNotExist
				},
				upload: func(_ context.Context, p actions.Package, _ []string) (string, error) {
					if p.Action.Name == "target" {
						return "same-target-hash", nil
					}
					return "alias-hash", nil
				},
			})
			_, err = s2.Upload(ctx, targetV2, nil)
			assert.NoErr(t, err)

			hash, err := s2.Fetch(ctx, refV2, "")
			assert.NoErr(t, err)
			assert.Loosely(t, hash, should.Equal("alias-hash"))
		})
	})
}

func TestReadOnlyStorage(t *testing.T) {
	ftt.Run("Test ReadOnlyStorage", t, func(t *ftt.Test) {
		ctx := context.Background()
		uploadCalled := false
		s := NewReadOnlyStorage(&testStorage{
			resolve: func(_ context.Context, _ actions.Package, id string) (string, error) {
				return "resolved:" + id, nil
			},
			fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
				return "fetched:" + id, nil
			},
			upload: func(_ context.Context, _ actions.Package, _ []string) (string, error) {
				uploadCalled = true
				return "uploaded", nil
			},
		})

		hash, err := s.Resolve(ctx, actions.Package{}, "id1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hash, should.Equal("resolved:id1"))

		hash, err = s.Fetch(ctx, actions.Package{}, "id1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hash, should.Equal("fetched:id1"))

		hash, err = s.Upload(ctx, actions.Package{}, []string{"id1"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hash, should.BeEmpty)
		assert.Loosely(t, uploadCalled, should.BeFalse)
	})
}
