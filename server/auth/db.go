// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/lazyslot"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/mathrand"

	"github.com/luci/luci-go/server/secrets"

	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/service/protocol"
	"github.com/luci/luci-go/server/auth/signing"
)

// DB is interface to access a database of authorization related information.
//
// It is static read only object that represent snapshot of auth data at some
// moment in time.
type DB interface {
	// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
	// to authenticate access for given email.
	IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error)

	// IsMember returns true if the given identity belongs to the given group.
	//
	// Unknown groups are considered empty. May return errors if underlying
	// datastore has issues.
	IsMember(c context.Context, id identity.Identity, group string) (bool, error)

	// SharedSecrets is secrets.Store with secrets in Auth DB.
	//
	// Such secrets are usually generated on central Auth Service and are known
	// to all trusted services (so that they can use them to exchange data).
	SharedSecrets(c context.Context) (secrets.Store, error)

	// GetWhitelistForIdentity returns name of the IP whitelist to use to check
	// IP of requests from given `ident`.
	//
	// It's used to restrict access for certain account to certain IP subnets.
	//
	// Returns ("", nil) if `ident` is not IP restricted.
	GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error)

	// IsInWhitelist returns true if IP address belongs to given named
	// IP whitelist.
	//
	// IP whitelist is a set of IP subnets. Unknown IP whitelists are considered
	// empty. May return errors if underlying datastore has issues.
	IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error)

	// GetAuthServiceCertificates returns a bundle with certificates of a primary
	// auth service.
	//
	// They can be used to verify signature of messages signed by the primary's
	// private key. Used for validating various tokens.
	GetAuthServiceCertificates(c context.Context) (*signing.PublicCertificates, error)
}

// DBProvider fetches a most recent DB instance.
//
// The returned instance is static and represents a snapshot of DB at some
// point.
//
// It's part of auth library global config, see Config.
type DBProvider func(c context.Context) (DB, error)

// DBCacheUpdater knows how to update local in-memory copy of DB.
//
// Used by NewDBCache.
type DBCacheUpdater func(c context.Context, prev DB) (DB, error)

// NewDBCache returns a factory of DB instances that uses local memory to
// cache DB instances for 5-10 seconds. It uses supplied callback to refetch DB
// from some permanent storage when cache expires.
//
// Even though the return value is technically a function, treat it as a heavy
// stateful object, since it has the cache of DB in its closure.
func NewDBCache(updater DBCacheUpdater) DBProvider {
	cacheSlot := lazyslot.Slot{
		Fetcher: func(c context.Context, prev lazyslot.Value) (lazyslot.Value, error) {
			var prevDB DB
			if prev.Value != nil {
				prevDB = prev.Value.(DB)
			}
			newDB, err := updater(c, prevDB)
			if err != nil {
				return lazyslot.Value{}, err
			}
			expTime := 5*time.Second + time.Duration(mathrand.Get(c).Intn(5000))*time.Millisecond
			return lazyslot.Value{
				Value:      newDB,
				Expiration: clock.Now(c).Add(expTime),
			}, nil
		},
	}

	// Actual factory that just grabs DB from the cache (triggering lazy refetch).
	return func(c context.Context) (DB, error) {
		val, err := cacheSlot.Get(c)
		if err != nil {
			return nil, err
		}
		return val.Value.(DB), nil
	}
}

// ErroringDB implements DB by forbidding all access and returning errors.
type ErroringDB struct {
	Error error // returned by all calls
}

// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
// to authenticate access for given email.
func (db ErroringDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	logging.Errorf(c, "%s", db.Error)
	return false, db.Error
}

// IsMember returns true if the given identity belongs to the given group.
//
// Unknown groups are considered empty. May return errors if underlying
// datastore has issues.
func (db ErroringDB) IsMember(c context.Context, id identity.Identity, group string) (bool, error) {
	logging.Errorf(c, "%s", db.Error)
	return false, db.Error
}

// SharedSecrets is secrets.Store with secrets in Auth DB.
//
// Such secrets are usually generated on central Auth Service and are known
// to all trusted services (so that they can use them to exchange data).
func (db ErroringDB) SharedSecrets(c context.Context) (secrets.Store, error) {
	logging.Errorf(c, "%s", db.Error)
	return nil, db.Error
}

// GetWhitelistForIdentity returns name of the IP whitelist to use to check
// IP of requests from given `ident`.
//
// It's used to restrict access for certain account to certain IP subnets.
//
// Returns ("", nil) if `ident` is not IP restricted.
func (db ErroringDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	logging.Errorf(c, "%s", db.Error)
	return "", db.Error
}

// IsInWhitelist returns true if IP address belongs to given named IP whitelist.
//
// IP whitelist is a set of IP subnets. Unknown IP whitelists are considered
// empty. May return errors if underlying datastore has issues.
func (db ErroringDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	logging.Errorf(c, "%s", db.Error)
	return false, db.Error
}

