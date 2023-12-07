// Copyright 2015 The LUCI Authors.
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

package authtest

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
)

// FakeDB implements authdb.DB by mocking membership and permission checks.
//
// Initialize it with a bunch of mocks like:
//
// db := authtest.NewFakeDB(
//
//	authtest.MockMembership("user:a@example.com", "group"),
//	authtest.MockPermission("user:a@example.com", "proj:realm", perm),
//	...
//
// )
//
// The list of mocks can also be extended later via db.AddMocks(...).
type FakeDB struct {
	m         sync.RWMutex
	err       error                              // if not nil, return this error
	perID     map[identity.Identity]*mockedForID // id => groups and perms it has
	ips       map[string]stringset.Set           // IP => allowlists it belongs to
	realmData map[string]*protocol.RealmData     // realm name => data
	groups    stringset.Set                      // groups mentioned by the mocks
}

var _ authdb.DB = (*FakeDB)(nil)

// Condition evaluates attributes passed to HasPermission and decides if the
// permission should apply.
//
// Used for mocking conditional bindings.
type Condition func(realms.Attrs) bool

// RestrictAttribute produces a Condition that check the given attribute has any
// of the given values.
//
// Its logic matches AttributeRestriction condition in the RealmsDB.
func RestrictAttribute(attr string, vals ...string) Condition {
	set := stringset.NewFromSlice(vals...)
	return func(attrs realms.Attrs) bool {
		val, ok := attrs[attr]
		return ok && set.Has(val)
	}
}

// mockedForID is mocked groups and permissions of some identity.
type mockedForID struct {
	groups stringset.Set // a set of group names
	perms  []mockedPerm
}

// mockedPerm is a single permission of a single identity.
type mockedPerm struct {
	realm string
	perm  realms.Permission
	cond  Condition
}

// MockedDatum is a return value of various Mock* constructors.
type MockedDatum struct {
	// apply mutates the db to apply the mock, called under the write lock.
	apply func(db *FakeDB)
}

// MockMembership modifies db to make IsMember(id, group) == true.
func MockMembership(id identity.Identity, group string) MockedDatum {
	return MockedDatum{
		apply: func(db *FakeDB) {
			db.addGroup(group)
			db.mockedForID(id).groups.Add(group)
		},
	}
}

// MockGroup adds a group (potentially empty) to the fake DB.
func MockGroup(group string, ids []identity.Identity) MockedDatum {
	return MockedDatum{
		apply: func(db *FakeDB) {
			db.addGroup(group)
			for _, id := range ids {
				db.mockedForID(id).groups.Add(group)
			}
		},
	}
}

// MockPermission modifies db to make HasPermission(id, realm, perm, …) == true.
//
// Panics if `realm` is not a valid globally scoped realm, i.e. it doesn't look
// like "<project>:<realm>".
//
// Optional `conds` allow mocking conditional bindings by defining a condition
// on realms.Attrs that must evaluate to true to allow this permission. Multiple
// `conds` callbacks are AND'ed together to get the final verdict.
func MockPermission(id identity.Identity, realm string, perm realms.Permission, conds ...Condition) MockedDatum {
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		panic(err)
	}
	return MockedDatum{
		apply: func(db *FakeDB) {
			perID := db.mockedForID(id)
			perID.perms = append(perID.perms, mockedPerm{
				realm: realm,
				perm:  perm,
				cond: func(attrs realms.Attrs) bool {
					for _, cond := range conds {
						if !cond(attrs) {
							return false
						}
					}
					return true
				},
			})
		},
	}
}

// MockRealmData modifies what db's GetRealmData returns.
//
// Panics if `realm` is not a valid globally scoped realm, i.e. it doesn't look
// like "<project>:<realm>".
func MockRealmData(realm string, data *protocol.RealmData) MockedDatum {
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		panic(err)
	}
	return MockedDatum{
		apply: func(db *FakeDB) {
			if db.realmData == nil {
				db.realmData = make(map[string]*protocol.RealmData, 1)
			}
			db.realmData[realm] = data
		},
	}
}

