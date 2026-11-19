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
			err := pchk.Check(ctx, tc.perm)
			if tc.wantCode != codes.OK || tc.wantErrLike != "" {
				if tc.wantCode != codes.OK {
					assert.That(t, err, grpccode.ShouldBe(tc.wantCode))
				}
				assert.ErrIsLike(t, err, tc.wantErrLike)
				return
			}

			assert.NoErr(t, err)
			assert.That(t, pchk.Prefix(), should.Equal(prefix))
			assert.That(t, pchk.Config(), should.Equal(cfg))
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
			_ = NewCache(ctx).ForPrefix(prefix, cfg).Check(ctx, Permission{})
		}, should.Panic)
	})
}
