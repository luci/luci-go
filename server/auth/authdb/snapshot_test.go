// Copyright 2016 The LUCI Authors.
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

package authdb

import (
	"context"
	"encoding/json"
	"flag"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/authdb/internal/graph"
	"go.chromium.org/luci/server/auth/authdb/internal/legacy"
	"go.chromium.org/luci/server/auth/authdb/internal/oauthid"
	"go.chromium.org/luci/server/auth/authdb/internal/realmset"
	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/caching"
)

var (
	perm1       = realms.RegisterPermission("luci.dev.testing1")
	perm2       = realms.RegisterPermission("luci.dev.testing2")
	unknownPerm = realms.RegisterPermission("luci.dev.unknown")

	// For the benchmark that uses real AuthDB dumps.
	bbBuildsGet = realms.RegisterPermission("buildbucket.builds.get")
)

func init() {
	perm1.AddFlags(realms.UsedInQueryRealms)
	bbBuildsGet.AddFlags(realms.UsedInQueryRealms)
}

func TestSnapshotDB(t *testing.T) {
	ctx := context.Background()

	securityConfig, _ := proto.Marshal(&protocol.SecurityConfig{
		InternalServiceRegexp: []string{
			`(.*-dot-)?i1\.example\.com`,
			`(.*-dot-)?i2\.example\.com`,
		},
	})

	db, err := NewSnapshotDB(&protocol.AuthDB{
		OauthClientId: "primary-client-id",
		OauthAdditionalClientIds: []string{
			"additional-client-id-1",
			"additional-client-id-2",
		},
		TokenServerUrl: "http://token-server",
		Groups: []*protocol.AuthGroup{
			{
				Name:    "direct",
				Members: []string{"user:abc@example.com"},
			},
			{
				Name:  "via glob",
				Globs: []string{"user:*@example.com"},
			},
			{
				Name:   "via nested",
				Nested: []string{"direct"},
			},
			{
				Name:   "cycle",
				Nested: []string{"cycle"},
			},
			{
				Name:   "unknown nested",
				Nested: []string{"unknown"},
			},
			{
				Name: "empty",
			},
		},
		IpWhitelistAssignments: []*protocol.AuthIPWhitelistAssignment{
			{
				Identity:    "user:abc@example.com",
				IpWhitelist: "allowlist",
			},
		},
		IpWhitelists: []*protocol.AuthIPWhitelist{
			{
				Name: "allowlist",
				Subnets: []string{
					"1.2.3.4/32",
					"10.0.0.0/8",
				},
			},
			{
				Name: "empty",
			},
		},
		SecurityConfig: securityConfig,
	}, "http://auth-service", 1234, false)
	if err != nil {
		panic(err)
	}

	ftt.Run("IsAllowedOAuthClientID works", t, func(t *ftt.Test) {
		call := func(email, clientID string) bool {
			res, err := db.IsAllowedOAuthClientID(ctx, email, clientID)
			assert.Loosely(t, err, should.BeNil)
			return res
		}

		assert.Loosely(t, call("dude@example.com", ""), should.BeFalse)
		assert.Loosely(t, call("dude@example.com", oauthid.GoogleAPIExplorerClientID), should.BeTrue)
		assert.Loosely(t, call("dude@example.com", "primary-client-id"), should.BeTrue)
		assert.Loosely(t, call("dude@example.com", "additional-client-id-2"), should.BeTrue)
		assert.Loosely(t, call("dude@example.com", "unknown-client-id"), should.BeFalse)
	})

	ftt.Run("IsInternalService works", t, func(t *ftt.Test) {
		call := func(hostname string) bool {
			res, err := db.IsInternalService(ctx, hostname)
			assert.Loosely(t, err, should.BeNil)
			return res
		}

		assert.Loosely(t, call("i1.example.com"), should.BeTrue)
		assert.Loosely(t, call("i2.example.com"), should.BeTrue)
		assert.Loosely(t, call("abc-dot-i1.example.com"), should.BeTrue)
		assert.Loosely(t, call("external.example.com"), should.BeFalse)
		assert.Loosely(t, call("something-i1.example.com"), should.BeFalse)
		assert.Loosely(t, call("i1.example.com-something"), should.BeFalse)
	})

	ftt.Run("IsMember works", t, func(t *ftt.Test) {
		call := func(ident string, groups ...string) bool {
			res, err := db.IsMember(ctx, identity.Identity(ident), groups)
			assert.Loosely(t, err, should.BeNil)
			return res
		}

		assert.Loosely(t, call("user:abc@example.com", "direct"), should.BeTrue)
		assert.Loosely(t, call("user:abc@Example.com", "direct"), should.BeTrue)
		assert.Loosely(t, call("user:another@example.com", "direct"), should.BeFalse)
		assert.Loosely(t, call("user:another@Example.com", "direct"), should.BeFalse)

		assert.Loosely(t, call("user:abc@example.com", "via glob"), should.BeTrue)
		assert.Loosely(t, call("user:Abc@example.com", "via glob"), should.BeTrue)
		assert.Loosely(t, call("user:abc@Example.com", "via glob"), should.BeFalse)
		assert.Loosely(t, call("user:abc@another.com", "via glob"), should.BeFalse)
		assert.Loosely(t, call("user:Abc@another.com", "via glob"), should.BeFalse)
		assert.Loosely(t, call("user:abc@Another.com", "via glob"), should.BeFalse)

		assert.Loosely(t, call("user:abc@example.com", "via nested"), should.BeTrue)
		assert.Loosely(t, call("user:Abc@Example.com", "via nested"), should.BeTrue)
		assert.Loosely(t, call("user:another@example.com", "via nested"), should.BeFalse)

		assert.Loosely(t, call("user:abc@example.com", "cycle"), should.BeFalse)
		assert.Loosely(t, call("user:abc@example.com", "unknown"), should.BeFalse)
		assert.Loosely(t, call("user:abc@example.com", "unknown nested"), should.BeFalse)

		assert.Loosely(t, call("user:abc@example.com"), should.BeFalse)
		assert.Loosely(t, call("user:abc@example.com", "unknown", "direct"), should.BeTrue)
		assert.Loosely(t, call("user:abc@Example.com", "unknown", "direct"), should.BeTrue)
		assert.Loosely(t, call("user:abc@example.com", "via glob", "direct"), should.BeTrue)
		assert.Loosely(t, call("user:abc@Example.com", "via glob", "direct"), should.BeTrue)
		assert.Loosely(t, call("user:abc@Example.com", "unknown", "via glob"), should.BeFalse)
	})

	ftt.Run("CheckMembership works", t, func(t *ftt.Test) {
		call := func(ident string, groups ...string) []string {
			res, err := db.CheckMembership(ctx, identity.Identity(ident), groups)
			assert.Loosely(t, err, should.BeNil)
			return res
		}

		assert.Loosely(t, call("user:abc@example.com", "direct"), should.Match([]string{"direct"}))
		assert.Loosely(t, call("user:Abc@Example.COM", "direct"), should.Match([]string{"direct"}))
		assert.Loosely(t, call("user:another@example.com", "direct"), should.BeNil)

		assert.Loosely(t, call("user:abc@example.com", "via glob"), should.Match([]string{"via glob"}))
		assert.Loosely(t, call("user:abc@another.com", "via glob"), should.BeNil)

		assert.Loosely(t, call("user:abc@example.com", "via nested"), should.Match([]string{"via nested"}))
		assert.Loosely(t, call("user:Abc@Example.COM", "via nested"), should.Match([]string{"via nested"}))
		assert.Loosely(t, call("user:another@example.com", "via nested"), should.BeNil)

		assert.Loosely(t, call("user:abc@example.com", "cycle"), should.BeNil)
		assert.Loosely(t, call("user:abc@example.com", "unknown"), should.BeNil)
		assert.Loosely(t, call("user:abc@example.com", "unknown nested"), should.BeNil)

		assert.Loosely(t, call("user:abc@example.com"), should.BeNil)
		assert.Loosely(t, call("user:abc@example.com", "unknown", "direct"), should.Match([]string{"direct"}))
		assert.Loosely(t, call("user:abc@example.com", "via glob", "direct"), should.Match([]string{"via glob", "direct"}))
	})

	ftt.Run("FilterKnownGroups works", t, func(t *ftt.Test) {
		known, err := db.FilterKnownGroups(ctx, []string{"direct", "unknown", "empty", "direct", "unknown"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, known, should.Match([]string{"direct", "empty", "direct"}))
	})

	ftt.Run("With realms", t, func(t *ftt.Test) {
		db, err := NewSnapshotDB(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{
					Name:    "direct",
					Members: []string{"user:abc@example.com"},
				},
			},
			Realms: &protocol.Realms{
				ApiVersion: realmset.ExpectedAPIVersion,
				Permissions: []*protocol.Permission{
					{Name: perm1.Name()},
					{Name: perm2.Name()},
				},
				Conditions: []*protocol.Condition{
					{
						Op: &protocol.Condition_Restrict{
							Restrict: &protocol.Condition_AttributeRestriction{
								Attribute: "a",
								Values:    []string{"ok_1", "ok_2"},
							},
						},
					},
				},
				Realms: []*protocol.Realm{
					{
						Name: "proj:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Principals:  []string{"user:root@example.com"},
							},
						},
						Data: &protocol.RealmData{
							EnforceInService: []string{"root"},
						},
					},
					{
						Name: "proj:some/realm",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Principals:  []string{"user:realm@example.com", "group:direct"},
							},
							{
								Conditions:  []uint32{0},
								Permissions: []uint32{0},
								Principals:  []string{"user:cond@example.com"},
							},
						},
						Data: &protocol.RealmData{
							EnforceInService: []string{"some"},
						},
					},
					{
						Name: "another:realm",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Principals:  []string{"user:realm@example.com"},
							},
						},
					},
					{
						Name: "proj:empty",
					},
				},
			},
		}, "http://auth-service", 1234, false)
		assert.Loosely(t, err, should.BeNil)

		t.Run("HasPermission works", func(t *ftt.Test) {
			// A direct hit.
			ok, err := db.HasPermission(ctx, "user:realm@example.com", perm1, "proj:some/realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)

			// A hit through a group.
			ok, err = db.HasPermission(ctx, "user:abc@example.com", perm1, "proj:some/realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)

			// A hit through a group despite email having different casing.
			ok, err = db.HasPermission(ctx, "user:Abc@Example.COM", perm1, "proj:some/realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)

			// Fallback to the root.
			ok, err = db.HasPermission(ctx, "user:root@example.com", perm1, "proj:unknown", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)

			// No permission.
			ok, err = db.HasPermission(ctx, "user:realm@example.com", perm2, "proj:some/realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)

			// Unknown root realm.
			ok, err = db.HasPermission(ctx, "user:realm@example.com", perm1, "unknown:@root", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)

			// Unknown permission.
			ok, err = db.HasPermission(ctx, "user:realm@example.com", unknownPerm, "proj:some/realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)

			// Empty realm.
			ok, err = db.HasPermission(ctx, "user:realm@example.com", perm1, "proj:empty", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)

			// Invalid realm name.
			_, err = db.HasPermission(ctx, "user:realm@example.com", perm1, "@root", nil)
			assert.Loosely(t, err, should.ErrLike("bad global realm name"))
		})

		t.Run("Conditional bindings", func(t *ftt.Test) {
			ok, err := db.HasPermission(ctx, "user:cond@example.com", perm1, "proj:some/realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)

			ok, err = db.HasPermission(ctx, "user:cond@example.com", perm1, "proj:some/realm", realms.Attrs{"a": "ok_1"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)

			ok, err = db.HasPermission(ctx, "user:cond@example.com", perm1, "proj:some/realm", realms.Attrs{"a": "ok_2"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)

			ok, err = db.HasPermission(ctx, "user:cond@example.com", perm1, "proj:some/realm", realms.Attrs{"a": "???"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)

			ok, err = db.HasPermission(ctx, "user:cond@example.com", perm2, "proj:some/realm", realms.Attrs{"a": "ok_1"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("QueryRealms works", func(t *ftt.Test) {
			// A direct hit.
			r, err := db.QueryRealms(ctx, "user:realm@example.com", perm1, "", nil)
			sort.Strings(r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.Match([]string{"another:realm", "proj:some/realm"}))

			// A direct hit with different casing.
			r, err = db.QueryRealms(ctx, "user:Realm@example.com", perm1, "", nil)
			sort.Strings(r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.Match([]string{"another:realm", "proj:some/realm"}))

			// Filtering by project.
			r, err = db.QueryRealms(ctx, "user:realm@example.com", perm1, "proj", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.Match([]string{"proj:some/realm"}))

			// A hit through a group.
			r, err = db.QueryRealms(ctx, "user:abc@example.com", perm1, "", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.Match([]string{"proj:some/realm"}))
		})

		t.Run("QueryRealms with conditional bindings", func(t *ftt.Test) {
			r, err := db.QueryRealms(ctx, "user:cond@example.com", perm1, "", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.BeEmpty)

			r, err = db.QueryRealms(ctx, "user:cond@example.com", perm1, "", realms.Attrs{"a": "ok_1"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.Match([]string{"proj:some/realm"}))

			r, err = db.QueryRealms(ctx, "user:cond@example.com", perm1, "", realms.Attrs{"a": "???"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.BeEmpty)
		})

		t.Run("QueryRealms with unindexed permission", func(t *ftt.Test) {
			_, err := db.QueryRealms(ctx, "user:realm@example.com", perm2, "", nil)
			assert.Loosely(t, err, should.ErrLike("permission luci.dev.testing2 cannot be used in QueryRealms"))
		})

		t.Run("GetRealmData works", func(t *ftt.Test) {
			// Known realm with data.
			d, err := db.GetRealmData(ctx, "proj:some/realm")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, d, should.Match(&protocol.RealmData{
				EnforceInService: []string{"some"},
			}))

			// Known realm, with no data.
			d, err = db.GetRealmData(ctx, "proj:empty")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, d, should.Match(&protocol.RealmData{}))

			// Fallback to root.
			d, err = db.GetRealmData(ctx, "proj:unknown")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, d, should.Match(&protocol.RealmData{
				EnforceInService: []string{"root"},
			}))

			// Completely unknown.
			d, err = db.GetRealmData(ctx, "unknown:unknown")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, d, should.BeNil)
		})
	})

	ftt.Run("GetCertificates works", t, func(t *ftt.Test) {
		tokenService := signingtest.NewSigner(&signing.ServiceInfo{
			AppID:              "token-server",
			ServiceAccountName: "token-server-account@example.com",
		})

		calls := 0

		ctx := context.Background()
		ctx = caching.WithEmptyProcessCache(ctx)

		ctx = internal.WithTestTransport(ctx, func(r *http.Request, body string) (int, string) {
			calls++
			if r.URL.String() != "http://token-server/auth/api/v1/server/certificates" {
				return 404, "Wrong URL"
			}
			certs, err := tokenService.Certificates(ctx)
			if err != nil {
				panic(err)
			}
			blob, err := json.Marshal(certs)
			if err != nil {
				panic(err)
			}
			return 200, string(blob)
		})

		certs, err := db.GetCertificates(ctx, "user:token-server-account@example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, certs, should.NotBeNil)

		// Fetched one bundle.
		assert.Loosely(t, calls, should.Equal(1))

		// For unknown signer returns (nil, nil).
		certs, err = db.GetCertificates(ctx, "user:unknown@example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, certs, should.BeNil)
	})

	ftt.Run("IsAllowedIP works", t, func(t *ftt.Test) {
		l, err := db.GetAllowlistForIdentity(ctx, "user:abc@example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, l, should.Equal("allowlist"))

		l, err = db.GetAllowlistForIdentity(ctx, "user:unknown@example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, l, should.BeEmpty)

		call := func(ip, allowlist string) bool {
			ipaddr := net.ParseIP(ip)
			assert.Loosely(t, ipaddr, should.NotBeNil)
			res, err := db.IsAllowedIP(ctx, ipaddr, allowlist)
			assert.Loosely(t, err, should.BeNil)
			return res
		}

		assert.Loosely(t, call("1.2.3.4", "allowlist"), should.BeTrue)
		assert.Loosely(t, call("10.255.255.255", "allowlist"), should.BeTrue)
		assert.Loosely(t, call("9.255.255.255", "allowlist"), should.BeFalse)
		assert.Loosely(t, call("1.2.3.4", "empty"), should.BeFalse)
	})

	ftt.Run("Revision works", t, func(t *ftt.Test) {
		assert.Loosely(t, Revision(&SnapshotDB{Rev: 123}), should.Equal(123))
		assert.Loosely(t, Revision(ErroringDB{}), should.BeZero)
		assert.Loosely(t, Revision(nil), should.BeZero)
	})

	ftt.Run("SnapshotDBFromTextProto works", t, func(t *ftt.Test) {
		db, err := SnapshotDBFromTextProto(strings.NewReader(`
			groups {
				name: "group"
				members: "user:a@example.com"
			}
		`))
		assert.Loosely(t, err, should.BeNil)
		yes, err := db.IsMember(ctx, "user:a@example.com", []string{"group"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, yes, should.BeTrue)
	})

	ftt.Run("SnapshotDBFromTextProto bad proto", t, func(t *ftt.Test) {
		_, err := SnapshotDBFromTextProto(strings.NewReader(`
			groupz {}
		`))
		assert.Loosely(t, err, should.ErrLike("not a valid AuthDB text proto file"))
	})

	ftt.Run("SnapshotDBFromTextProto bad structure", t, func(t *ftt.Test) {
		_, err := SnapshotDBFromTextProto(strings.NewReader(`
			groups {
				name: "group 1"
				nested: "group 2"
			}
			groups {
				name: "group 2"
				nested: "group 1"
			}
		`))
		assert.Loosely(t, err, should.ErrLike("dependency cycle found"))
	})
}

var authDBPath = flag.String("authdb", "", "path to AuthDB proto to use in tests")
var testAuthDB *protocol.AuthDB

func TestMain(m *testing.M) {
	flag.Parse()
	if *authDBPath != "" {
		testAuthDB = readTestDB(*authDBPath)
	}
	os.Exit(m.Run())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

func randString(min, max int, seen stringset.Set) string {
	for {
		b := make([]rune, min+rand.Intn(max))
		for i := range b {
			b[i] = letterRunes[rand.Intn(len(letterRunes))]
		}
		s := string(b)
		if seen.Add(s) {
			return s
		}
	}
}

func readTestDB(path string) *protocol.AuthDB {
	blob, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	authdb := protocol.AuthDB{}
	if err := proto.Unmarshal(blob, &authdb); err != nil {
		panic(err)
	}
	return &authdb
}

func makeTestDB(users, groups int) *protocol.AuthDB {
	if testAuthDB != nil {
		return testAuthDB
	}

	db := &protocol.AuthDB{}

	domains := make([]string, 50)
	seenDomains := stringset.New(50)
	for i := range domains {
		domains[i] = "@" + randString(5, 15, seenDomains) + ".com"
	}
	members := make([]string, users)
	seenMembers := stringset.New(users)
	for i := range members {
		members[i] = "user:" + randString(3, 20, seenMembers) + domains[rand.Intn(len(domains))]
	}

	seenGroups := stringset.New(groups)
	for range groups {
		s := rand.Intn(len(members))
		l := rand.Intn(len(members) - s)
		db.Groups = append(db.Groups, &protocol.AuthGroup{
			Name:    randString(10, 30, seenGroups),
			Members: members[s : s+l],
		})
	}

	return db
}

type queryableGraph interface {
	IsMember(ident identity.Identity, group string) graph.IsMemberResult
}

func oldQueryableGraph(db *protocol.AuthDB) queryableGraph {
	q, err := legacy.BuildGroups(db.Groups)
	if err != nil {
		panic(err)
	}
	return q
}

func newQueryableGraph(db *protocol.AuthDB) queryableGraph {
	q, err := graph.BuildQueryable(db.Groups)
	if err != nil {
		panic(err)
	}
	return q
}

func memUsage(t *testing.T, cb func(*runtime.MemStats)) {
	var m1, m2 runtime.MemStats

	cb(&m1)
	runtime.GC()
	runtime.ReadMemStats(&m2)

	t.Logf("HeapAlloc: %1.f Kb", float64(m1.HeapAlloc-m2.HeapAlloc)/1024)
}

func runMemUsageTest(t *testing.T, cb func(db *protocol.AuthDB) queryableGraph) {
	db := makeTestDB(1000, 500)
	memUsage(t, func(m *runtime.MemStats) {
		q := cb(db)
		runtime.GC()
		runtime.ReadMemStats(m)
		runtime.KeepAlive(q)
	})
}

// Note: to run some benchmarks with real AuthDB:
//   go run go.chromium.org/luci/auth/client/cmd/authdb-dump -output-proto-file auth.db
//   go test . -run=TestMemUsage* -v -authdb auth.db
//   go test -bench=QueryRealms -run=XXX -authdb auth.db

func TestMemUsageOld(t *testing.T) {
	runMemUsageTest(t, oldQueryableGraph)
}

func TestMemUsageNew(t *testing.T) {
	runMemUsageTest(t, newQueryableGraph)
}

func TestCompareNewAndOld(t *testing.T) {
	db := makeTestDB(100, 50)
	old := oldQueryableGraph(db)
	new := newQueryableGraph(db)

	idSet := stringset.New(0)
	for _, g := range db.Groups {
		for _, m := range g.Members {
			idSet.Add(m)
		}
	}
	idents := idSet.ToSlice()
	sort.Strings(idents)

	for _, g := range db.Groups {
		for _, id := range idents {
			r1 := old.IsMember(identity.Identity(id), g.Name)
			r2 := new.IsMember(identity.Identity(id), g.Name)
			if r1 != r2 {
				t.Fatalf("IsMember(%q, %q): %v != %v", id, g.Name, r1, r2)
			}
		}
	}
}

func runIsMemberBenchmark(b *testing.B, db *protocol.AuthDB, q queryableGraph) {
	idSet := stringset.New(0)
	for _, g := range db.Groups {
		for _, m := range g.Members {
			idSet.Add(m)
		}
	}
	idents := idSet.ToSlice()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, g := range db.Groups {
			for _, id := range idents {
				q.IsMember(identity.Identity(id), g.Name)
			}
		}
	}
}

func BenchmarkIsMemberOld(b *testing.B) {
	db := makeTestDB(100, 50)
	runIsMemberBenchmark(b, db, oldQueryableGraph(db))
}

func BenchmarkIsMemberNew(b *testing.B) {
	db := makeTestDB(100, 50)
	runIsMemberBenchmark(b, db, newQueryableGraph(db))
}

func BenchmarkQueryRealms(b *testing.B) {
	ctx := context.Background()
	db, err := NewSnapshotDB(makeTestDB(100, 50), "http://auth-service", 1234, false)
	if err != nil {
		b.Fatalf("bad AuthDB: %s", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		realms, err := db.QueryRealms(ctx, "user:someone@example.com", bbBuildsGet, "", nil)
		if i == 0 {
			if err != nil {
				b.Fatalf("QueryRealms failed: %s", err)
			}
			b.Logf("Hits: %d", len(realms))
		}
	}
}
