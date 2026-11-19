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

package perm

import (
	"cmp"
	"errors"
	"maps"
	"regexp"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestCheckRealms(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	fakeDB := authtest.NewFakeDB(
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, ListInstances.Perm()),
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-only-reader@admins.example.com", testRealm, ListInstances.Perm()),
		authtest.MockPermission("user:realm-diagnose-acl@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, ListInstances.Perm()),
		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, WriteInstance.Perm()),
	)

	cases := []struct {
		name string

		user  identity.Identity
		perm  Permission
		realm string

		dbOverride  *authtest.FakeDB
		wantCode    codes.Code
		wantErrLike string
	}{
		{
			name:  "realm_reader_allowed",
			user:  "user:realm-reader@admins.example.com",
			perm:  ListInstances,
			realm: testRealm,
		},
		{
			name:  "realm_writer_allowed",
			user:  "user:realm-writer@admins.example.com",
			perm:  WriteInstance,
			realm: testRealm,
		},
		{
			name:        "unmapped_realm_denied",
			user:        "user:realm-reader@admins.example.com",
			perm:        ListInstances,
			realm:       "",
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:realm-reader@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "detailed_error_when_caller_has_diagnose_acl_in_realm",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perm:        WriteInstance,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "generic_error_when_caller_lacks_diagnose_acl_in_realm",
			user:        "user:realm-only-reader@admins.example.com",
			perm:        WriteInstance,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:realm-only-reader@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "generic_error_for_anonymous_caller",
			user:        identity.AnonymousIdentity,
			perm:        ListInstances,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or not visible to unauthenticated callers`,
		},
		{
			name:        "authdb_perm_check_error",
			user:        "user:realm-writer@admins.example.com",
			dbOverride:  authtest.NewFakeDB(authtest.MockError(errors.New("authdb perm boom"))),
			perm:        WriteInstance,
			realm:       testRealm,
			wantErrLike: "authdb perm boom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &prefixcfg.Entry{
				Service: &prefixcfg.ServiceEntry{OwningLuciProject: "proj"},
				Realm:   tc.realm,
			}
			ctx := auth.WithState(t.Context(), &authtest.FakeState{
				Identity: tc.user,
				FakeDB:   cmp.Or(tc.dbOverride, fakeDB),
			})

			pchk := NewCache(ctx, nil).ForPrefix(prefix, cfg)
			has, err := pchk.Check(ctx, tc.perm, nil)
			if tc.wantCode != codes.OK || tc.wantErrLike != "" {
				assert.That(t, has, should.Match(Set(nil)))
				if tc.wantCode != codes.OK {
					assert.That(t, err, grpccode.ShouldBe(tc.wantCode))
				}
				assert.ErrIsLike(t, err, tc.wantErrLike)
				return
			}

			assert.NoErr(t, err)
			assert.That(t, pchk.Prefix(), should.Equal(prefix))
			assert.That(t, pchk.Config(), should.Equal(cfg))
			assert.That(t, has, should.Match(SetOf(tc.perm)))
		})
	}

	t.Run("empty_required_perm_panics", func(t *testing.T) {
		t.Parallel()

		cfg := &prefixcfg.Entry{
			Service: &prefixcfg.ServiceEntry{OwningLuciProject: "proj"},
			Realm:   testRealm,
		}
		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:realm-reader@admins.example.com",
			FakeDB:   fakeDB,
		})
		assert.That(t, func() {
			_, _ = NewCache(ctx, nil).ForPrefix(prefix, cfg).Check(ctx, Permission{}, nil)
		}, should.Panic)
	})
}

func TestCheckSetsAndOptional(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	fakeDB := authtest.NewFakeDB(
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, ListInstances.Perm()),
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-diagnose-acl@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, ListInstances.Perm()),
		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, WriteInstance.Perm()),
	)

	cases := []struct {
		name string

		user         identity.Identity
		perms        Checkable
		optional     Checkable
		wantOptional Checkable

		wantCode    codes.Code
		wantErrLike string
	}{
		{
			name:  "multi_perm_all_allowed_in_realm",
			user:  "user:realm-writer@admins.example.com",
			perms: SetOf(ListInstances, WriteInstance),
		},
		{
			name:  "multi_perm_ignores_zero_permission",
			user:  "user:realm-reader@admins.example.com",
			perms: SetOf(ListInstances, Permission{}),
		},
		{
			name:        "multi_perm_partial_missing_reports_only_missing",
			user:        "user:realm-reader@admins.example.com",
			perms:       SetOf(ListInstances, WriteInstance),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-reader@admins.example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "multi_perm_all_missing_reports_plural_sorted",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       SetOf(WriteInstance, ListInstances),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the [cipd.packages.listInstances,cipd.packages.writeInstance] permissions in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:         "optional_single_perm_granted_in_realm",
			user:         "user:realm-reader@admins.example.com",
			perms:        ListInstances,
			optional:     DiagnoseACL,
			wantOptional: DiagnoseACL,
		},
		{
			name:         "optional_multi_perm_partial_granted_in_realm",
			user:         "user:realm-writer@admins.example.com",
			perms:        ListInstances,
			optional:     SetOf(WriteInstance, ReadInstanceInfo, HidePackage),
			wantOptional: WriteInstance,
		},
		{
			name:        "optional_perms_omitted_from_error_when_required_perm_missing",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       ReadInstanceData,
			optional:    SetOf(ReadInstanceInfo, ListRefs, ReadInstanceTags),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the cipd.packages.readInstanceData permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:         "nil_required_with_optional_perms",
			user:         "user:realm-reader@admins.example.com",
			perms:        nil,
			optional:     SetOf(DiagnoseACL, WriteInstance),
			wantOptional: DiagnoseACL,
		},
		{
			name:     "nil_required_with_ungranted_optional_perms_succeeds",
			user:     identity.AnonymousIdentity,
			perms:    nil,
			optional: DiagnoseACL,
		},
		{
			name:         "nil_required_with_all_permissions_checkable",
			user:         "user:realm-reader@admins.example.com",
			perms:        nil,
			optional:     AllPermissionsSet(),
			wantOptional: SetOf(ListInstances, DiagnoseACL),
		},
	}

	cfg := &prefixcfg.Entry{
		Service: &prefixcfg.ServiceEntry{OwningLuciProject: "proj"},
		Realm:   testRealm,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := auth.WithState(t.Context(), &authtest.FakeState{
				Identity: tc.user,
				FakeDB:   fakeDB,
			})

			has, err := NewCache(ctx, nil).ForPrefix(prefix, cfg).Check(ctx, tc.perms, tc.optional)
			if tc.wantCode != codes.OK || tc.wantErrLike != "" {
				assert.That(t, has, should.Match(Set(nil)))
				assert.That(t, err, grpccode.ShouldBe(tc.wantCode))
				assert.ErrIsLike(t, err, tc.wantErrLike)
				return
			}

			assert.NoErr(t, err)
			var wantHas Set
			if tc.perms != nil {
				wantHas = maps.Clone(tc.perms.Perms())
			}
			if tc.wantOptional != nil {
				for p := range tc.wantOptional.Perms() {
					wantHas.Add(p)
				}
			}
			assert.That(t, has, should.Match(wantHas))
		})
	}

	t.Run("non_nil_empty_set_panics", func(t *testing.T) {
		t.Parallel()

		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:realm-reader@admins.example.com",
			FakeDB:   fakeDB,
		})
		assert.That(t, func() {
			var empty Set
			_, _ = NewCache(ctx, nil).ForPrefix(prefix, cfg).Check(ctx, empty, nil)
		}, should.Panic)
		assert.That(t, func() {
			_, _ = NewCache(ctx, nil).ForPrefix(prefix, cfg).Check(ctx, SetOf(), nil)
		}, should.Panic)
	})
}

func TestCheckAttrsUnconditional(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	fakeDB := authtest.NewFakeDB(
		authtest.MockPermission("user:cond@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission(
			"user:cond@admins.example.com", testRealm, WriteRef.Perm(),
			authtest.RestrictAttribute("cipd.package.refName", "latest"),
		),
		authtest.MockPermission("user:uncond@admins.example.com", testRealm, WriteRef.Perm()),
	)

	cfg := &prefixcfg.Entry{
		Service: &prefixcfg.ServiceEntry{OwningLuciProject: "proj"},
		Realm:   testRealm,
	}

	// Conditional grant denies Unconditional() check and omits it from AllPermissionsSet().
	condCtx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: "user:cond@admins.example.com",
		FakeDB:   fakeDB,
	})
	pchkCond := NewCache(condCtx, nil).ForPrefix(prefix, cfg)
	has, err := pchkCond.Check(condCtx, WriteRef.Unconditional(), nil)
	assert.That(t, has, should.Match(Set(nil)))
	assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	assert.ErrIsLike(t, err, `"user:cond@admins.example.com" is missing the cipd.packages.writeRef permission in prefix "a/b" (realm "proj:realm")`)

	has, err = pchkCond.Check(condCtx, nil, AllPermissionsSet())
	assert.NoErr(t, err)
	assert.That(t, has, should.Match(SetOf(DiagnoseACL)))

	// Unconditional grant allows both Unconditional() and Bind(...) and appears in AllPermissionsSet().
	uncondCtx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: "user:uncond@admins.example.com",
		FakeDB:   fakeDB,
	})
	pchkUncond := NewCache(uncondCtx, nil).ForPrefix(prefix, cfg)
	has, err = pchkUncond.Check(uncondCtx, WriteRef.Unconditional(), WriteRef.Bind("any"))
	assert.NoErr(t, err)
	assert.That(t, has, should.Match(SetOf(WriteRef.Unconditional(), WriteRef.Bind("any"))))

	has, err = pchkUncond.Check(uncondCtx, nil, AllPermissionsSet())
	assert.NoErr(t, err)
	assert.That(t, has, should.Match(SetOf(WriteRef.Unconditional())))
}

func TestCheckAttrs(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	fakeDB := authtest.NewFakeDB(
		authtest.MockPermission("user:realm-diagnose-acl@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission(
			"user:realm-ref-writer@admins.example.com", testRealm, WriteRef.Perm(),
			authtest.RestrictAttribute("cipd.package.refName", "latest"),
		),
	)

	writeAlphaPerm := WriteRef.Bind("alpha")
	writeLatestPerm := WriteRef.Bind("latest")
	writeProdPerm := WriteRef.Bind("prod")

	cases := []struct {
		name string

		user         identity.Identity
		perms        Checkable
		optional     Checkable
		wantOptional Checkable

		wantCode    codes.Code
		wantErrLike string
	}{
		{
			name:  "conditional_attr_allowed",
			user:  "user:realm-ref-writer@admins.example.com",
			perms: writeLatestPerm,
		},
		{
			name:        "conditional_attr_denied",
			user:        "user:realm-ref-writer@admins.example.com",
			perms:       writeProdPerm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:realm-ref-writer@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "multi_perm_duplicate_bound_attrs_deduplicated",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       SetOf(WriteRef.Bind("alpha"), WriteRef.Bind("alpha")),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the cipd.packages.writeRef{cipd.package.refName:"alpha"} permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "multi_perm_same_perm_different_attrs_one_missing",
			user:        "user:realm-ref-writer@admins.example.com",
			perms:       SetOf(writeLatestPerm, writeProdPerm),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:realm-ref-writer@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "multi_perm_with_attrs_detailed_error_sorted_by_attr",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       SetOf(writeProdPerm, writeAlphaPerm),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the [cipd.packages.writeRef{cipd.package.refName:"alpha"},cipd.packages.writeRef{cipd.package.refName:"prod"}] permissions in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:         "optional_perm_with_bound_attr",
			user:         "user:realm-ref-writer@admins.example.com",
			perms:        writeLatestPerm,
			optional:     SetOf(writeLatestPerm, writeProdPerm),
			wantOptional: writeLatestPerm,
		},
	}

	cfg := &prefixcfg.Entry{
		Service: &prefixcfg.ServiceEntry{OwningLuciProject: "proj"},
		Realm:   testRealm,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := auth.WithState(t.Context(), &authtest.FakeState{
				Identity: tc.user,
				FakeDB:   fakeDB,
			})

			has, err := NewCache(ctx, nil).ForPrefix(prefix, cfg).Check(ctx, tc.perms, tc.optional)
			if tc.wantCode != codes.OK || tc.wantErrLike != "" {
				assert.That(t, has, should.Match(Set(nil)))
				assert.That(t, err, grpccode.ShouldBe(tc.wantCode))
				assert.ErrIsLike(t, err, tc.wantErrLike)
				return
			}

			assert.NoErr(t, err)
			wantHas := maps.Clone(tc.perms.Perms())
			if tc.wantOptional != nil {
				for p := range tc.wantOptional.Perms() {
					wantHas.Add(p)
				}
			}
			assert.That(t, has, should.Match(wantHas))
		})
	}
}

func TestCheckGroups(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	fakeDB := authtest.NewFakeDB(
		authtest.MockMembership("user:viewer@admins.example.com", PrefixesViewers),
		authtest.MockMembership("user:admin@admins.example.com", "administrators"),

		// Even if granted in a realm, DeletePackage must not be checked against realms.
		authtest.MockPermission("user:realm-delete-holder@admins.example.com", testRealm, DeletePackage.Perm()),
		authtest.MockPermission("user:realm-delete-holder@admins.example.com", testRealm, DiagnoseACL.Perm()),

		// user@ is not in PrefixesViewers, but has ReadLegacyRoles in the realm.
		// Conversely, viewer@ IS in PrefixesViewers, but doesn't explicitly have ReadLegacyRoles in the realm.
		authtest.MockPermission("user:user@example.com", testRealm, ReadLegacyRoles.Perm()),
	)

	cases := []struct {
		name string

		user  identity.Identity
		perms Checkable
		realm string

		dbOverride  *authtest.FakeDB
		wantCode    codes.Code
		wantErrLike string
	}{
		{
			name:  "implied_group_prefixes_viewers_allows_read",
			user:  "user:viewer@admins.example.com",
			perms: ListInstances,
		},
		{
			name:        "implied_group_prefixes_viewers_denies_write_with_detailed_error",
			user:        "user:viewer@admins.example.com",
			perms:       WriteInstance,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:viewer@admins.example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "implied_group_prefixes_viewers_denies_delete",
			user:        "user:viewer@admins.example.com",
			perms:       DeletePackage,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:viewer@admins.example.com" is missing the cipd.packages.delete permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:  "implied_group_administrators_allows_delete",
			user:  "user:admin@admins.example.com",
			perms: DeletePackage,
		},
		{
			name:        "delete_package_does_not_check_realm",
			user:        "user:realm-delete-holder@admins.example.com",
			perms:       DeletePackage,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-delete-holder@admins.example.com" is missing the cipd.packages.delete permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "authdb_group_check_error",
			user:        "user:viewer@admins.example.com",
			dbOverride:  authtest.NewFakeDB(authtest.MockError(errors.New("authdb group boom"))),
			perms:       ListInstances,
			realm:       testRealm,
			wantErrLike: "authdb group boom",
		},
		{
			name:  "user_has_permission_without_group",
			user:  "user:user@example.com",
			realm: testRealm,
			perms: ReadLegacyRoles,
		},
		{
			name:  "viewer_has_permission_via_group",
			user:  "user:viewer@admins.example.com",
			realm: testRealm,
			perms: ReadLegacyRoles,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &prefixcfg.Entry{
				Service: &prefixcfg.ServiceEntry{OwningLuciProject: "proj"},
				Realm:   tc.realm,
			}
			ctx := auth.WithState(t.Context(), &authtest.FakeState{
				Identity: tc.user,
				FakeDB:   cmp.Or(tc.dbOverride, fakeDB),
			})

			has, err := NewCache(ctx, nil).ForPrefix(prefix, cfg).Check(ctx, tc.perms, nil)
			if tc.wantCode != codes.OK || tc.wantErrLike != "" {
				assert.That(t, has, should.Match(Set(nil)))
				if tc.wantCode != codes.OK {
					assert.That(t, err, grpccode.ShouldBe(tc.wantCode))
				}
				assert.ErrIsLike(t, err, tc.wantErrLike)
				return
			}

			assert.NoErr(t, err)
			assert.That(t, has, should.Match(tc.perms.Perms()))
		})
	}
}

func TestCheckWritePolicy(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	adminsOnlyWriters := []*regexp.Regexp{regexp.MustCompile(`^.*@admins\.example\.com$`)}

	fakeDB := authtest.NewFakeDB(
		authtest.MockMembership("user:viewer@example.com", PrefixesViewers),
		authtest.MockMembership("user:admin@example.com", "administrators"),

		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, WriteInstance.Perm()),
		authtest.MockPermission("user:realm-writer@example.com", testRealm, ListInstances.Perm()),
		authtest.MockPermission("user:realm-writer@example.com", testRealm, WriteInstance.Perm()),
		authtest.MockPermission("user:realm-writer@example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-diagnose@example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-legacy-roles-reader@example.com", testRealm, ReadLegacyRoles.Perm()),
	)

	cases := []struct {
		name string

		user         identity.Identity
		perms        Checkable
		optional     Checkable
		wantOptional Checkable

		dbOverride  *authtest.FakeDB
		wantCode    codes.Code
		wantErrLike string
	}{
		{
			name:        "writers_regexp_blocks_disallowed_realm_writer",
			user:        "user:realm-writer@example.com",
			perms:       WriteInstance,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-writer@example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm"): rejected by global policy`,
		},
		{
			name:  "writers_regexp_allows_matching_writer",
			user:  "user:realm-writer@admins.example.com",
			perms: WriteInstance,
		},
		{
			name:  "writers_regexp_does_not_block_read_permission",
			user:  "user:realm-writer@example.com",
			perms: ListInstances,
		},
		{
			name:  "writers_regexp_does_not_block_read_legacy_roles_via_realm",
			user:  "user:realm-legacy-roles-reader@example.com",
			perms: ReadLegacyRoles,
		},
		{
			name:  "writers_regexp_does_not_block_read_legacy_roles_for_prefixes_viewer",
			user:  "user:viewer@example.com",
			perms: ReadLegacyRoles,
		},
		{
			name:        "writers_regexp_blocks_delete_package",
			user:        "user:admin@example.com",
			perms:       DeletePackage,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:admin@example.com" is not allowed to see it`,
		},
		{
			name:        "multi_perm_writers_regexp_blocks_only_write_perm",
			user:        "user:realm-writer@example.com",
			perms:       SetOf(ListInstances, WriteInstance),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-writer@example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm"): rejected by global policy`,
		},
		{
			name:        "multi_perm_both_read_and_write_missing_reports_policy_rejected_write_perm",
			user:        "user:realm-diagnose@example.com",
			perms:       SetOf(ListInstances, WriteInstance),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose@example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm"): rejected by global policy`,
		},
		{
			name:         "nil_required_with_writers_regexp_filters_write_optional_perms",
			user:         "user:realm-writer@example.com",
			perms:        nil,
			optional:     SetOf(ListInstances, DiagnoseACL, WriteInstance),
			wantOptional: SetOf(ListInstances, DiagnoseACL),
		},
		{
			name:        "can_diagnose_authdb_error_propagated",
			user:        "user:realm-writer@example.com",
			dbOverride:  authtest.NewFakeDB(authtest.MockError(errors.New("authdb diagnose boom"))),
			perms:       WriteInstance,
			wantErrLike: "authdb diagnose boom",
		},
	}

	cfg := &prefixcfg.Entry{
		Service: &prefixcfg.ServiceEntry{
			OwningLuciProject:      "proj",
			AllowWritersFromRegexp: adminsOnlyWriters,
		},
		Realm: testRealm,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := auth.WithState(t.Context(), &authtest.FakeState{
				Identity: tc.user,
				FakeDB:   cmp.Or(tc.dbOverride, fakeDB),
			})

			has, err := NewCache(ctx, nil).ForPrefix(prefix, cfg).Check(ctx, tc.perms, tc.optional)
			if tc.wantCode != codes.OK || tc.wantErrLike != "" {
				assert.That(t, has, should.Match(Set(nil)))
				if tc.wantCode != codes.OK {
					assert.That(t, err, grpccode.ShouldBe(tc.wantCode))
				}
				assert.ErrIsLike(t, err, tc.wantErrLike)
				return
			}

			assert.NoErr(t, err)
			var wantHas Set
			if tc.perms != nil {
				wantHas = maps.Clone(tc.perms.Perms())
			}
			if tc.wantOptional != nil {
				for p := range tc.wantOptional.Perms() {
					wantHas.Add(p)
				}
			}
			assert.That(t, has, should.Match(wantHas))
		})
	}
}

