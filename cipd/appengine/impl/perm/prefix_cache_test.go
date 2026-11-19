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

	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
)

func TestCheckRealms(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "a/b"
		testRealm = "proj:realm"
	)

	fakeDB := authtest.NewFakeDB(
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, testPermRead.Perm()),
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-only-reader@admins.example.com", testRealm, testPermRead.Perm()),
		authtest.MockPermission("user:realm-diagnose-acl@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, testPermRead.Perm()),
		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, testPermWrite.Perm()),
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
			perm:  testPermRead,
			realm: testRealm,
		},
		{
			name:  "realm_writer_allowed",
			user:  "user:realm-writer@admins.example.com",
			perm:  testPermWrite,
			realm: testRealm,
		},
		{
			name:        "unmapped_realm_denied",
			user:        "user:realm-reader@admins.example.com",
			perm:        testPermRead,
			realm:       "",
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:realm-reader@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "detailed_error_when_caller_has_diagnose_acl_in_realm",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perm:        testPermWrite,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the cipd.testing.write permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "generic_error_when_caller_lacks_diagnose_acl_in_realm",
			user:        "user:realm-only-reader@admins.example.com",
			perm:        testPermWrite,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:realm-only-reader@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "generic_error_for_anonymous_caller",
			user:        identity.AnonymousIdentity,
			perm:        testPermRead,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or not visible to unauthenticated callers`,
		},
		{
			name:        "authdb_perm_check_error",
			user:        "user:realm-writer@admins.example.com",
			dbOverride:  authtest.NewFakeDB(authtest.MockError(errors.New("authdb perm boom"))),
			perm:        testPermWrite,
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

			pchk := NewCache(ctx).ForPrefix(prefix, cfg)
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
			_, _ = NewCache(ctx).ForPrefix(prefix, cfg).Check(ctx, Permission{}, nil)
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
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, testPermRead.Perm()),
		authtest.MockPermission("user:realm-reader@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-diagnose-acl@admins.example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, testPermRead.Perm()),
		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, testPermWrite.Perm()),
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
			perms: SetOf(testPermRead, testPermWrite),
		},
		{
			name:  "multi_perm_ignores_zero_permission",
			user:  "user:realm-reader@admins.example.com",
			perms: SetOf(testPermRead, Permission{}),
		},
		{
			name:        "multi_perm_partial_missing_reports_only_missing",
			user:        "user:realm-reader@admins.example.com",
			perms:       SetOf(testPermRead, testPermWrite),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-reader@admins.example.com" is missing the cipd.testing.write permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "multi_perm_all_missing_reports_plural_sorted",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       SetOf(testPermWrite, testPermRead),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the [cipd.testing.read,cipd.testing.write] permissions in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:         "optional_single_perm_granted_in_realm",
			user:         "user:realm-reader@admins.example.com",
			perms:        testPermRead,
			optional:     DiagnoseACL,
			wantOptional: DiagnoseACL,
		},
		{
			name:         "optional_multi_perm_partial_granted_in_realm",
			user:         "user:realm-writer@admins.example.com",
			perms:        testPermRead,
			optional:     SetOf(testPermWrite, testPermExtra, DiagnoseACL),
			wantOptional: testPermWrite,
		},
		{
			name:        "optional_perms_omitted_from_error_when_required_perm_missing",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       testPermWrite,
			optional:    SetOf(testPermRead, testPermExtra),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the cipd.testing.write permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:         "nil_required_with_optional_perms",
			user:         "user:realm-reader@admins.example.com",
			perms:        nil,
			optional:     SetOf(DiagnoseACL, testPermWrite),
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
			wantOptional: SetOf(testPermRead, DiagnoseACL),
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

			has, err := NewCache(ctx).ForPrefix(prefix, cfg).Check(ctx, tc.perms, tc.optional)
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
			_, _ = NewCache(ctx).ForPrefix(prefix, cfg).Check(ctx, empty, nil)
		}, should.Panic)
		assert.That(t, func() {
			_, _ = NewCache(ctx).ForPrefix(prefix, cfg).Check(ctx, SetOf(), nil)
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
			"user:cond@admins.example.com", testRealm, testPermAttr.Perm(),
			authtest.RestrictAttribute("cipd.testing.key", "latest"),
		),
		authtest.MockPermission("user:uncond@admins.example.com", testRealm, testPermAttr.Perm()),
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
	pchkCond := NewCache(condCtx).ForPrefix(prefix, cfg)
	has, err := pchkCond.Check(condCtx, testPermAttr.Unconditional(), nil)
	assert.That(t, has, should.Match(Set(nil)))
	assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	assert.ErrIsLike(t, err, `"user:cond@admins.example.com" is missing the cipd.testing.attr permission in prefix "a/b" (realm "proj:realm")`)

	has, err = pchkCond.Check(condCtx, nil, AllPermissionsSet())
	assert.NoErr(t, err)
	assert.That(t, has, should.Match(SetOf(DiagnoseACL)))

	// Unconditional grant allows both Unconditional() and Bind(...) and appears in AllPermissionsSet().
	uncondCtx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: "user:uncond@admins.example.com",
		FakeDB:   fakeDB,
	})
	pchkUncond := NewCache(uncondCtx).ForPrefix(prefix, cfg)
	has, err = pchkUncond.Check(uncondCtx, testPermAttr.Unconditional(), testPermAttr.Bind("any"))
	assert.NoErr(t, err)
	assert.That(t, has, should.Match(SetOf(testPermAttr.Unconditional(), testPermAttr.Bind("any"))))

	has, err = pchkUncond.Check(uncondCtx, nil, AllPermissionsSet())
	assert.NoErr(t, err)
	assert.That(t, has, should.Match(SetOf(testPermAttr.Unconditional())))
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
			"user:realm-attr-writer@admins.example.com", testRealm, testPermAttr.Perm(),
			authtest.RestrictAttribute("cipd.testing.key", "latest"),
		),
	)

	attrAlpha := testPermAttr.Bind("alpha")
	attrLatest := testPermAttr.Bind("latest")
	attrProd := testPermAttr.Bind("prod")

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
			user:  "user:realm-attr-writer@admins.example.com",
			perms: attrLatest,
		},
		{
			name:        "conditional_attr_denied",
			user:        "user:realm-attr-writer@admins.example.com",
			perms:       attrProd,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:realm-attr-writer@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "multi_perm_duplicate_bound_attrs_deduplicated",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       SetOf(testPermAttr.Bind("alpha"), testPermAttr.Bind("alpha")),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the cipd.testing.attr{cipd.testing.key:"alpha"} permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "multi_perm_same_perm_different_attrs_one_missing",
			user:        "user:realm-attr-writer@admins.example.com",
			perms:       SetOf(attrLatest, attrProd),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:realm-attr-writer@admins.example.com" is not allowed to see it`,
		},
		{
			name:        "multi_perm_with_attrs_detailed_error_sorted_by_attr",
			user:        "user:realm-diagnose-acl@admins.example.com",
			perms:       SetOf(attrProd, attrAlpha),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose-acl@admins.example.com" is missing the [cipd.testing.attr{cipd.testing.key:"alpha"},cipd.testing.attr{cipd.testing.key:"prod"}] permissions in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:         "optional_perm_with_bound_attr",
			user:         "user:realm-attr-writer@admins.example.com",
			perms:        attrLatest,
			optional:     SetOf(attrLatest, attrProd),
			wantOptional: attrLatest,
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

			has, err := NewCache(ctx).ForPrefix(prefix, cfg).Check(ctx, tc.perms, tc.optional)
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

		// Even if granted in a realm, a fake permission must not be checked against realms.
		authtest.MockPermission("user:realm-fake-holder@admins.example.com", testRealm, testPermFake.Perm()),
		authtest.MockPermission("user:realm-fake-holder@admins.example.com", testRealm, DiagnoseACL.Perm()),

		// user@ is not an admin, but has testPermGroup.
		// Conversely, admin@ IS an admin, but doesn't explicitly have testPermGroup.
		authtest.MockPermission("user:user@example.com", testRealm, testPermGroup.Perm()),
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
			perms: testPermRead,
		},
		{
			name:        "implied_group_prefixes_viewers_denies_write_with_detailed_error",
			user:        "user:viewer@admins.example.com",
			perms:       testPermWrite,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:viewer@admins.example.com" is missing the cipd.testing.write permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "implied_group_prefixes_viewers_denies_fake_admin_perm",
			user:        "user:viewer@admins.example.com",
			perms:       testPermFake,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:viewer@admins.example.com" is missing the cipd.testing.fake permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:  "implied_group_administrators_allows_fake_admin_perm",
			user:  "user:admin@admins.example.com",
			perms: testPermFake,
		},
		{
			name:        "fake_perm_does_not_check_realm",
			user:        "user:realm-fake-holder@admins.example.com",
			perms:       testPermFake,
			realm:       testRealm,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-fake-holder@admins.example.com" is missing the cipd.testing.fake permission in prefix "a/b" (realm "proj:realm")`,
		},
		{
			name:        "authdb_group_check_error",
			user:        "user:viewer@admins.example.com",
			dbOverride:  authtest.NewFakeDB(authtest.MockError(errors.New("authdb group boom"))),
			perms:       testPermRead,
			realm:       testRealm,
			wantErrLike: "authdb group boom",
		},
		{
			name:  "user_has_permission_without_group",
			user:  "user:user@example.com",
			realm: testRealm,
			perms: testPermGroup,
		},
		{
			name:  "admin_has_permission_via_group",
			user:  "user:admin@admins.example.com",
			realm: testRealm,
			perms: testPermGroup,
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

			has, err := NewCache(ctx).ForPrefix(prefix, cfg).Check(ctx, tc.perms, nil)
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

		authtest.MockPermission("user:realm-writer@admins.example.com", testRealm, testPermWrite.Perm()),
		authtest.MockPermission("user:realm-writer@example.com", testRealm, testPermRead.Perm()),
		authtest.MockPermission("user:realm-writer@example.com", testRealm, testPermWrite.Perm()),
		authtest.MockPermission("user:realm-writer@example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-diagnose@example.com", testRealm, DiagnoseACL.Perm()),
		authtest.MockPermission("user:realm-nonwrite@example.com", testRealm, testPermExtra.Perm()),
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
			perms:       testPermWrite,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-writer@example.com" is missing the cipd.testing.write permission in prefix "a/b" (realm "proj:realm"): rejected by global policy`,
		},
		{
			name:  "writers_regexp_allows_matching_writer",
			user:  "user:realm-writer@admins.example.com",
			perms: testPermWrite,
		},
		{
			name:  "writers_regexp_does_not_block_read_permission",
			user:  "user:realm-writer@example.com",
			perms: testPermRead,
		},
		{
			name:  "writers_regexp_does_not_block_non_write_perm_via_realm",
			user:  "user:realm-nonwrite@example.com",
			perms: testPermExtra,
		},
		{
			name:  "writers_regexp_does_not_block_non_write_perm_for_prefixes_viewer",
			user:  "user:viewer@example.com",
			perms: testPermRead,
		},
		{
			name:        "writers_regexp_blocks_fake_write_perm",
			user:        "user:admin@example.com",
			perms:       testPermFake,
			wantCode:    codes.PermissionDenied,
			wantErrLike: `prefix "a/b" doesn't exist or "user:admin@example.com" is not allowed to see it`,
		},
		{
			name:        "multi_perm_writers_regexp_blocks_only_write_perm",
			user:        "user:realm-writer@example.com",
			perms:       SetOf(testPermRead, testPermWrite),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-writer@example.com" is missing the cipd.testing.write permission in prefix "a/b" (realm "proj:realm"): rejected by global policy`,
		},
		{
			name:        "multi_perm_both_read_and_write_missing_reports_policy_rejected_write_perm",
			user:        "user:realm-diagnose@example.com",
			perms:       SetOf(testPermRead, testPermWrite),
			wantCode:    codes.PermissionDenied,
			wantErrLike: `"user:realm-diagnose@example.com" is missing the cipd.testing.write permission in prefix "a/b" (realm "proj:realm"): rejected by global policy`,
		},
		{
			name:         "nil_required_with_writers_regexp_filters_write_optional_perms",
			user:         "user:realm-writer@example.com",
			perms:        nil,
			optional:     SetOf(testPermRead, DiagnoseACL, testPermWrite),
			wantOptional: SetOf(testPermRead, DiagnoseACL),
		},
		{
			name:        "can_diagnose_authdb_error_propagated",
			user:        "user:realm-writer@example.com",
			dbOverride:  authtest.NewFakeDB(authtest.MockError(errors.New("authdb diagnose boom"))),
			perms:       testPermWrite,
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

			has, err := NewCache(ctx).ForPrefix(prefix, cfg).Check(ctx, tc.perms, tc.optional)
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
