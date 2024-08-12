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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/importscfg"
	"go.chromium.org/luci/auth_service/testsupport"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func testGroupImporterConfig() *GroupImporterConfig {
	return &GroupImporterConfig{
		Kind: "GroupImporterConfig",
		ID:   "config",
		ConfigProto: `
			# Schema for this file:
			# https://config.luci.app/schemas/services/chrome-infra-auth:imports.cfg
			# See GroupImporterConfig message.

			# Groups pushed by //depot/google3/googleclient/chrome/infra/groups_push_cron/
			#
			# To add new groups, see README.md there.

			tarball_upload {
			name: "test_groups.tar.gz"
			authorized_uploader: "test-push-cron@system.example.com"
			systems: "tst"
			}

			tarball_upload {
			name: "example_groups.tar.gz"
			authorized_uploader: "another-push-cron@system.example.com"
			systems: "examp"
			}
		`,
		ConfigRevision: []byte("some-config-revision"),
		ModifiedBy:     "some-user@example.com",
		ModifiedTS:     testModifiedTS,
	}
}
func TestGroupImporterConfigModel(t *testing.T) {
	t.Parallel()

	Convey("testing GroupImporterConfig entity", t, func() {
		ctx := memory.Use(context.Background())

		Convey("testing GetGroupImporterConfig", func() {
			_, err := GetGroupImporterConfig(ctx)
			So(err, ShouldEqual, datastore.ErrNoSuchEntity)

			groupCfg := testGroupImporterConfig()
			So(datastore.Put(ctx, groupCfg), ShouldBeNil)

			actual, err := GetGroupImporterConfig(ctx)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, groupCfg)
		})

		Convey("updateGroupImporterConfig works", func() {
			ctx = testsupport.SetTestContextSigner(ctx, "test-app-id", "test-app-id@example.com")

			Convey("creates if it doesn't exist", func() {
				ctx = clock.Set(ctx, testclock.New(testCreatedTS))

				_, err := GetGroupImporterConfig(ctx)
				So(err, ShouldEqual, datastore.ErrNoSuchEntity)

				testConfig := &configspb.GroupImporterConfig{
					TarballUpload: []*configspb.GroupImporterConfig_TarballUploadEntry{
						{
							Name:               "tarball-upload-test",
							AuthorizedUploader: []string{"test-uploader@example.com"},
							Systems:            []string{"tarballtest"},
						},
					},
				}
				testMeta := &config.Meta{
					ConfigSet:   config.Set("service/chrome-infra-auth-test"),
					Path:        "imports.cfg",
					ContentHash: "a1b2c3",
					Revision:    "10001a",
					ViewURL:     "config-url.example.com/imports/10001a",
				}
				So(updateGroupImporterConfig(ctx, testConfig, testMeta, false), ShouldBeNil)

				stored, err := GetGroupImporterConfig(ctx)
				So(err, ShouldBeNil)
				actual, err := stored.ToProto()
				So(err, ShouldBeNil)
				So(actual, ShouldResembleProto, testConfig)

				Convey("updating works", func() {
					ctx = clock.Set(ctx, testclock.New(testModifiedTS))

					Convey("no-op for same revision", func() {
						So(updateGroupImporterConfig(ctx, testConfig, testMeta, false), ShouldBeNil)

						stored, err := GetGroupImporterConfig(ctx)
						So(err, ShouldBeNil)
						So(stored.ModifiedTS, ShouldEqual, testCreatedTS)
						actual, err := stored.ToProto()
						So(err, ShouldBeNil)
						So(actual, ShouldResembleProto, testConfig)
					})

					Convey("updated for different revision", func() {
						otherConfig := &configspb.GroupImporterConfig{
							TarballUpload: []*configspb.GroupImporterConfig_TarballUploadEntry{
								{
									Name:               "other-tarball-upload-test",
									AuthorizedUploader: []string{"test-uploader@example.com"},
									Systems:            []string{"tarballtest"},
								},
							},
						}
						otherMeta := &config.Meta{
							ConfigSet:   config.Set("service/chrome-infra-auth-test"),
							Path:        "imports.cfg",
							ContentHash: "a1b2c3",
							Revision:    "10001b",
							ViewURL:     "config-url.example.com/imports/10001b",
						}
						So(updateGroupImporterConfig(ctx, otherConfig, otherMeta, false), ShouldBeNil)

						stored, err := GetGroupImporterConfig(ctx)
						So(err, ShouldBeNil)
						So(stored.ModifiedTS, ShouldEqual, testModifiedTS)
						actual, err := stored.ToProto()
						So(err, ShouldBeNil)
						So(actual, ShouldResembleProto, otherConfig)
					})

					Convey("no-op when in dry run", func() {
						otherConfig := &configspb.GroupImporterConfig{
							TarballUpload: []*configspb.GroupImporterConfig_TarballUploadEntry{
								{
									Name:               "other-tarball-upload-test",
									AuthorizedUploader: []string{"test-uploader@example.com"},
									Systems:            []string{"tarballtest"},
								},
							},
						}
						otherMeta := &config.Meta{
							ConfigSet:   config.Set("service/chrome-infra-auth-test"),
							Path:        "imports.cfg",
							ContentHash: "a1b2c3",
							Revision:    "10001b",
							ViewURL:     "config-url.example.com/imports/10001b",
						}
						So(updateGroupImporterConfig(ctx, otherConfig, otherMeta, true), ShouldBeNil)

						stored, err := GetGroupImporterConfig(ctx)
						So(err, ShouldBeNil)
						So(stored.ModifiedTS, ShouldEqual, testCreatedTS)
						actual, err := stored.ToProto()
						So(err, ShouldBeNil)
						So(actual, ShouldResembleProto, testConfig)
					})
				})
			})
		})
	})
}