func TestPrefixCacheHasRoleEdgeCases(t *testing.T) {
	t.Parallel()

	meta := testutil.MetadataStore{}
	cfg := &prefixcfg.Entry{
		Service: &prefixcfg.ServiceEntry{EnableLegacyAclFallback: true},
	}
	ctx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: "user:stranger@example.com",
		FakeDB:   authtest.NewFakeDB(),
	})

	pchk := NewCache(ctx, &meta).ForPrefix("a/b", cfg)

	// Out-of-bounds roles return false without touching metadata.
	has, err := pchk.HasRole(ctx, repopb.Role_ROLE_UNSPECIFIED)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, pchk.gotMeta, should.BeFalse)

	has, err = pchk.HasRole(ctx, repopb.Role(999))
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, pchk.gotMeta, should.BeFalse)

	// Negative role implication: lacking READER implies lacking WRITER and OWNER
	// without additional evaluations in checkedRoles.
	has, err = pchk.HasRole(ctx, repopb.Role_READER)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, pchk.gotMeta, should.BeTrue)

	has, err = pchk.HasRole(ctx, repopb.Role_WRITER)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, pchk.checkedRoles, should.Match(boolCache[repopb.Role]{repopb.Role_READER: false}))

	// GetMetadata error propagates without setting gotMeta.
	badPchk := NewCache(ctx, &meta).ForPrefix("//bad", cfg)
	_, err = badPchk.Check(ctx, DiagnoseACL, nil)
	assert.Loosely(t, err, should.NotBeNil)
	assert.That(t, badPchk.gotMeta, should.BeFalse)
}

