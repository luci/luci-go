// Copyright 2024 The LUCI Authors.
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

package model

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	gaemem "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/testsupport"
)

func testImportedConfigRevisions(ctx context.Context, revs map[string]*configRevisionInfo) *ImportedConfigRevisions {
	e := &ImportedConfigRevisions{
		Kind:   "_ImportedConfigRevisions",
		ID:     "self",
		Parent: RootKey(ctx),
	}
	e.Revisions, _ = json.Marshal(revs)
	return e

}
func TestImportedConfigRevisions(t *testing.T) {
	t.Parallel()

	ftt.Run("testing ImportedConfigRevisions entity", t, func(t *ftt.Test) {
		ctx := gaemem.Use(context.Background())

		t.Run("getImportedConfigRevisions works", func(t *ftt.Test) {
			_, err := getImportedConfigRevisions(ctx)
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

			testRevs := map[string]*configRevisionInfo{
				"oauth.cfg": {
					Revision: "10001a",
					ViewURL:  "https://test.config.example.com/oauth/10001a",
				},
				"security.cfg": {
					Revision: "10001a",
					ViewURL:  "https://test.config.example.com/security/10001a",
				},
			}
			testEntity := testImportedConfigRevisions(ctx, testRevs)
			assert.Loosely(t, datastore.Put(ctx, testEntity), should.BeNil)

			actual, err := getImportedConfigRevisions(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(testEntity))
		})

		t.Run("updateImportedConfigRevisions works", func(t *ftt.Test) {
			testRevs := map[string]*configRevisionInfo{
				"settings.cfg": {
					Revision: "10001a",
					ViewURL:  "https://test.config.example.com/settings/10001a",
				},
				"oauth.cfg": {
					Revision: "10001a",
					ViewURL:  "https://test.config.example.com/oauth/10001a",
				},
				"security.cfg": {
					Revision: "10001a",
					ViewURL:  "https://test.config.example.com/security/10001a",
				},
			}
			affectedRevs := map[string]*configRevisionInfo{
				"oauth.cfg": {
					Revision: "10001a",
					ViewURL:  "https://test.config.example.com/oauth/10001a",
				},
				"security.cfg": {
					Revision: "10001a",
					ViewURL:  "https://test.config.example.com/security/10001a",
				},
			}

			t.Run("creates if it doesn't exist", func(t *ftt.Test) {
				assert.Loosely(t, updateImportedConfigRevisions(ctx, testRevs, false), should.BeNil)

				actual, err := getImportedConfigRevisions(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual, should.Resemble(testImportedConfigRevisions(ctx, affectedRevs)))

				t.Run("updating existing works", func(t *ftt.Test) {
					updatedRevs := map[string]*configRevisionInfo{
						"oauth.cfg": {
							Revision: "10001b",
							ViewURL:  "https://test.config.example.com/oauth/10001b",
						},
					}
					assert.Loosely(t, updateImportedConfigRevisions(ctx, updatedRevs, false), should.BeNil)

					expectedRevs := map[string]*configRevisionInfo{
						"oauth.cfg": {
							Revision: "10001b",
							ViewURL:  "https://test.config.example.com/oauth/10001b",
						},
						"security.cfg": {
							Revision: "10001a",
							ViewURL:  "https://test.config.example.com/security/10001a",
						},
					}
					actual, err := getImportedConfigRevisions(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, actual, should.Resemble(testImportedConfigRevisions(ctx, expectedRevs)))
				})

				t.Run("revision info for excluded paths is maintained", func(t *ftt.Test) {
					allowlistRev := map[string]*configRevisionInfo{
						"ip_allowlist.cfg": {
							Revision: "10001a",
							ViewURL:  "https://test.config.example.com/allowlist/10001a",
						},
					}
					assert.Loosely(t, updateImportedConfigRevisions(ctx, allowlistRev, false), should.BeNil)

					expectedRevs := map[string]*configRevisionInfo{
						"ip_allowlist.cfg": {
							Revision: "10001a",
							ViewURL:  "https://test.config.example.com/allowlist/10001a",
						},
						"oauth.cfg": {
							Revision: "10001a",
							ViewURL:  "https://test.config.example.com/oauth/10001a",
						},
						"security.cfg": {
							Revision: "10001a",
							ViewURL:  "https://test.config.example.com/security/10001a",
						},
					}
					actual, err := getImportedConfigRevisions(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, actual, should.Resemble(testImportedConfigRevisions(ctx, expectedRevs)))
				})

				t.Run("config that doesn't affect the AuthDB is ignored", func(t *ftt.Test) {
					trivialRev := map[string]*configRevisionInfo{
						"imports.cfg": {
							Revision: "10001a",
							ViewURL:  "https://test.config.example.com/imports/10001a",
						},
					}
					assert.Loosely(t, updateImportedConfigRevisions(ctx, trivialRev, false), should.BeNil)

					actual, err := getImportedConfigRevisions(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, actual, should.Resemble(testImportedConfigRevisions(ctx, affectedRevs)))
				})
			})
		})
	})
}