func TestLoadGroupFile(t *testing.T) {
	t.Parallel()
	testDomain := "example.com"

	Convey("Testing LoadGroupFile()", t, func() {
		ctx := context.Background()
		Convey("OK", func() {
			body := strings.Join([]string{"", "b", "a", "a", ""}, "\n")
			aIdent, _ := identity.MakeIdentity(fmt.Sprintf("user:a@%s", testDomain))
			bIdent, _ := identity.MakeIdentity(fmt.Sprintf("user:b@%s", testDomain))
			actual, err := loadGroupFile(ctx, body, testDomain)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, []identity.Identity{
				aIdent,
				bIdent,
			})
		})
		Convey("Handles temporary accounts", func() {
			cIdent, _ := identity.MakeIdentity("user:c@test")
			actual, err := loadGroupFile(ctx, "c%test@gtempaccount.com", "")
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, []identity.Identity{
				cIdent,
			})
		})
		Convey("bad id", func() {
			body := "bad id"
			_, err := loadGroupFile(ctx, body, testDomain)
			So(err, ShouldErrLike, `auth: bad value "bad id@example.com" for identity kind "user"`)
		})
	})
}

func TestExtractTarArchive(t *testing.T) {
	t.Parallel()
	Convey("valid tarball with skippable files", t, func() {
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
		So(err, ShouldBeNil)
		So(entries, ShouldResemble, expected)
	})
}

func TestLoadTarball(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	Convey("testing loadTarball", t, func() {
		Convey("invalid tarball bad identity", func() {
			bundle := testsupport.BuildTargz(map[string][]byte{
				"at_root":      []byte("a\nb"),
				"ldap/group-a": []byte("a\n!!!!!!"),
			})
			_, err := loadTarball(ctx, bytes.NewReader(bundle), "example.com", []string{"ldap"}, []string{"ldap/group-a", "ldap/group-b"})
			So(err, ShouldErrLike, `auth: bad value "!!!!!!@example.com" for identity kind "user"`)
		})
		Convey("valid tarball with skippable files", func() {
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
			So(err, ShouldBeNil)
			aIdent, _ := identity.MakeIdentity("user:a@example.com")
			bIdent, _ := identity.MakeIdentity("user:b@example.com")
			So(m, ShouldResemble, map[string]GroupBundle{
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
			})
		})
	})
}

