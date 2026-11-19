// Copyright 2026 The LUCI Authors.
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

package repo

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/perm"
	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestRepoHasPermissionHelpers(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	meta := testutil.MetadataStore{}
	pfxMeta := meta.Populate(prefix, &repopb.PrefixMetadata{
		Acls: []*repopb.PrefixMetadata_ACL{
			{
				Role:       repopb.Role_READER,
				Principals: []string{"user:legacy-reader@example.com"},
			},
		},
	})

	fakeDB := authtest.NewFakeDB(
		authtest.MockPermission("user:realm-reader@example.com", testRealm, perm.ListInstances.Perm()),
		authtest.MockPermission("user:realm-reader@example.com", testRealm, perm.DiagnoseACL.Perm()),
	)

	t.Run("lookup_prefix_cfg_error", func(t *testing.T) {
		t.Parallel()

		impl := newMockImpl(&meta, nil)
		impl.lookupPrefixCfg = func(ctx context.Context, pkg string) (*prefixcfg.Entry, error) {
			return nil, errors.New("config lookup boom")
		}

		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:realm-reader@example.com",
			FakeDB:   fakeDB,
		})
		cfg, has, err := impl.hasPermission(ctx, prefix, perm.ListInstances, nil)
		assert.Loosely(t, cfg, should.BeNil)
		assert.That(t, has, should.Match(perm.Set(nil)))
		assert.ErrIsLike(t, err, "config lookup boom")
	})

	t.Run("has_permission_realm_and_denied", func(t *testing.T) {
		t.Parallel()

		impl := newMockImpl(&meta, nil)
		impl.lookupPrefixCfg = func(ctx context.Context, pkg string) (*prefixcfg.Entry, error) {
			return &prefixcfg.Entry{
				Service: &prefixcfg.ServiceEntry{OwningLuciProject: "proj"},
				Realm:   testRealm,
			}, nil
		}

		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:realm-reader@example.com",
			FakeDB:   fakeDB,
		})
		cfg, has, err := impl.hasPermission(ctx, prefix, perm.ListInstances, perm.DiagnoseACL)
		assert.NoErr(t, err)
		assert.That(t, cfg.Realm, should.Equal(testRealm))
		assert.That(t, has, should.Match(perm.SetOf(perm.ListInstances, perm.DiagnoseACL)))

		_, _, err = impl.hasPermission(ctx, prefix, perm.WriteInstance, nil)
		assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		assert.ErrIsLike(t, err, `missing the cipd.packages.writeInstance permission`)
	})

	t.Run("prefix_perm_checker_wires_metadata_storage", func(t *testing.T) {
		t.Parallel()

		impl := newMockImpl(&meta, nil)
		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:legacy-reader@example.com",
			FakeDB:   fakeDB,
		})

		pchk, err := impl.prefixPermChecker(ctx, prefix)
		assert.NoErr(t, err)
		has, err := pchk.Check(ctx, perm.ListInstances, nil)
		assert.NoErr(t, err)
		assert.That(t, has, should.Match(perm.SetOf(perm.ListInstances)))

		metas, err := pchk.GetMetadata(ctx)
		assert.NoErr(t, err)
		assert.That(t, metas, should.Match([]*repopb.PrefixMetadata{pfxMeta}))
	})
}
