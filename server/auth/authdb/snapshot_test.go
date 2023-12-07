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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
	c := context.Background()

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

	Convey("IsAllowedOAuthClientID works", t, func() {
		call := func(email, clientID string) bool {
			res, err := db.IsAllowedOAuthClientID(c, email, clientID)
			So(err, ShouldBeNil)
			return res
		}

		So(call("dude@example.com", ""), ShouldBeFalse)
		So(call("dude@example.com", oauthid.GoogleAPIExplorerClientID), ShouldBeTrue)
		So(call("dude@example.com", "primary-client-id"), ShouldBeTrue)
		So(call("dude@example.com", "additional-client-id-2"), ShouldBeTrue)
		So(call("dude@example.com", "unknown-client-id"), ShouldBeFalse)
	})

	Convey("IsInternalService works", t, func() {
		call := func(hostname string) bool {
			res, err := db.IsInternalService(c, hostname)
			So(err, ShouldBeNil)
			return res
		}

		So(call("i1.example.com"), ShouldBeTrue)
		So(call("i2.example.com"), ShouldBeTrue)
		So(call("abc-dot-i1.example.com"), ShouldBeTrue)
		So(call("external.example.com"), ShouldBeFalse)
		So(call("something-i1.example.com"), ShouldBeFalse)
		So(call("i1.example.com-something"), ShouldBeFalse)
	})

	Convey("IsMember works", t, func() {
		call := func(ident string, groups ...string) bool {
			res, err := db.IsMember(c, identity.Identity(ident), groups)
			So(err, ShouldBeNil)
			return res
		}

		So(call("user:abc@example.com", "direct"), ShouldBeTrue)
		So(call("user:another@example.com", "direct"), ShouldBeFalse)

		So(call("user:abc@example.com", "via glob"), ShouldBeTrue)
		So(call("user:abc@another.com", "via glob"), ShouldBeFalse)

		So(call("user:abc@example.com", "via nested"), ShouldBeTrue)
		So(call("user:another@example.com", "via nested"), ShouldBeFalse)

		So(call("user:abc@example.com", "cycle"), ShouldBeFalse)
		So(call("user:abc@example.com", "unknown"), ShouldBeFalse)
		So(call("user:abc@example.com", "unknown nested"), ShouldBeFalse)

		So(call("user:abc@example.com"), ShouldBeFalse)
		So(call("user:abc@example.com", "unknown", "direct"), ShouldBeTrue)
		So(call("user:abc@example.com", "via glob", "direct"), ShouldBeTrue)
	})

	Convey("CheckMembership works", t, func() {
		call := func(ident string, groups ...string) []string {
			res, err := db.CheckMembership(c, identity.Identity(ident), groups)
			So(err, ShouldBeNil)
			return res
		}

		So(call("user:abc@example.com", "direct"), ShouldResemble, []string{"direct"})
		So(call("user:another@example.com", "direct"), ShouldBeNil)

		So(call("user:abc@example.com", "via glob"), ShouldResemble, []string{"via glob"})
		So(call("user:abc@another.com", "via glob"), ShouldBeNil)

		So(call("user:abc@example.com", "via nested"), ShouldResemble, []string{"via nested"})
		So(call("user:another@example.com", "via nested"), ShouldBeNil)

		So(call("user:abc@example.com", "cycle"), ShouldBeNil)
		So(call("user:abc@example.com", "unknown"), ShouldBeNil)
		So(call("user:abc@example.com", "unknown nested"), ShouldBeNil)

		So(call("user:abc@example.com"), ShouldBeNil)
		So(call("user:abc@example.com", "unknown", "direct"), ShouldResemble, []string{"direct"})
		So(call("user:abc@example.com", "via glob", "direct"), ShouldResemble, []string{"via glob", "direct"})
	})

	Convey("FilterKnownGroups works", t, func() {
		known, err := db.FilterKnownGroups(c, []string{"direct", "unknown", "empty", "direct", "unknown"})
		So(err, ShouldBeNil)
		So(known, ShouldResemble, []string{"direct", "empty", "direct"})
	})

	Convey("With realms", t, func() {
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
		So(err, ShouldBeNil)

		Convey("HasPermission works", func() {
			// A direct hit.
			ok, err := db.HasPermission(c, "user:realm@example.com", perm1, "proj:some/realm", nil)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)

			// A hit through a group.
			ok, err = db.HasPermission(c, "user:abc@example.com", perm1, "proj:some/realm", nil)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)

			// Fallback to the root.
			ok, err = db.HasPermission(c, "user:root@example.com", perm1, "proj:unknown", nil)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)

			// No permission.
			ok, err = db.HasPermission(c, "user:realm@example.com", perm2, "proj:some/realm", nil)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)

			// Unknown root realm.
			ok, err = db.HasPermission(c, "user:realm@example.com", perm1, "unknown:@root", nil)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)

			// Unknown permission.
			ok, err = db.HasPermission(c, "user:realm@example.com", unknownPerm, "proj:some/realm", nil)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)

			// Empty realm.
			ok, err = db.HasPermission(c, "user:realm@example.com", perm1, "proj:empty", nil)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)

			// Invalid realm name.
			_, err = db.HasPermission(c, "user:realm@example.com", perm1, "@root", nil)
			So(err, ShouldErrLike, "bad global realm name")
		})

		Convey("Conditional bindings", func() {
			ok, err := db.HasPermission(c, "user:cond@example.com", perm1, "proj:some/realm", nil)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)

			ok, err = db.HasPermission(c, "user:cond@example.com", perm1, "proj:some/realm", realms.Attrs{"a": "ok_1"})
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)

			ok, err = db.HasPermission(c, "user:cond@example.com", perm1, "proj:some/realm", realms.Attrs{"a": "ok_2"})
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)

			ok, err = db.HasPermission(c, "user:cond@example.com", perm1, "proj:some/realm", realms.Attrs{"a": "???"})
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)

			ok, err = db.HasPermission(c, "user:cond@example.com", perm2, "proj:some/realm", realms.Attrs{"a": "ok_1"})
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("QueryRealms works", func() {
			// A direct hit.
			r, err := db.QueryRealms(c, "user:realm@example.com", perm1, "", nil)
			sort.Strings(r)
			So(err, ShouldBeNil)
			So(r, ShouldResemble, []string{"another:realm", "proj:some/realm"})

			// Filtering by project.
			r, err = db.QueryRealms(c, "user:realm@example.com", perm1, "proj", nil)
			So(err, ShouldBeNil)
			So(r, ShouldResemble, []string{"proj:some/realm"})

			// A hit through a group.
			r, err = db.QueryRealms(c, "user:abc@example.com", perm1, "", nil)
			So(err, ShouldBeNil)
			So(r, ShouldResemble, []string{"proj:some/realm"})
		})

		Convey("QueryRealms with conditional bindings", func() {
			r, err := db.QueryRealms(c, "user:cond@example.com", perm1, "", nil)
			So(err, ShouldBeNil)
			So(r, ShouldBeEmpty)

			r, err = db.QueryRealms(c, "user:cond@example.com", perm1, "", realms.Attrs{"a": "ok_1"})
			So(err, ShouldBeNil)
			So(r, ShouldResemble, []string{"proj:some/realm"})

			r, err = db.QueryRealms(c, "user:cond@example.com", perm1, "", realms.Attrs{"a": "???"})
			So(err, ShouldBeNil)
			So(r, ShouldBeEmpty)
		})

		Convey("QueryRealms with unindexed permission", func() {
			_, err := db.QueryRealms(c, "user:realm@example.com", perm2, "", nil)
			So(err, ShouldErrLike, "permission luci.dev.testing2 cannot be used in QueryRealms")
		})

		Convey("GetRealmData works", func() {
			// Known realm with data.
			d, err := db.GetRealmData(c, "proj:some/realm")
			So(err, ShouldBeNil)
			So(d, ShouldResembleProto, &protocol.RealmData{
				EnforceInService: []string{"some"},
			})

			// Known realm, with no data.
			d, err = db.GetRealmData(c, "proj:empty")
			So(err, ShouldBeNil)
			So(d, ShouldResembleProto, &protocol.RealmData{})

			// Fallback to root.
			d, err = db.GetRealmData(c, "proj:unknown")
			So(err, ShouldBeNil)
			So(d, ShouldResembleProto, &protocol.RealmData{
				EnforceInService: []string{"root"},
			})

			// Completely unknown.
			d, err = db.GetRealmData(c, "unknown:unknown")
			So(err, ShouldBeNil)
			So(d, ShouldBeNil)
		})
	})

	Convey("GetCertificates works", t, func(c C) {
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
		So(err, ShouldBeNil)
		So(certs, ShouldNotBeNil)

		// Fetched one bundle.
		So(calls, ShouldEqual, 1)

		// For unknown signer returns (nil, nil).
		certs, err = db.GetCertificates(ctx, "user:unknown@example.com")
		So(err, ShouldBeNil)
		So(certs, ShouldBeNil)
	})

	Convey("IsAllowedIP works", t, func() {
		l, err := db.GetAllowlistForIdentity(c, "user:abc@example.com")
		So(err, ShouldBeNil)
		So(l, ShouldEqual, "allowlist")

		l, err = db.GetAllowlistForIdentity(c, "user:unknown@example.com")
		So(err, ShouldBeNil)
		So(l, ShouldEqual, "")

		call := func(ip, allowlist string) bool {
			ipaddr := net.ParseIP(ip)
			So(ipaddr, ShouldNotBeNil)
			res, err := db.IsAllowedIP(c, ipaddr, allowlist)
			So(err, ShouldBeNil)
			return res
		}

		So(call("1.2.3.4", "allowlist"), ShouldBeTrue)
		So(call("10.255.255.255", "allowlist"), ShouldBeTrue)
		So(call("9.255.255.255", "allowlist"), ShouldBeFalse)
		So(call("1.2.3.4", "empty"), ShouldBeFalse)
	})

	Convey("Revision works", t, func() {
		So(Revision(&SnapshotDB{Rev: 123}), ShouldEqual, 123)
		So(Revision(ErroringDB{}), ShouldEqual, 0)
		So(Revision(nil), ShouldEqual, 0)
	})

	Convey("SnapshotDBFromTextProto works", t, func() {
		db, err := SnapshotDBFromTextProto(strings.NewReader(`
			groups {
				name: "group"
				members: "user:a@example.com"
			}
		`))
		So(err, ShouldBeNil)
		yes, err := db.IsMember(c, "user:a@example.com", []string{"group"})
		So(err, ShouldBeNil)
		So(yes, ShouldBeTrue)
	})

	Convey("SnapshotDBFromTextProto bad proto", t, func() {
		_, err := SnapshotDBFromTextProto(strings.NewReader(`
			groupz {}
		`))
		So(err, ShouldErrLike, "not a valid AuthDB text proto file")
	})

	Convey("SnapshotDBFromTextProto bad structure", t, func() {
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
		So(err, ShouldErrLike, "dependency cycle found")
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
	for i := 0; i < groups; i++ {
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
