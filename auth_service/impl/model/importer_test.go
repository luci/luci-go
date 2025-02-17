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

package model

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/configspb"
	customerrors "go.chromium.org/luci/auth_service/impl/errors"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/importscfg"
	"go.chromium.org/luci/auth_service/testsupport"
)

func TestLoadGroupFile(t *testing.T) {
	t.Parallel()
	testDomain := "example.com"

	ftt.Run("Testing LoadGroupFile()", t, func(t *ftt.Test) {
		t.Run("OK", func(t *ftt.Test) {
			body := strings.Join([]string{"", "b", "a", "a", ""}, "\n")
			aIdent, _ := identity.MakeIdentity(fmt.Sprintf("user:a@%s", testDomain))
			bIdent, _ := identity.MakeIdentity(fmt.Sprintf("user:b@%s", testDomain))
			actual, err := loadGroupFile(body, testDomain)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match([]identity.Identity{
				aIdent,
				bIdent,
			}))
		})
		t.Run("Handles temporary accounts", func(t *ftt.Test) {
			cIdent, _ := identity.MakeIdentity("user:c@test")
			actual, err := loadGroupFile("c%test@gtempaccount.com", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match([]identity.Identity{
				cIdent,
			}))
		})
		t.Run("bad id", func(t *ftt.Test) {
			body := "bad id"
			_, err := loadGroupFile(body, testDomain)
			assert.Loosely(t, err, should.ErrLike(`auth: bad value "bad id@example.com" for identity kind "user"`))
		})
	})
}

func TestExtractTarArchive(t *testing.T) {
	t.Parallel()
	ftt.Run("valid tarball with skippable files", t, func(t *ftt.Test) {
		expected := map[string][]byte{
			"at_root":             []byte("a\nb"),
			"ldap/ bad name":      []byte("a\nb"),
			"ldap/group-a":        []byte("a\nb"),
			"ldap/group-b":        []byte("a\nb"),
			"ldap/group-c":        []byte("a\nb"),
			"ldap/deeper/group-a": []byte("a\nb"),
			"not-ldap/group-a":    []byte("a\nb"),
		}
		bundle := testsupport.BuildTargz(expected)
		entries, err := extractTarArchive(bytes.NewReader(bundle))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, entries, should.Match(expected))
	})
}

func TestLoadTarball(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	ftt.Run("testing loadTarball", t, func(t *ftt.Test) {
		t.Run("invalid tarball bad identity", func(t *ftt.Test) {
			bundle := testsupport.BuildTargz(map[string][]byte{
				"at_root":      []byte("a\nb"),
				"ldap/group-a": []byte("a\n!!!!!!"),
			})
			_, err := loadTarball(ctx, bytes.NewReader(bundle), "example.com", []string{"ldap"}, []string{"ldap/group-a", "ldap/group-b"})
			assert.Loosely(t, err, should.ErrLike(`auth: bad value "!!!!!!@example.com" for identity kind "user"`))
		})
		t.Run("valid tarball with skippable files", func(t *ftt.Test) {
			bundle := testsupport.BuildTargz(map[string][]byte{
				"at_root":                   []byte("a\nb"),
				"ldap/ bad name":            []byte("a\nb"),
				"ldap/group-a":              []byte("a\nb"),
				"ldap/group-b":              []byte("a\nb"),
				"ldap/group-c":              []byte("a\nb"),
				"ldap/deeper/group-a":       []byte("a\nb"),
				"not-ldap/group-a":          []byte("a\nb"),
				"InvalidSystem/group-valid": []byte("a\nb"),
			})
			m, err := loadTarball(ctx, bytes.NewReader(bundle), "example.com", []string{"ldap", "InvalidSystem"}, []string{"ldap/group-a", "ldap/group-b", "InvalidSystem/group-valid"})
			assert.Loosely(t, err, should.BeNil)
			aIdent, _ := identity.MakeIdentity("user:a@example.com")
			bIdent, _ := identity.MakeIdentity("user:b@example.com")
			assert.Loosely(t, m, should.Match(map[string]GroupBundle{
				"ldap": {
					"ldap/group-a": {
						aIdent,
						bIdent,
					},
					"ldap/group-b": {
						aIdent,
						bIdent,
					},
				},
			}))
		})
	})
}