func TestGetStoredRealmsCfgRevs(t *testing.T) {
	t.Parallel()

	ftt.Run("getting stored project realms config rev works", t, func(t *ftt.Test) {
		ctx := gaemem.Use(context.Background())

		t.Run("successful if none exist", func(t *ftt.Test) {
			configRevs, err := getStoredRealmsCfgRevs(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, configRevs, should.BeEmpty)
		})

		t.Run("returns the stored configs from metadata", func(t *ftt.Test) {
			// Set up metadata for a project's realms config in datastore.
			assert.Loosely(t, datastore.Put(ctx, &AuthProjectRealmsMeta{
				Kind:         "AuthProjectRealmsMeta",
				ID:           "meta",
				Parent:       projectRealmsKey(ctx, "test"),
				PermsRev:     "123",
				ConfigRev:    "1234",
				ConfigDigest: "test-digest",
				ModifiedTS:   testModifiedTS,
			}), should.BeNil)

			configRevs, err := getStoredRealmsCfgRevs(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, configRevs, should.Resemble([]*RealmsCfgRev{
				{
					ProjectID:    "test",
					ConfigRev:    "1234",
					PermsRev:     "123",
					ConfigDigest: "test-digest",
				},
			}))
		})
	})
}

func TestUpdateRealms(t *testing.T) {
	t.Parallel()

	simpleProjectRealm := func(ctx context.Context, projectName string, expectedRealmsBody []byte, authDBRev int) *AuthProjectRealms {
		return &AuthProjectRealms{
			AuthVersionedEntityMixin: AuthVersionedEntityMixin{
				ModifiedTS:    testCreatedTS,
				ModifiedBy:    "user:test-app-id@example.com",
				AuthDBRev:     int64(authDBRev),
				AuthDBPrevRev: int64(authDBRev - 1),
			},
			Kind:      "AuthProjectRealms",
			ID:        fmt.Sprintf("test-project-%s", projectName),
			Parent:    RootKey(ctx),
			Realms:    expectedRealmsBody,
			ConfigRev: testsupport.TestRevision,
			PermsRev:  "permissions.cfg:123",
		}
	}

	simpleProjectRealmMeta := func(ctx context.Context, projectName string) *AuthProjectRealmsMeta {
		return &AuthProjectRealmsMeta{
			Kind:         "AuthProjectRealmsMeta",
			ID:           "meta",
			Parent:       datastore.NewKey(ctx, "AuthProjectRealms", fmt.Sprintf("test-project-%s", projectName), 0, RootKey(ctx)),
			ConfigRev:    testsupport.TestRevision,
			ConfigDigest: testsupport.TestContentHash,
			ModifiedTS:   testCreatedTS,
			PermsRev:     "permissions.cfg:123",
		}
	}

	ftt.Run("testing updating realms", t, func(t *ftt.Test) {
		ctx := auth.WithState(gaemem.Use(context.Background()), &authtest.FakeState{
			Identity: "user:someone@example.com",
		})
		ctx = testsupport.SetTestContextSigner(ctx, "test-app-id", "test-app-id@example.com")
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		t.Run("works", func(t *ftt.Test) {
			t.Run("simple config 1 entry", func(t *ftt.Test) {
				configBody, _ := prototext.Marshal(&realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{
							Name: "test-realm",
						},
					},
				})

				revs := []*RealmsCfgRev{
					{
						ProjectID:    "test-project-a",
						ConfigRev:    testsupport.TestRevision,
						ConfigDigest: testsupport.TestContentHash,
						ConfigBody:   configBody,
					},
				}
				err := updateRealms(ctx, testsupport.PermissionsDB(false), revs, false, "latest config")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

				expectedRealmsBody, err := ToStorableRealms(&protocol.Realms{
					Realms: []*protocol.Realm{
						{
							Name: "test-project-a:@root",
						},
						{
							Name: "test-project-a:test-realm",
						},
					},
				})
				assert.Loosely(t, err, should.BeNil)

				fetchedPRealms, err := GetAuthProjectRealms(ctx, "test-project-a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealms, should.Resemble(simpleProjectRealm(ctx, "a", expectedRealmsBody, 1)))

				fetchedPRealmMeta, err := GetAuthProjectRealmsMeta(ctx, "test-project-a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealmMeta, should.Resemble(simpleProjectRealmMeta(ctx, "a")))
			})

			t.Run("updating project entry with config changes", func(t *ftt.Test) {
				cfgBody, _ := prototext.Marshal(&realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{
							Name: "test-realm",
						},
					},
				})

				revs := []*RealmsCfgRev{
					{
						ProjectID:    "test-project-a",
						ConfigRev:    testsupport.TestRevision,
						ConfigDigest: testsupport.TestContentHash,
						ConfigBody:   cfgBody,
					},
				}
				assert.Loosely(t, updateRealms(ctx, testsupport.PermissionsDB(false), revs, false, "latest config"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

				expectedRealmsBody, err := ToStorableRealms(&protocol.Realms{
					Realms: []*protocol.Realm{
						{
							Name: "test-project-a:@root",
						},
						{
							Name: "test-project-a:test-realm",
						},
					},
				})
				assert.Loosely(t, err, should.BeNil)

				fetchedPRealms, err := GetAuthProjectRealms(ctx, "test-project-a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealms, should.Resemble(simpleProjectRealm(ctx, "a", expectedRealmsBody, 1)))

				fetchedPRealmMeta, err := GetAuthProjectRealmsMeta(ctx, "test-project-a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealmMeta, should.Resemble(simpleProjectRealmMeta(ctx, "a")))

				cfgBody, _ = prototext.Marshal(&realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{
							Name: "test-realm",
						},
						{
							Name: "test-realm-2",
						},
					},
				})

				revs = []*RealmsCfgRev{
					{
						ProjectID:    "test-project-a",
						ConfigRev:    testsupport.TestRevision,
						ConfigDigest: testsupport.TestContentHash,
						ConfigBody:   cfgBody,
					},
				}
				assert.Loosely(t, updateRealms(ctx, testsupport.PermissionsDB(false), revs, false, "latest config"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))

				expectedRealmsBody, err = ToStorableRealms(&protocol.Realms{
					Realms: []*protocol.Realm{
						{
							Name: "test-project-a:@root",
						},
						{
							Name: "test-project-a:test-realm",
						},
						{
							Name: "test-project-a:test-realm-2",
						},
					},
				})
				assert.Loosely(t, err, should.BeNil)

				fetchedPRealms, err = GetAuthProjectRealms(ctx, "test-project-a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealms, should.Resemble(simpleProjectRealm(ctx, "a", expectedRealmsBody, 2)))

				fetchedPRealmMeta, err = GetAuthProjectRealmsMeta(ctx, "test-project-a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealmMeta, should.Resemble(simpleProjectRealmMeta(ctx, "a")))
			})

			t.Run("updating many projects", func(t *ftt.Test) {
				cfgBody1, _ := prototext.Marshal(&realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{
							Name: "test-realm",
						},
						{
							Name: "test-realm-2",
						},
					},
				})

				cfgBody2, _ := prototext.Marshal(&realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{
							Name: "test-realm",
						},
						{
							Name: "test-realm-3",
						},
					},
				})

				revs := []*RealmsCfgRev{
					{
						ProjectID:    "test-project-a",
						ConfigRev:    testsupport.TestRevision,
						ConfigDigest: testsupport.TestContentHash,
						ConfigBody:   cfgBody1,
					},
					{
						ProjectID:    "test-project-b",
						ConfigRev:    testsupport.TestRevision,
						ConfigDigest: testsupport.TestContentHash,
						ConfigBody:   cfgBody2,
					},
				}
				assert.Loosely(t, updateRealms(ctx, testsupport.PermissionsDB(false), revs, false, "latest config"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

				expectedRealmsBodyA, err := ToStorableRealms(&protocol.Realms{
					Realms: []*protocol.Realm{
						{
							Name: "test-project-a:@root",
						},
						{
							Name: "test-project-a:test-realm",
						},
						{
							Name: "test-project-a:test-realm-2",
						},
					},
				})
				assert.Loosely(t, err, should.BeNil)

				expectedRealmsBodyB, err := ToStorableRealms(&protocol.Realms{
					Realms: []*protocol.Realm{
						{
							Name: "test-project-b:@root",
						},
						{
							Name: "test-project-b:test-realm",
						},
						{
							Name: "test-project-b:test-realm-3",
						},
					},
				})
				assert.Loosely(t, err, should.BeNil)

				fetchedPRealmsA, err := GetAuthProjectRealms(ctx, "test-project-a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealmsA, should.Resemble(simpleProjectRealm(ctx, "a", expectedRealmsBodyA, 1)))

				fetchedPRealmsB, err := GetAuthProjectRealms(ctx, "test-project-b")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealmsB, should.Resemble(simpleProjectRealm(ctx, "b", expectedRealmsBodyB, 1)))

				fetchedPRealmMetaA, err := GetAuthProjectRealmsMeta(ctx, "test-project-a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealmMetaA, should.Resemble(simpleProjectRealmMeta(ctx, "a")))

				fetchedPRealmMetaB, err := GetAuthProjectRealmsMeta(ctx, "test-project-b")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetchedPRealmMetaB, should.Resemble(simpleProjectRealmMeta(ctx, "b")))
			})
		})
	})
}

