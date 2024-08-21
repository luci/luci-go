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

package model

import (
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		pkg := &Package{Name: "pkg"}
		inst1 := &Instance{
			InstanceID:   strings.Repeat("1", 40),
			Package:      PackageKey(ctx, "pkg"),
			RegisteredBy: "user:1@example.com",
		}
		inst2 := &Instance{
			InstanceID:   strings.Repeat("2", 40),
			Package:      PackageKey(ctx, "pkg"),
			RegisteredBy: "user:2@example.com",
		}
		missing := &Instance{
			InstanceID:   strings.Repeat("3", 40),
			Package:      PackageKey(ctx, "pkg"),
			RegisteredBy: "user:3@example.com",
		}

		assert.Loosely(t, datastore.Put(ctx,
			pkg, inst1, inst2, // note: do not store 'missing' here
			&Ref{
				Name:       "latest",
				Package:    PackageKey(ctx, "pkg"),
				InstanceID: inst2.InstanceID,
			},
			&Ref{
				Name:       "broken",
				Package:    PackageKey(ctx, "pkg"),
				InstanceID: missing.InstanceID,
			},
			&Tag{
				ID:       TagID(&api.Tag{Key: "ver", Value: "1"}),
				Instance: datastore.KeyForObj(ctx, inst1),
				Tag:      "ver:1",
			},
			&Tag{
				ID:       TagID(&api.Tag{Key: "ver", Value: "ambiguous"}),
				Instance: datastore.KeyForObj(ctx, inst1),
				Tag:      "ver:ambiguous",
			},
			&Tag{
				ID:       TagID(&api.Tag{Key: "ver", Value: "ambiguous"}),
				Instance: datastore.KeyForObj(ctx, inst2),
				Tag:      "ver:ambiguous",
			},
			&Tag{
				ID:       TagID(&api.Tag{Key: "ver", Value: "broken"}),
				Instance: datastore.KeyForObj(ctx, missing),
				Tag:      "ver:broken",
			},
		), should.BeNil)

		t.Run("Resolves instance ID", func(t *ftt.Test) {
			inst, err := ResolveVersion(ctx, "pkg", inst1.InstanceID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inst, should.Resemble(inst1))
		})

		t.Run("Resolves ref", func(t *ftt.Test) {
			inst, err := ResolveVersion(ctx, "pkg", "latest")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inst, should.Resemble(inst2))
		})

		t.Run("Resolves tag", func(t *ftt.Test) {
			inst, err := ResolveVersion(ctx, "pkg", "ver:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inst, should.Resemble(inst1))
		})

		t.Run("Fails on unrecognized version format", func(t *ftt.Test) {
			_, err := ResolveVersion(ctx, "pkg", "::::")
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("not a valid version identifier"))
		})

		t.Run("Handles missing instance", func(t *ftt.Test) {
			t.Run("Via instance ID", func(t *ftt.Test) {
				_, err := ResolveVersion(ctx, "pkg", missing.InstanceID)
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
			t.Run("Via broken ref", func(t *ftt.Test) {
				_, err := ResolveVersion(ctx, "pkg", "broken")
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
			t.Run("Via broken tag", func(t *ftt.Test) {
				_, err := ResolveVersion(ctx, "pkg", "ver:broken")
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
		})

		t.Run("Handles missing package", func(t *ftt.Test) {
			t.Run("Via instance ID", func(t *ftt.Test) {
				_, err := ResolveVersion(ctx, "pkg2", inst1.InstanceID)
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
			t.Run("Via ref", func(t *ftt.Test) {
				_, err := ResolveVersion(ctx, "pkg2", "latest")
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
			t.Run("Via tag", func(t *ftt.Test) {
				_, err := ResolveVersion(ctx, "pkg2", "ver:1")
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
		})

		t.Run("Missing ref", func(t *ftt.Test) {
			_, err := ResolveVersion(ctx, "pkg", "missing")
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such ref"))
		})

		t.Run("Missing tag", func(t *ftt.Test) {
			_, err := ResolveVersion(ctx, "pkg", "ver:missing")
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such tag"))
		})

		t.Run("Ambiguous tag", func(t *ftt.Test) {
			_, err := ResolveVersion(ctx, "pkg", "ver:ambiguous")
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("ambiguity when resolving the tag"))
		})
	})
}