// MockIPAllowlist modifies db to make IsAllowedIP(ip, allowlist) == true.
//
// Panics if `ip` is not a valid IP address.
func MockIPAllowlist(ip, allowlist string) MockedDatum {
	if net.ParseIP(ip) == nil {
		panic(fmt.Sprintf("%q is not a valid IP address", ip))
	}
	return MockedDatum{
		apply: func(db *FakeDB) {
			l, ok := db.ips[ip]
			if !ok {
				l = stringset.New(1)
				if db.ips == nil {
					db.ips = make(map[string]stringset.Set, 1)
				}
				db.ips[ip] = l
			}
			l.Add(allowlist)
		},
	}
}

// MockError modifies db to make its methods return this error.
//
// `err` may be nil, in which case the previously mocked error is removed.
func MockError(err error) MockedDatum {
	return MockedDatum{
		apply: func(db *FakeDB) { db.err = err },
	}
}

// NewFakeDB creates a FakeDB populated with the given mocks.
//
// Construct mocks using MockMembership, MockPermission, MockIPAllowlist and
// MockError functions.
func NewFakeDB(mocks ...MockedDatum) *FakeDB {
	db := &FakeDB{}
	db.AddMocks(mocks...)
	return db
}

// AddMocks applies a bunch of mocks to the state in the db.
func (db *FakeDB) AddMocks(mocks ...MockedDatum) {
	db.m.Lock()
	defer db.m.Unlock()
	for _, m := range mocks {
		m.apply(db)
	}
}

// Use installs the fake db into the context.
//
// Note that if you use auth.WithState(ctx, &authtest.FakeState{...}), you don't
// need this method. Modify FakeDB in the FakeState instead. See its doc for
// some examples.
func (db *FakeDB) Use(ctx context.Context) context.Context {
	return auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
		cfg.DBProvider = func(context.Context) (authdb.DB, error) {
			return db, nil
		}
		return cfg
	})
}

// IsMember is part of authdb.DB interface.
func (db *FakeDB) IsMember(ctx context.Context, id identity.Identity, groups []string) (bool, error) {
	hits, err := db.CheckMembership(ctx, id, groups)
	if err != nil {
		return false, err
	}
	return len(hits) > 0, nil
}

// CheckMembership is part of authdb.DB interface.
func (db *FakeDB) CheckMembership(ctx context.Context, id identity.Identity, groups []string) (out []string, err error) {
	db.m.RLock()
	defer db.m.RUnlock()

	if db.err != nil {
		return nil, db.err
	}

	if mocked := db.perID[id]; mocked != nil {
		for _, group := range groups {
			if mocked.groups.Has(group) {
				out = append(out, group)
			}
		}
	}

	return
}

// HasPermission is part of authdb.DB interface.
func (db *FakeDB) HasPermission(ctx context.Context, id identity.Identity, perm realms.Permission, realm string, attrs realms.Attrs) (bool, error) {
	// This flips a flag forbidding registration of new permissions. Presumably
	// this should help catching "dynamic" permission registration in tests,
	// before it panics in production.
	realms.ForbidPermissionChanges()

	db.m.RLock()
	defer db.m.RUnlock()

	if db.err != nil {
		return false, db.err
	}

	if mocked := db.perID[id]; mocked != nil {
		for _, mockedPerm := range mocked.perms {
			if mockedPerm.realm == realm && mockedPerm.perm == perm && mockedPerm.cond(attrs) {
				return true, nil
			}
		}
	}

	return false, nil
}