func TestIngestTarball(t *testing.T) {
	t.Parallel()

	Convey("testing IngestTarball", t, func() {
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

		Convey("not configured", func() {
			_, _, err := IngestTarball(ctx, "test_groups.tar.gz", nil)
			So(err, ShouldErrLike, "not configured")
		})

		Convey("with importer configuration set", func() {
			// Set up imports config for the test cases below.
			So(importscfg.SetConfig(ctx, testConfig), ShouldBeNil)

			Convey("invalid tarball name", func() {
				_, _, err := IngestTarball(ctx, "", nil)
				So(err, ShouldErrLike, "empty tarball name")
			})
			Convey("unknown tarball", func() {
				_, _, err := IngestTarball(ctx, "zzz", nil)
				So(err, ShouldErrLike, "entry not found")
			})
			Convey("unauthorized", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				_, _, err := IngestTarball(ctx, "test_groups.tar.gz", bytes.NewReader(bundle))
				So(err, ShouldErrLike, `"someone@example.com" is not an authorized uploader`)
			})

			Convey("actually imports groups", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:test-push-cron@system.example.com",
				})

				g := makeAuthGroup(ctx, "administrators")
				g.AuthVersionedEntityMixin = testAuthVersionedEntityMixin()
				_, err := CreateAuthGroup(ctx, g, false, "Imported from group bundles", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				updatedGroups, revision, err := IngestTarball(ctx, "test_groups.tar.gz", bytes.NewReader(bundle))
				So(err, ShouldBeNil)
				So(revision, ShouldEqual, 2)
				So(updatedGroups, ShouldResemble, []string{
					"tst/group-a",
					"tst/group-b",
					"tst/group-c",
				})
			})
		})
	})
}