func TestIngestTarball(t *testing.T) {
	t.Parallel()

	ftt.Run("testing IngestTarball", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		testConfig := &configspb.GroupImporterConfig{
			TarballUpload: []*configspb.GroupImporterConfig_TarballUploadEntry{
				{
					Name:               "test_groups.tar.gz",
					AuthorizedUploader: []string{"test-push-cron@system.example.com"},
					Systems:            []string{"tst"},
				},
				{
					Name:               "example_groups.tar.gz",
					AuthorizedUploader: []string{"another-push-cron@system.example.com"},
					Systems:            []string{"examp"},
				},
			},
		}

		bundle := testsupport.BuildTargz(map[string][]byte{
			"at_root":            []byte("a\nb"),
			"tst/ bad name":      []byte("a\nb"),
			"tst/group-a":        []byte("a@example.com\nb@example.test.com"),
			"tst/group-b":        []byte("a@example.com"),
			"tst/group-c":        []byte("a@example.com\nc@test-example.com"),
			"tst/deeper/group-a": []byte("a\nb"),
			"not-tst/group-a":    []byte("a\nb"),
		})

		t.Run("not configured", func(t *ftt.Test) {
			_, _, err := IngestTarball(ctx, "test_groups.tar.gz", nil)
			assert.Loosely(t, err, should.ErrLike(ErrImporterNotConfigured))
		})

		t.Run("with importer configuration set", func(t *ftt.Test) {
			// Set up imports config for the test cases below.
			assert.Loosely(t, importscfg.SetConfig(ctx, testConfig), should.BeNil)

			t.Run("invalid tarball name", func(t *ftt.Test) {
				_, _, err := IngestTarball(ctx, "", nil)
				assert.Loosely(t, err, should.ErrLike(ErrUnauthorizedUploader))
			})
			t.Run("unknown tarball", func(t *ftt.Test) {
				_, _, err := IngestTarball(ctx, "zzz", nil)
				assert.Loosely(t, err, should.ErrLike(ErrUnauthorizedUploader))
			})
			t.Run("unauthorized", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				_, _, err := IngestTarball(ctx, "test_groups.tar.gz", bytes.NewReader(bundle))
				assert.Loosely(t, err, should.ErrLike(ErrUnauthorizedUploader))
				assert.Loosely(t, err, should.ErrLike(`"someone@example.com"`))
			})
			t.Run("invalid tarball data", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:test-push-cron@system.example.com",
				})
				_, _, err := IngestTarball(ctx, "test_groups.tar.gz", bytes.NewReader(nil))
				assert.Loosely(t, err, should.ErrLike(ErrInvalidTarball))
				assert.Loosely(t, err, should.ErrLike("EOF"))
			})

			t.Run("actually imports groups", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:test-push-cron@system.example.com",
				})

				g := makeAuthGroup(ctx, "administrators")
				g.AuthVersionedEntityMixin = testAuthVersionedEntityMixin()
				assert.Loosely(t, datastore.Put(ctx, g), should.BeNil)

				updatedGroups, revision, err := IngestTarball(ctx, "test_groups.tar.gz", bytes.NewReader(bundle))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, revision, should.Equal(1))
				assert.Loosely(t, updatedGroups, should.Match([]string{
					"tst/group-a",
					"tst/group-b",
					"tst/group-c",
				}))
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
			})
		})
	})
}