// QueryRealms is part of authdb.DB interface.
func (db *FakeDB) QueryRealms(ctx context.Context, id identity.Identity, perm realms.Permission, project string, attrs realms.Attrs) ([]string, error) {
	// This implicitly flips a flag forbidding registration of new permissions.
	// Presumably this should help catching "dynamic" permission registration
	// in tests, before it panics in production. We also need the result to check
	// UsedInQueryRealms flag.
	flags := realms.RegisteredPermissions()

	db.m.RLock()
	defer db.m.RUnlock()

	if db.err != nil {
		return nil, db.err
	}

	if project != "" {
		if err := realms.ValidateProjectName(project); err != nil {
			return nil, err
		}
	}

	if flags[perm]&realms.UsedInQueryRealms == 0 {
		return nil, errors.Reason("permission %s cannot be used in QueryRealms: it was not flagged with UsedInQueryRealms flag", perm).Err()
	}

	var out []string
	if mocked := db.perID[id]; mocked != nil {
		for _, mockedPerm := range mocked.perms {
			if mockedPerm.perm == perm && mockedPerm.cond(attrs) {
				if realmProj, _ := realms.Split(mockedPerm.realm); project == "" || project == realmProj {
					out = append(out, mockedPerm.realm)
				}
			}
		}
	}

	// The result in production in inherently unordered, since ordering it takes
	// time and in many applications the order doesn't really matter, so always
	// sorting it is wasteful. Simulate this behavior in tests too. If callers of
	// QueryRealms want the result ordered, they should sort it themselves.
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })

	return out, nil
}

// FilterKnownGroups is part of authdb.DB interface.
func (db *FakeDB) FilterKnownGroups(ctx context.Context, groups []string) ([]string, error) {
	db.m.RLock()
	defer db.m.RUnlock()

	if db.err != nil {
		return nil, db.err
	}

	var filtered []string
	for _, gr := range groups {
		if db.groups.Has(gr) {
			filtered = append(filtered, gr)
		}
	}
	return filtered, nil
}

// IsAllowedOAuthClientID is part of authdb.DB interface.
func (db *FakeDB) IsAllowedOAuthClientID(ctx context.Context, email, clientID string) (bool, error) {
	return true, nil
}

// IsInternalService is part of authdb.DB interface.
func (db *FakeDB) IsInternalService(ctx context.Context, hostname string) (bool, error) {
	return false, nil
}

// GetCertificates is part of authdb.DB interface.
func (db *FakeDB) GetCertificates(ctx context.Context, id identity.Identity) (*signing.PublicCertificates, error) {
	return nil, fmt.Errorf("GetCertificates is not implemented by FakeDB")
}

// GetAllowlistForIdentity is part of authdb.DB interface.
func (db *FakeDB) GetAllowlistForIdentity(ctx context.Context, ident identity.Identity) (string, error) {
	return "", nil
}

// IsAllowedIP is part of authdb.DB interface.
func (db *FakeDB) IsAllowedIP(ctx context.Context, ip net.IP, allowlist string) (bool, error) {
	db.m.RLock()
	defer db.m.RUnlock()
	if db.err != nil {
		return false, db.err
	}
	return db.ips[ip.String()].Has(allowlist), nil
}

// GetAuthServiceURL is part of authdb.DB interface.
func (db *FakeDB) GetAuthServiceURL(ctx context.Context) (string, error) {
	return "", fmt.Errorf("GetAuthServiceURL is not implemented by FakeDB")
}

// GetTokenServiceURL is part of authdb.DB interface.
func (db *FakeDB) GetTokenServiceURL(ctx context.Context) (string, error) {
	return "", fmt.Errorf("GetTokenServiceURL is not implemented by FakeDB")
}

// GetRealmData is part of authdb.DB interface.
func (db *FakeDB) GetRealmData(ctx context.Context, realm string) (*protocol.RealmData, error) {
	db.m.RLock()
	defer db.m.RUnlock()
	return db.realmData[realm], nil
}

// addGroup adds a group to the list of known groups.
func (db *FakeDB) addGroup(group string) {
	if db.groups == nil {
		db.groups = stringset.New(1)
	}
	db.groups.Add(group)
}

// mockedForID returns db.perID[id], initializing it if necessary.
//
// Called under the write lock.
func (db *FakeDB) mockedForID(id identity.Identity) *mockedForID {
	m, ok := db.perID[id]
	if !ok {
		m = &mockedForID{groups: stringset.New(0)}
		if db.perID == nil {
			db.perID = make(map[identity.Identity]*mockedForID, 1)
		}
		db.perID[id] = m
	}
	return m
}