func TestCheckLegacyFallback(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	meta := testutil.MetadataStore{}
	rootMeta := meta.Populate("", &repopb.PrefixMetadata{
		Acls: []*repopb.PrefixMetadata_ACL{
			{
				Role:       repopb.Role_OWNER,
				Principals: []string{"user:legacy-root-owner@admins.example.com"},
			},
		},
	})
	pfxMeta := meta.Populate(prefix, &repopb.PrefixMetadata{
		Acls: []*repopb.PrefixMetadata_ACL{
			{
				Role:       repopb.Role_READER,
				Principals: []string{"user:legacy-reader@admins.example.com"},
			},
			{
				Role:       repopb.Role_WRITER,
				Principals: []string{"user:legacy-writer@admins.example.com", "user:legacy-writer@example.com", "group:legacy-writers"},
			},
			{
				Role:       repopb.Role_OWNER,
				Principals: []string{"user:legacy-owner@admins.example.com"},
			},
		},
	})
	expectedMetas := []*repopb.PrefixMetadata{rootMeta, pfxMeta}

	adminsOnlyWriters := []*regexp.Regexp{regexp.MustCompile(`^.*@admins\.example\.com$`)}

	fakeDB := authtest.NewFakeDB(
		authtest.MockMembership("user:group-legacy-writer@admins.example.com", "legacy-writers"),
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, ListInstances.Perm()),
		authtest.MockPermission("user:realm-diagnose-acl@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:legacy-root-owner@admins.example.com", testRealm, DeletePackage.Perm()),
		authtest.MockPermission("user:legacy-root-owner@admins.example.com", testRealm, DiagnoseACL.Perm()),
	)

	cases := []struct {
		name string

		user         identity.Identity
		perms        Checkable
		optional     Checkable
		wantOptional Checkable

		realm           string
		disableFallback bool
		prefixcfg.ServiceEntry

		dbOverride  *authtest.FakeDB
		wantCode    codes.Code
		wantErrLike string
	}{
		{
			name:            "legacy_role_ignored_when_fallback_disabled",
			user:            "user:legacy-owner@admins.example.com",
			perms:           ListInstances,
			realm:           testRealm,
			disableFallback: true,
			wantCode:        codes.PermissionDenied,
			wantErrLike:     `prefix "a/b" doesn't exist or "user:legacy-owner@admins.example.com" is not allowed to see it`,
		},
		{
			name:  "fallback_to_legacy_reader_with_realm_configured",
			user:  "user:legacy-reader@admins.example.com",
			perms: ReadInstanceMetadata,
			realm: testRealm,
		},
		{
			name:  "fallback_to_inherited_legacy_owner_when_no_realm_mapped",
			user:  "user:legacy-root-owner@admins.example.com",
			perms: DetachInstanceMetadata.Bind("some-key"),
		},
		{
			name:        "fallback_denied_returns_detailed_error_when_caller_has_realm_diagnose_acl",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       WriteInstance,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm"), and also does not have role: WRITER`,
		},
		{
			name:        "fallback_denied_returns_detailed_error_via_legacy_reader_diagnose_acl",
			user:        "user:legacy-reader@admins.example.com",
			perms:       WriteInstance,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:legacy-reader@admins.example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm"), and also does not have role: WRITER`,
		},
		{
			name:        "fallback_denied_returns_generic_error_for_stranger",
			user:        "user:stranger@admins.example.com",
			perms:       WriteInstance,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:stranger@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "delete_package_does_not_check_realm_or_legacy_fallback",
			user:        "user:legacy-root-owner@admins.example.com",
			perms:       DeletePackage,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:legacy-root-owner@admins.example.com" is missing the cipd.packages.delete permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:  "writers_regexp_blocks_disallowed_legacy_writer_but_preserves_legacy_diagnose",
			user:  "user:legacy-writer@example.com",
			perms: WriteInstance,
			realm: testRealm,
			ServiceEntry: prefixcfg.ServiceEntry{
				AllowWritersFromRegexp: adminsOnlyWriters,
			},
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:legacy-writer@example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm"): rejected by global policy`,
		},
		{
			name:  "writers_regexp_does_not_block_read_permission_via_legacy_fallback",
			user:  "user:legacy-writer@example.com",
			perms: ListInstances,
			realm: testRealm,
			ServiceEntry: prefixcfg.ServiceEntry{
				AllowWritersFromRegexp: adminsOnlyWriters,
			},
		},
		{
			name:  "multi_perm_fallback_allowed",
			user:  "user:legacy-writer@admins.example.com",
			perms: SetOf(ListInstances, WriteInstance),
			realm: testRealm,
		},
		{
			name:        "multi_perm_fallback_partial_satisfied_reports_only_missing_and_max_role",
			user:        "user:legacy-reader@admins.example.com",
			perms:       SetOf(ListInstances, WriteInstance),
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:legacy-reader@admins.example.com" is missing the cipd.packages.writeInstance permission in prefix "a/b" (realm "proj:realm"), and also does not have role: WRITER`,
		},
		{
			name:        "multi_perm_fallback_all_missing_reports_plural_and_max_role",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       SetOf(ListInstances, HidePackage),
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the [cipd.packages.hide,cipd.packages.listInstances] permissions in prefix "a/b" (realm "proj:realm"), and also does not have role: OWNER`,
		},
		{
			name:         "optional_multi_perm_granted_via_legacy_fallback",
			user:         "user:legacy-reader@admins.example.com",
			perms:        ReadInstanceData,
			optional:     SetOf(ReadInstanceInfo, ListRefs, WriteInstance),
			wantOptional: SetOf(ReadInstanceInfo, ListRefs),
			realm:        testRealm,
		},
		{
			name:         "nil_required_with_optional_perms_via_legacy_fallback",
			user:         "user:legacy-reader@admins.example.com",
			perms:        nil,
			optional:     SetOf(DiagnoseACL, WriteInstance),
			wantOptional: DiagnoseACL,
			realm:        testRealm,
		},
		{
			name:        "check_role_fallback_error",
			user:        "user:stranger@admins.example.com",
			dbOverride:  authtest.NewFakeDB(authtest.MockError(errors.New("authdb role boom"))),
			perms:       WriteInstance,
			wantErrLike: "authdb role boom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			se := tc.ServiceEntry
			se.OwningLuciProject = cmp.Or(se.OwningLuciProject, "proj")
			if !tc.disableFallback {
				se.EnableLegacyAclFallback = true
			}
			cfg := &prefixcfg.Entry{Service: &se, Realm: tc.realm}

			ctx := auth.WithState(t.Context(), &authtest.FakeState{
				Identity: tc.user,
				FakeDB:   cmp.Or(tc.dbOverride, fakeDB),
			})

			has, err := NewCache(ctx, &meta).ForPrefix(prefix, cfg).Check(ctx, tc.perms, tc.optional)
			if tc.wantCode != codes.OK || tc.wantErrLike != "" {
				assert.That(t, has, should.Match(Set(nil)))
				if tc.wantCode != codes.OK {
					assert.That(t, err, grpccode.ShouldBe(tc.wantCode))
				}
				assert.ErrIsLike(t, err, tc.wantErrLike)
				return
			}

			assert.NoErr(t, err)
			var wantHas Set
			if tc.perms != nil {
				wantHas = maps.Clone(tc.perms.Perms())
			}
			if tc.wantOptional != nil {
				for p := range tc.wantOptional.Perms() {
					wantHas.Add(p)
				}
			}
			assert.That(t, has, should.Match(wantHas))
		})
	}

	t.Run("prefix_cache_lazy_metadata_and_role_implication", func(t *testing.T) {
		t.Parallel()

		cfg := &prefixcfg.Entry{
			Service: &prefixcfg.ServiceEntry{
				OwningLuciProject:       "proj",
				EnableLegacyAclFallback: true,
			},
			Realm: testRealm,
		}

		// When satisfied by realm permissions, metadata is not fetched eagerly,
		// but GetMetadata fetches it lazily on demand.
		realmCtx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:realm-reader@admins.example.com",
			FakeDB:   fakeDB,
		})
		pchk := NewCache(realmCtx, &meta).ForPrefix(prefix, cfg)
		_, err := pchk.Check(realmCtx, ListInstances, nil)
		assert.NoErr(t, err)
		assert.That(t, pchk.gotMeta, should.BeFalse)

		metas, err := pchk.GetMetadata(realmCtx)
		assert.NoErr(t, err)
		assert.That(t, pchk.gotMeta, should.BeTrue)
		assert.That(t, metas, should.Match(expectedMetas))

		// When satisfied via legacy WRITER role, READER is implied without
		// recomputing, and metas is already cached.
		legacyCtx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:group-legacy-writer@admins.example.com",
			FakeDB:   fakeDB,
		})
		pchk2 := NewCache(legacyCtx, &meta).ForPrefix(prefix, cfg)
		got, err := pchk2.Check(legacyCtx, WriteInstance, ListInstances)
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(SetOf(WriteInstance, ListInstances)))
		assert.That(t, pchk2.gotMeta, should.BeTrue)
		assert.That(t, pchk2.checkedRoles, should.Match(boolCache[repopb.Role]{repopb.Role_WRITER: true}))

		// Multi-prefix isolation: checkedRoles on "a/b" does not leak to sibling "other".
		siblingMeta := testutil.MetadataStore{}
		siblingMeta.Populate("a/b", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{Role: repopb.Role_READER, Principals: []string{"user:scoped-reader@admins.example.com"}},
			},
		})
		cfgWithFallback := &prefixcfg.Entry{
			Service: &prefixcfg.ServiceEntry{EnableLegacyAclFallback: true},
		}
		scopedCtx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:scoped-reader@admins.example.com",
			FakeDB:   fakeDB,
		})
		pcache := NewCache(scopedCtx, &siblingMeta)
		_, err = pcache.ForPrefix("a/b", cfgWithFallback).Check(scopedCtx, ListInstances, nil)
		assert.NoErr(t, err)
		_, err = pcache.ForPrefix("other", cfgWithFallback).Check(scopedCtx, ListInstances, nil)
		assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})
}