func TestProcessRealmsConfigChanges(t *testing.T) {
	t.Parallel()

	ftt.Run("testing realms config changes", t, func(t *ftt.Test) {
		ctx := auth.WithState(gaemem.Use(context.Background()), &authtest.FakeState{
			Identity: "user:someone@example.com",
		})
		ctx = testsupport.SetTestContextSigner(ctx, "test-app-id", "test-app-id@example.com")
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		permsDB := testsupport.PermissionsDB(false)
		configBody, _ := prototext.Marshal(&realmsconf.RealmsCfg{
			Realms: []*realmsconf.Realm{
				{
					Name: "test-realm",
				},
			},
		})

		// makeFetchedCfgRev returns a RealmsCfgRev for the project,
		// with only the fields that would be populated when fetching it
		// from LUCI Config.
		// from stored info.
		makeFetchedCfgRev := func(projectID string) *RealmsCfgRev {
			return &RealmsCfgRev{
				ProjectID:    projectID,
				ConfigRev:    testsupport.TestRevision,
				ConfigDigest: testsupport.TestContentHash,
				ConfigBody:   configBody,
				PermsRev:     "",
			}
		}

		// makeStoredCfgRev returns a RealmsCfgRev for the project,
		// with only the fields that would be populated when creating it
		// from stored info.
		makeStoredCfgRev := func(projectID string) *RealmsCfgRev {
			return &RealmsCfgRev{
				ProjectID:    projectID,
				ConfigRev:    testsupport.TestRevision,
				ConfigDigest: testsupport.TestContentHash,
				ConfigBody:   []byte{},
				PermsRev:     permsDB.Rev,
			}
		}

		// putProjectRealms stores an AuthProjectRealms into datastore
		// for the project.
		putProjectRealms := func(ctx context.Context, projectID string) error {
			return datastore.Put(ctx, &AuthProjectRealms{
				AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
				Kind:                     "AuthProjectRealms",
				ID:                       projectID,
				Parent:                   RootKey(ctx),
			})
		}

		// putProjectRealmsMeta stores an AuthProjectRealmsMeta into datastore
		// for the project.
		putProjectRealmsMeta := func(ctx context.Context, projectID string) error {
			return datastore.Put(ctx, makeAuthProjectRealmsMeta(ctx, projectID))
		}

		// runJobs is a helper function to execute callbacks.
		runJobs := func(jobs []func() error) bool {
			success := true
			for _, job := range jobs {
				if err := job(); err != nil {
					success = false
				}
			}
			return success
		}

		t.Run("no-op when up to date", func(t *ftt.Test) {
			latest := []*RealmsCfgRev{
				makeFetchedCfgRev("@internal"),
				makeFetchedCfgRev("test-project-a"),
			}
			stored := []*RealmsCfgRev{
				makeStoredCfgRev("test-project-a"),
				makeStoredCfgRev("@internal"),
			}
			assert.Loosely(t, putProjectRealms(ctx, "test-project-a"), should.BeNil)
			assert.Loosely(t, putProjectRealms(ctx, "@internal"), should.BeNil)

			jobs, err := processRealmsConfigChanges(ctx, permsDB, latest, stored, false, "Updated from update-realms cron job")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobs, should.BeEmpty)

			assert.Loosely(t, runJobs(jobs), should.BeTrue)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(0))

			actualRealms, err := GetAllAuthProjectRealms(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.HaveLength(2))
			// Confirm realms metadata was not created as this should be no-op.
			actualRealmsMeta, err := GetAllAuthProjectRealmsMeta(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealmsMeta, should.HaveLength(0))
		})

		t.Run("add realms for new project", func(t *ftt.Test) {
			latest := []*RealmsCfgRev{
				makeFetchedCfgRev("@internal"),
				makeFetchedCfgRev("test-project-a"),
			}
			stored := []*RealmsCfgRev{
				makeStoredCfgRev("@internal"),
			}
			assert.Loosely(t, putProjectRealms(ctx, "@internal"), should.BeNil)

			jobs, err := processRealmsConfigChanges(ctx, permsDB, latest, stored, false, "Updated from update-realms cron job")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobs, should.HaveLength(1))

			assert.Loosely(t, runJobs(jobs), should.BeTrue)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

			actualRealms, err := GetAllAuthProjectRealms(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.HaveLength(2))
			actualRealmsMeta, err := GetAllAuthProjectRealmsMeta(ctx)
			assert.Loosely(t, err, should.BeNil)
			// Only one project had its realms created, so only one would have
			// its metadata created.
			assert.Loosely(t, actualRealmsMeta, should.HaveLength(1))
		})

		t.Run("update existing realms.cfg", func(t *ftt.Test) {
			latest := []*RealmsCfgRev{
				makeFetchedCfgRev("@internal"),
				makeFetchedCfgRev("test-project-a"),
			}
			latest[1].ConfigDigest = "different-digest"
			stored := []*RealmsCfgRev{
				makeStoredCfgRev("test-project-a"),
				makeStoredCfgRev("@internal"),
			}
			assert.Loosely(t, putProjectRealms(ctx, "test-project-a"), should.BeNil)
			assert.Loosely(t, putProjectRealms(ctx, "@internal"), should.BeNil)

			jobs, err := processRealmsConfigChanges(ctx, permsDB, latest, stored, false, "Updated from update-realms cron job")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobs, should.HaveLength(1))

			assert.Loosely(t, runJobs(jobs), should.BeTrue)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

			actualRealms, err := GetAllAuthProjectRealms(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.HaveLength(2))
			actualRealmsMeta, err := GetAllAuthProjectRealmsMeta(ctx)
			assert.Loosely(t, err, should.BeNil)
			// Only one project had its realms updated, so only one would have
			// its metadata created as part of the update.
			assert.Loosely(t, actualRealmsMeta, should.HaveLength(1))
		})

		t.Run("delete project realms if realms.cfg no longer exists", func(t *ftt.Test) {
			latest := []*RealmsCfgRev{
				makeFetchedCfgRev("@internal"),
			}
			stored := []*RealmsCfgRev{
				makeStoredCfgRev("test-project-a"),
				makeStoredCfgRev("@internal"),
			}
			assert.Loosely(t, putProjectRealms(ctx, "test-project-a"), should.BeNil)
			assert.Loosely(t, putProjectRealms(ctx, "@internal"), should.BeNil)
			// Set up realms metadata that should be deleted as well.
			assert.Loosely(t, putProjectRealmsMeta(ctx, "test-project-a"), should.BeNil)
			assert.Loosely(t, putProjectRealmsMeta(ctx, "@internal"), should.BeNil)

			jobs, err := processRealmsConfigChanges(ctx, permsDB, latest, stored, false, "Updated from update-realms cron job")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobs, should.HaveLength(1))

			assert.Loosely(t, runJobs(jobs), should.BeTrue)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

			actualRealms, err := GetAllAuthProjectRealms(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.HaveLength(1))
			actualRealmsMeta, err := GetAllAuthProjectRealmsMeta(ctx)
			assert.Loosely(t, err, should.BeNil)
			// Only one project had its realms deleted, so there should still be
			// one remaining.
			assert.Loosely(t, actualRealmsMeta, should.HaveLength(1))
		})

		t.Run("clean up lingering metadata", func(t *ftt.Test) {
			latest := []*RealmsCfgRev{
				makeFetchedCfgRev("@internal"),
			}
			stored := []*RealmsCfgRev{
				makeStoredCfgRev("test-project-a"),
				makeStoredCfgRev("@internal"),
			}
			assert.Loosely(t, putProjectRealms(ctx, "@internal"), should.BeNil)
			// Set up lingering realms metadata.
			assert.Loosely(t, putProjectRealmsMeta(ctx, "test-project-a"), should.BeNil)

			jobs, err := processRealmsConfigChanges(ctx, permsDB, latest, stored, false, "Updated from update-realms cron job")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobs, should.HaveLength(1))

			assert.Loosely(t, runJobs(jobs), should.BeTrue)
			// No replication or changelog generation required for metadata.
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(0))

			actualRealms, err := GetAllAuthProjectRealms(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.HaveLength(1))
			actualRealmsMeta, err := GetAllAuthProjectRealmsMeta(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealmsMeta, should.HaveLength(0))
		})

		t.Run("update if there is a new revision of permissions", func(t *ftt.Test) {
			latest := []*RealmsCfgRev{
				makeFetchedCfgRev("@internal"),
				makeFetchedCfgRev("test-project-a"),
			}
			stored := []*RealmsCfgRev{
				makeStoredCfgRev("test-project-a"),
				makeStoredCfgRev("@internal"),
			}
			stored[1].PermsRev = "permissions.cfg:old"
			assert.Loosely(t, putProjectRealms(ctx, "test-project-a"), should.BeNil)
			assert.Loosely(t, putProjectRealms(ctx, "@internal"), should.BeNil)

			jobs, err := processRealmsConfigChanges(ctx, permsDB, latest, stored, false, "Updated from update-realms cron job")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobs, should.HaveLength(1))

			assert.Loosely(t, runJobs(jobs), should.BeTrue)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

			actualRealms, err := GetAllAuthProjectRealms(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.HaveLength(2))
			actualRealmsMeta, err := GetAllAuthProjectRealmsMeta(ctx)
			assert.Loosely(t, err, should.BeNil)
			// Only one project had its realms updated, so only one would have
			// its metadata created as part of the update.
			assert.Loosely(t, actualRealmsMeta, should.HaveLength(1))
		})

		t.Run("AuthDB revisions are limited when permissions change", func(t *ftt.Test) {
			projectCount := 3 * maxReevaluationRevisions
			latest := make([]*RealmsCfgRev, projectCount)
			stored := make([]*RealmsCfgRev, projectCount)
			for i := 0; i < projectCount; i++ {
				projectID := fmt.Sprintf("test-project-%d", i)
				latest[i] = makeFetchedCfgRev(projectID)
				stored[i] = makeStoredCfgRev(projectID)
				stored[i].PermsRev = "permissions.cfg:old"
				assert.Loosely(t, putProjectRealms(ctx, projectID), should.BeNil)
			}

			jobs, err := processRealmsConfigChanges(ctx, permsDB, latest, stored, false, "Updated from update-realms cron job")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobs, should.HaveLength(maxReevaluationRevisions))

			assert.Loosely(t, runJobs(jobs), should.BeTrue)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2*maxReevaluationRevisions))
		})
	})
}