func TestImportBundles(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing importBundles", t, func(t *ftt.Test) {
		// Set up craetor and modifier identities.
		creator := "user:test-creator@example.com"
		modifier := "user:test-modifier@example.com"
		callerIdent := identity.Identity(modifier)

		ctx := memory.Use(context.Background())
		tc := testclock.New(testModifiedTS)
		ctx = clock.Set(ctx, tc)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			tc.Add(d)
		})
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		aIdent, _ := identity.MakeIdentity("user:a@example.com")
		bundles := map[string]GroupBundle{
			"ext": {
				"ext/group-a": {aIdent},
			},
			"sys": {
				"sys/group-a": {aIdent},
				"sys/group-b": {aIdent},
				"sys/group-c": {aIdent},
			},
		}
		baseSlice := []string{"ext/group-a", "sys/group-a", "sys/group-b", "sys/group-c"}
		baseGroupBundles := stringset.NewFromSlice(baseSlice...).ToSortedSlice()

		t.Run("aborts without AdminGroup", func(t *ftt.Test) {
			updatedGroups, revision, err := importBundles(ctx, bundles, callerIdent, nil)
			assert.Loosely(t, err, should.ErrLike(customerrors.ErrInvalidReference))
			assert.Loosely(t, err, should.ErrLike("aborting groups import"))
			assert.Loosely(t, err, should.ErrLike(AdminGroup))
			assert.Loosely(t, updatedGroups, should.BeEmpty)
			assert.Loosely(t, revision, should.BeZero)
		})

		t.Run("with AdminGroup", func(t *ftt.Test) {
			adminGroup := emptyAuthGroup(ctx, AdminGroup)
			assert.Loosely(t, datastore.Put(ctx, adminGroup), should.BeNil)

			t.Run("Creating groups", func(t *ftt.Test) {
				// Define the expected groups that should be created.
				// Note: for created groups, the last modifying action *is* the creation.
				sGroupA := testExternalAuthGroup(ctx, "sys/group-a", modifier, []string{string(aIdent)}, testModifiedTS)
				sGroupB := testExternalAuthGroup(ctx, "sys/group-b", modifier, []string{string(aIdent)}, testModifiedTS)
				sGroupC := testExternalAuthGroup(ctx, "sys/group-c", modifier, []string{string(aIdent)}, testModifiedTS)
				eGroupA := testExternalAuthGroup(ctx, "ext/group-a", modifier, []string{string(aIdent)}, testModifiedTS)

				updatedGroups, revision, err := importBundles(ctx, bundles, callerIdent, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(baseGroupBundles))
				assert.Loosely(t, revision, should.Equal(1))
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

				// Check each group was created as expected.
				groupA, err := GetAuthGroup(ctx, sGroupA.ID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, groupA, should.Match(sGroupA))
				groupB, err := GetAuthGroup(ctx, sGroupB.ID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, groupB, should.Match(sGroupB))
				groupC, err := GetAuthGroup(ctx, sGroupC.ID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, groupC, should.Match(sGroupC))
				groupAe, err := GetAuthGroup(ctx, eGroupA.ID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, groupAe, should.Match(eGroupA))
			})

			t.Run("Updating Groups", func(t *ftt.Test) {
				// Set up datastore with the initial state of the external group.
				g := testExternalAuthGroup(ctx, "sys/group-a", creator, []string{"user:b@example.com", "user:c@example.com"}, testCreatedTS)
				assert.Loosely(t, datastore.Put(ctx, g, testAuthReplicationState(ctx, 1)), should.BeNil)

				updatedGroups, revision, err := importBundles(ctx, bundles, callerIdent, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(baseGroupBundles))
				assert.Loosely(t, revision, should.Equal(2))

				// Check the group was imported as expected.
				sGroupA := testExternalAuthGroup(ctx, "sys/group-a", creator, []string{string(aIdent)}, testCreatedTS)
				sGroupA.AuthVersionedEntityMixin = AuthVersionedEntityMixin{
					ModifiedBy:    modifier,
					ModifiedTS:    testModifiedTS,
					AuthDBRev:     2,
					AuthDBPrevRev: 1,
				}
				actual, err := GetAuthGroup(ctx, sGroupA.ID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual, should.Match(sGroupA))
			})

			t.Run("Deleting Groups", func(t *ftt.Test) {
				// Set up datastore with the external group.
				g := testExternalAuthGroup(ctx, "sys/group-d", creator, []string{"user:a@example.com"}, testCreatedTS)
				assert.Loosely(t, datastore.Put(ctx, g, testAuthReplicationState(ctx, 1)), should.BeNil)

				updatedGroups, revision, err := importBundles(ctx, bundles, callerIdent, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(append(baseGroupBundles, "sys/group-d")))
				assert.Loosely(t, revision, should.Equal(2))

				_, err = GetAuthGroup(ctx, g.ID)
				assert.Loosely(t, err, should.ErrLike(datastore.ErrNoSuchEntity))
			})

			t.Run("Clearing Groups", func(t *ftt.Test) {
				// Set up datastore with the external group and a group that references it as a subgroup.
				g := testExternalAuthGroup(ctx, "sys/group-e", creator, []string{"user:a@example.com"}, testCreatedTS)
				superGroup := testAuthGroup(ctx, "test-group")
				superGroup.Nested = []string{"sys/group-e"}
				assert.Loosely(t, datastore.Put(ctx, g, superGroup, testAuthReplicationState(ctx, 1)), should.BeNil)

				updatedGroups, revision, err := importBundles(ctx, bundles, callerIdent, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(append(baseGroupBundles, "sys/group-e")))
				assert.Loosely(t, revision, should.Equal(2))

				actual, err := GetAuthGroup(ctx, g.ID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual.Members, should.BeEmpty)
			})

			t.Run("Large groups", func(t *ftt.Test) {
				bundle, groupsBundled := makeGroupBundle("test", 400)
				updatedGroups, rev, err := importBundles(ctx, bundle, callerIdent, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(groupsBundled))
				assert.Loosely(t, rev, should.Equal(2))
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				groups, err := GetAllAuthGroups(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, groups, should.HaveLength(401))
			})

			t.Run("Revision changes in between transactions", func(t *ftt.Test) {
				bundle, groupsBundled := makeGroupBundle("test", 500)
				updatedGroups, rev, err := importBundles(ctx, bundle, callerIdent, func() {
					assert.Loosely(t, datastore.Put(ctx, testAuthReplicationState(ctx, 3)), should.BeNil)
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(groupsBundled))
				assert.Loosely(t, rev, should.Equal(5))
			})

			t.Run("Large put Large delete", func(t *ftt.Test) {
				bundle, groupsBundledTest := makeGroupBundle("test", 1000)
				updatedGroups, rev, err := importBundles(ctx, bundle, callerIdent, func() {})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(groupsBundledTest))
				assert.Loosely(t, rev, should.Equal(5))
				bundle, groupsBundled := makeGroupBundle("example", 400)
				updatedGroups, rev, err = importBundles(ctx, bundle, callerIdent, func() {})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(groupsBundled))
				assert.Loosely(t, rev, should.Equal(7))

				bundle, groupsBundled = makeGroupBundle("tst", 500)
				bundle["test"] = GroupBundle{}
				groupsBundled = append(groupsBundled, groupsBundledTest...)
				updatedGroups, rev, err = importBundles(ctx, bundle, callerIdent, func() {})
				assert.Loosely(t, err, should.BeNil)
				sort.Strings(groupsBundled)
				assert.Loosely(t, updatedGroups, should.Match(groupsBundled))
				assert.Loosely(t, rev, should.Equal(15))
			})

			// The max number of create, update, or delete in one transaction
			// is 500. For every entity we modify we attach a history entity
			// in the code for importing we limit this to 200 so we can be
			// under the limit by only touching 400 entities.
			t.Run("150 put 150 del", func(t *ftt.Test) {
				bundle, groupsBundledTest := makeGroupBundle("test", 150)
				updatedGroups, rev, err := importBundles(ctx, bundle, callerIdent, func() {})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(groupsBundledTest))
				assert.Loosely(t, rev, should.Equal(1))
				bundle, groupsBundled := makeGroupBundle("tst", 150)
				bundle["test"] = GroupBundle{}
				groupsBundled = append(groupsBundled, groupsBundledTest...)
				sort.Strings(groupsBundled)
				updatedGroups, rev, err = importBundles(ctx, bundle, callerIdent, func() {})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, updatedGroups, should.Match(groupsBundled))
				assert.Loosely(t, rev, should.Equal(3))
			})
		})
	})
}

func makeGroupBundle(system string, size int) (map[string]GroupBundle, []string) {
	bundle := map[string]GroupBundle{}
	groupsBundled := stringset.New(0)
	bundle[system] = make(GroupBundle, size)
	for i := 0; i < size; i++ {
		group := fmt.Sprintf("%s/group-%d", system, i)
		bundle[system][group] = []identity.Identity{"user:a@example.com"}
		groupsBundled.Add(group)
	}
	return bundle, groupsBundled.ToSortedSlice()
}

func testExternalAuthGroup(ctx context.Context, name, createdBy string, members []string, createdTS time.Time) *AuthGroup {
	return &AuthGroup{
		Kind:   "AuthGroup",
		ID:     name,
		Parent: RootKey(ctx),
		AuthVersionedEntityMixin: AuthVersionedEntityMixin{
			ModifiedBy:    createdBy,
			ModifiedTS:    createdTS,
			AuthDBRev:     1,
			AuthDBPrevRev: 0,
		},
		Members:   members,
		Owners:    AdminGroup,
		CreatedTS: createdTS,
		CreatedBy: createdBy,
	}
}