// GetAuthServiceCertificates returns a bundle with certificates of a primary
// auth service.
//
// They can be used to verify signature of messages signed by the primary's
// private key. Used for validating various tokens.
func (db ErroringDB) GetAuthServiceCertificates(c context.Context) (*signing.PublicCertificates, error) {
	logging.Errorf(c, "%s", db.Error)
	return nil, db.Error
}

///

// OAuth client_id of https://apis-explorer.appspot.com/.
const googleAPIExplorerClientID = "292824132082.apps.googleusercontent.com"

// SnapshotDB implements DB using AuthDB proto message.
//
// Use NewSnapshotDB to create new instances. Don't touch public fields
// of existing instances.
type SnapshotDB struct {
	AuthServiceURL string // where it was fetched from
	Rev            int64  // its revision number

	clientIDs map[string]struct{} // set of allowed client IDs
	groups    map[string]*group   // map of all known groups
	secrets   secrets.StaticStore // secrets shared by all service with this DB

	assignments map[identity.Identity]string // IP whitelist assignements
	whitelists  map[string][]net.IPNet       // IP whitelists
}

// group is a node in a group graph. Nested groups are referenced directly via
// pointer.
type group struct {
	members map[identity.Identity]struct{} // set of all members
	globs   []identity.Glob                // list of all identity globs
	nested  []*group                       // pointers to nested groups
}

var _ DB = &SnapshotDB{}

// NewSnapshotDB creates new instance of SnapshotDB.
//
// It does some preprocessing to speed up subsequent checks. Return errors if
// it encounters inconsistencies.
func NewSnapshotDB(authDB *protocol.AuthDB, authServiceURL string, rev int64) (*SnapshotDB, error) {
	db := &SnapshotDB{
		AuthServiceURL: authServiceURL,
		Rev:            rev,
	}

	// Set of all allowed clientIDs.
	db.clientIDs = make(map[string]struct{}, 2+len(authDB.GetOauthAdditionalClientIds()))
	db.clientIDs[googleAPIExplorerClientID] = struct{}{}
	if authDB.GetOauthClientId() != "" {
		db.clientIDs[authDB.GetOauthClientId()] = struct{}{}
	}
	for _, cid := range authDB.GetOauthAdditionalClientIds() {
		if cid != "" {
			db.clientIDs[cid] = struct{}{}
		}
	}

	// First pass: build all `group` nodes.
	db.groups = make(map[string]*group, len(authDB.GetGroups()))
	for _, g := range authDB.GetGroups() {
		if db.groups[g.GetName()] != nil {
			return nil, fmt.Errorf("auth: bad AuthDB, group %q is listed twice", g.GetName())
		}
		gr := &group{}
		if len(g.GetMembers()) != 0 {
			gr.members = make(map[identity.Identity]struct{}, len(g.GetMembers()))
			for _, ident := range g.GetMembers() {
				gr.members[identity.Identity(ident)] = struct{}{}
			}
		}
		if len(g.GetGlobs()) != 0 {
			gr.globs = make([]identity.Glob, len(g.GetGlobs()))
			for i, glob := range g.GetGlobs() {
				gr.globs[i] = identity.Glob(glob)
			}
		}
		if len(g.GetNested()) != 0 {
			gr.nested = make([]*group, 0, len(g.GetNested()))
		}
		db.groups[g.GetName()] = gr
	}

	// Second pass: fill in `nested` with pointers, now that we have them.
	for _, g := range authDB.GetGroups() {
		gr := db.groups[g.GetName()]
		for _, nestedName := range g.GetNested() {
			if nestedGroup := db.groups[nestedName]; nestedGroup != nil {
				gr.nested = append(gr.nested, nestedGroup)
			}
		}
	}

	// Load all shared secrets.
	db.secrets = make(secrets.StaticStore, len(authDB.GetSecrets()))
	for _, s := range authDB.GetSecrets() {
		values := s.GetValues()
		if len(values) == 0 {
			continue
		}
		secret := secrets.Secret{
			Current: secrets.NamedBlob{Blob: values[0]}, // most recent on top
		}
		if len(values) > 1 {
			secret.Previous = make([]secrets.NamedBlob, len(values)-1)
			for i := 1; i < len(values); i++ {
				secret.Previous[i-1] = secrets.NamedBlob{Blob: values[i]}
			}
		}
		db.secrets[secrets.Key(s.GetName())] = secret
	}

	// Build map of IP whitelist assignments.
	db.assignments = make(map[identity.Identity]string, len(authDB.GetIpWhitelistAssignments()))
	for _, a := range authDB.GetIpWhitelistAssignments() {
		db.assignments[identity.Identity(a.GetIdentity())] = a.GetIpWhitelist()
	}

	// Parse all subnets into IPNet objects.
	db.whitelists = make(map[string][]net.IPNet, len(authDB.GetIpWhitelists()))
	for _, w := range authDB.GetIpWhitelists() {
		if len(w.GetSubnets()) == 0 {
			continue
		}
		nets := make([]net.IPNet, len(w.GetSubnets()))
		for i, subnet := range w.GetSubnets() {
			_, ipnet, err := net.ParseCIDR(subnet)
			if err != nil {
				return nil, fmt.Errorf("auth: bad subnet %q in IP list %q - %s", subnet, w.GetName(), err)
			}
			nets[i] = *ipnet
		}
		db.whitelists[w.GetName()] = nets
	}

	return db, nil
}

// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
// to authenticate access for given email.
func (db *SnapshotDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	// No need to whitelist client IDs for service accounts, since email address
	// uniquely identifies credentials used. Note: this is Google specific.
	if strings.HasSuffix(email, ".gserviceaccount.com") {
		return true, nil
	}

	// clientID must be set for non service accounts.
	if clientID == "" {
		return false, nil
	}

	_, ok := db.clientIDs[clientID]
	return ok, nil
}

// IsMember returns true if the given identity belongs to the given group.
//
// Unknown groups are considered empty. May return errors if underlying
// datastore has issues.
func (db *SnapshotDB) IsMember(c context.Context, id identity.Identity, groupName string) (bool, error) {
	// Cycle detection check uses a stack of groups currently being explored. Use
	// stack allocated array as a backing store to avoid unnecessary dynamic
	// allocation. If stack depth grows beyond 8, 'append' will reallocate it on
	// the heap.
	var backingStore [8]*group
	current := backingStore[:0]

	// Keep a set of all visited groups to avoid revisiting them in case of a
	// diamond-like graph, e.g A -> B, A -> C, B -> D, C -> D (we don't need to
	// visit D twice in this case).
	visited := make(map[*group]struct{}, 10)

	// isMember is used to recurse over nested groups.
	var isMember func(*group) bool

	isMember = func(gr *group) bool {
		// 'id' is a direct member?
		if _, ok := gr.members[id]; ok {
			return true
		}

		// 'id' matches some glob?
		for _, glob := range gr.globs {
			if glob.Match(id) {
				return true
			}
		}

		if len(gr.nested) == 0 {
			return false
		}

		current = append(current, gr) // popped before return

		found := false

	outer_loop:
		for _, nested := range gr.nested {
			// There should be no cycles, but do the check just in case there are,
			// seg faulting with stack overflow is very bad. In case of a cycle, skip
			// the offending group, but keep searching other groups.
			for _, ancestor := range current {
				if ancestor == nested {
					logging.Errorf(c, "auth: unexpected group nesting cycle in group %q", groupName)
					continue outer_loop
				}
			}

			// Explored 'nested' already (and didn't find anything) while visiting
			// some sibling branch? Skip.
			if _, seen := visited[nested]; seen {
				continue
			}

			if isMember(nested) {
				found = true
				break
			}
		}

		// Note that we don't use defers here since they have non-negligible runtime
		// cost. Using 'defer' here makes IsMember ~1.7x slower (1200 ns vs 700 ns),
		// See BenchmarkIsMember.
		current = current[:len(current)-1]
		visited[gr] = struct{}{}

		return found
	}

	if gr := db.groups[groupName]; gr != nil {
		return isMember(gr), nil
	}
	return false, nil
}

// SharedSecrets is secrets.Store with secrets in Auth DB.
//
// Such secrets are usually generated on central Auth Service and are known
// to all trusted services (so that they can use them to exchange data).
func (db *SnapshotDB) SharedSecrets(c context.Context) (secrets.Store, error) {
	return db.secrets, nil
}

// GetWhitelistForIdentity returns name of the IP whitelist to use to check
// IP of requests from given `ident`.
//
// It's used to restrict access for certain account to certain IP subnets.
//
// Returns ("", nil) if `ident` is not IP restricted.
func (db *SnapshotDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	return db.assignments[ident], nil
}

// IsInWhitelist returns true if IP address belongs to given named IP whitelist.
//
// IP whitelist is a set of IP subnets. Unknown IP whitelists are considered
// empty. May return errors if underlying datastore has issues.
func (db *SnapshotDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	for _, ipnet := range db.whitelists[whitelist] {
		if ipnet.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}

// GetAuthServiceCertificates returns a bundle with certificates of a primary
// auth service.
//
// They can be used to verify signature of messages signed by the primary's
// private key. Used for validating various tokens.
func (db *SnapshotDB) GetAuthServiceCertificates(c context.Context) (*signing.PublicCertificates, error) {
	// Note: FetchCertificates does caching inside.
	return signing.FetchCertificates(c, db.AuthServiceURL)
}