func TestImportBundles(t *testing.T) {
	t.Parallel()

	Convey("Testing importBundles", t, func() {
		userIdent := identity.Identity("user:test-modifier@example.com")
		ctx := memory.Use(context.Background())
		tc := testclock.New(testModifiedTS)
		ctx = clock.Set(ctx, tc)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			tc.Add(d)
		})
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		adminGroup := emptyAuthGroup(ctx, AdminGroup)
		So(datastore.Put(ctx, adminGroup), ShouldBeNil)

		aIdent, _ := identity.MakeIdentity("user:a@example.com")

		sGroupA := testExternalAuthGroup(ctx, "sys/group-a", []string{string(aIdent)})

		sGroupB := testExternalAuthGroup(ctx, "sys/group-b", []string{string(aIdent)})
		sGroupC := testExternalAuthGroup(ctx, "sys/group-c", []string{string(aIdent)})

		eGroupA := testExternalAuthGroup(ctx, "ext/group-a", []string{string(aIdent)})

		bundles := map[string]GroupBundle{
			"ext": {
				eGroupA.ID: {
					aIdent,
				},
			},
			"sys": {
				sGroupA.ID: {
					aIdent,
				},
				sGroupB.ID: {
					aIdent,
				},
				sGroupC.ID: {
					aIdent,
				},
			},
		}

		baseSlice := []string{eGroupA.ID, sGroupA.ID, sGroupB.ID, sGroupC.ID}
		baseGroupBundles := stringset.NewFromSlice(baseSlice...).ToSortedSlice()

		Convey("Creating groups", func() {
			updatedGroups, revision, err := importBundles(ctx, bundles, userIdent, nil)
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, baseGroupBundles)
			So(revision, ShouldEqual, 1)
			groupA, err := GetAuthGroup(ctx, sGroupA.ID)
			So(err, ShouldBeNil)
			So(groupA.Members, ShouldResemble, sGroupA.Members)

			groupB, err := GetAuthGroup(ctx, sGroupB.ID)
			So(err, ShouldBeNil)
			So(groupB.Members, ShouldResemble, sGroupB.Members)

			groupC, err := GetAuthGroup(ctx, sGroupC.ID)
			So(err, ShouldBeNil)
			So(groupC.Members, ShouldResemble, sGroupC.Members)

			groupAe, err := GetAuthGroup(ctx, eGroupA.ID)
			So(err, ShouldBeNil)
			So(groupAe.Members, ShouldResemble, eGroupA.Members)

			So(taskScheduler.Tasks(), ShouldHaveLength, 2)
		})

		Convey("Updating Groups", func() {
			g := testExternalAuthGroup(ctx, "sys/group-a", []string{"user:b@example.com", "user:c@example.com"})
			_, err := CreateAuthGroup(ctx, g, true, "Imported from group bundles", false)
			So(err, ShouldBeNil)
			group, err := GetAuthGroup(ctx, sGroupA.ID)
			So(err, ShouldBeNil)
			So(group.Members, ShouldResemble, g.Members)
			updatedGroups, revision, err := importBundles(ctx, bundles, userIdent, nil)
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, baseGroupBundles)
			So(revision, ShouldEqual, 2)
			group, err = GetAuthGroup(ctx, sGroupA.ID)
			So(err, ShouldBeNil)
			So(group.Members, ShouldResemble, sGroupA.Members)
		})

		Convey("Deleting Groups", func() {
			g := testExternalAuthGroup(ctx, "sys/group-d", []string{"user:a@example.com"})
			_, err := CreateAuthGroup(ctx, g, true, "Imported from group bundles", false)
			So(err, ShouldBeNil)

			grDS, err := GetAuthGroup(ctx, g.ID)
			So(err, ShouldBeNil)
			So(grDS.Members, ShouldResemble, g.Members)

			updatedGroups, revision, err := importBundles(ctx, bundles, userIdent, nil)
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, append(baseGroupBundles, "sys/group-d"))
			So(revision, ShouldEqual, 2)

			_, err = GetAuthGroup(ctx, g.ID)
			So(err, ShouldErrLike, datastore.ErrNoSuchEntity)
		})

		Convey("Large groups", func() {
			bundle, groupsBundled := makeGroupBundle("test", 400)
			updatedGroups, rev, err := importBundles(ctx, bundle, userIdent, nil)
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, groupsBundled)
			So(rev, ShouldEqual, 2)
			So(taskScheduler.Tasks(), ShouldHaveLength, 4)
			groups, err := GetAllAuthGroups(ctx)
			So(err, ShouldBeNil)
			So(groups, ShouldHaveLength, 401)
		})

		Convey("Revision changes in between transactions", func() {
			bundle, groupsBundled := makeGroupBundle("test", 500)
			updatedGroups, rev, err := importBundles(ctx, bundle, userIdent, func() {
				So(datastore.Put(ctx, testAuthReplicationState(ctx, 3)), ShouldBeNil)
			})
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, groupsBundled)
			So(rev, ShouldEqual, 5)
		})

		Convey("Large put Large delete", func() {
			bundle, groupsBundledTest := makeGroupBundle("test", 1000)
			updatedGroups, rev, err := importBundles(ctx, bundle, userIdent, func() {})
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, groupsBundledTest)
			So(rev, ShouldEqual, 5)
			bundle, groupsBundled := makeGroupBundle("example", 400)
			updatedGroups, rev, err = importBundles(ctx, bundle, userIdent, func() {})
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, groupsBundled)
			So(rev, ShouldEqual, 7)

			bundle, groupsBundled = makeGroupBundle("tst", 500)
			bundle["test"] = GroupBundle{}
			groupsBundled = append(groupsBundled, groupsBundledTest...)
			updatedGroups, rev, err = importBundles(ctx, bundle, userIdent, func() {})
			So(err, ShouldBeNil)
			sort.Strings(groupsBundled)
			So(updatedGroups, ShouldResemble, groupsBundled)
			So(rev, ShouldEqual, 15)
		})

		// The max number of create, update, or delete in one transaction
		// is 500. For every entity we modify we attach a history entity
		// in the code for importing we limit this to 200 so we can be
		// under the limit by only touching 400 entities.
		Convey("150 put 150 del", func() {
			bundle, groupsBundledTest := makeGroupBundle("test", 150)
			updatedGroups, rev, err := importBundles(ctx, bundle, userIdent, func() {})
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, groupsBundledTest)
			So(rev, ShouldEqual, 1)
			bundle, groupsBundled := makeGroupBundle("tst", 150)
			bundle["test"] = GroupBundle{}
			groupsBundled = append(groupsBundled, groupsBundledTest...)
			sort.Strings(groupsBundled)
			updatedGroups, rev, err = importBundles(ctx, bundle, userIdent, func() {})
			So(err, ShouldBeNil)
			So(updatedGroups, ShouldResemble, groupsBundled)
			So(rev, ShouldEqual, 3)
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
