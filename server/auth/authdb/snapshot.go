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
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
)

// OAuth client_id of https://apis-explorer.appspot.com/.
const googleAPIExplorerClientID = "292824132082.apps.googleusercontent.com"

// SnapshotDB implements DB using AuthDB proto message.
//
// Use NewSnapshotDB to create new instances. Don't touch public fields
// of existing instances.
type SnapshotDB struct {
	AuthServiceURL string // where it was fetched from
	Rev            int64  // its revision number

	tokenServiceURL string // URL of the token server as provided by Auth service

	clientIDs map[string]struct{} // set of allowed client IDs
	groups    map[string]*group   // map of all known groups

	assignments map[identity.Identity]string // IP whitelist assignements
	whitelists  map[string][]net.IPNet       // IP whitelists

	// Certs are loaded lazily in GetCertificates since they are used only when
	// checking delegation tokens, which is relatively rare.
	certs lazyslot.Slot
}

var _ DB = &SnapshotDB{}

// group is a node in a group graph. Nested groups are referenced directly via
// pointer.
type group struct {
	members map[identity.Identity]struct{} // set of all members
	globs   []identity.Glob                // list of all identity globs
	nested  []*group                       // pointers to nested groups
}

// certMap is used in GetCertificate and fetchTrustedCerts.
type certMap map[identity.Identity]*signing.PublicCertificates

// Revision returns a revision of an auth DB or 0 if it can't be determined.
//
// It's just a small helper that casts db to *SnapshotDB and extracts the
// revision from there.
func Revision(db DB) int64 {
	if snap, _ := db.(*SnapshotDB); snap != nil {
		return snap.Rev
	}
	return 0
}

// NewSnapshotDB creates new instance of SnapshotDB.
//
// It does some preprocessing to speed up subsequent checks. Return errors if
// it encounters inconsistencies.
func NewSnapshotDB(authDB *protocol.AuthDB, authServiceURL string, rev int64) (*SnapshotDB, error) {
	db := &SnapshotDB{
		AuthServiceURL: authServiceURL,
		Rev:            rev,

		tokenServiceURL: authDB.GetTokenServerUrl(),
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

// IsMember returns true if the given identity belongs to any of the groups.
//
// Unknown groups are considered empty. May return errors if underlying
// datastore has issues.
func (db *SnapshotDB) IsMember(c context.Context, id identity.Identity, groups []string) (bool, error) {
	// TODO(vadimsh): Optimize multi-group case.
	for _, gr := range groups {
		switch found, err := db.isMemberImpl(c, id, gr); {
		case err != nil:
			return false, err
		case found:
			return true, nil
		}
	}
	return false, nil
}

// CheckMembership returns groups from the given list the identity belongs to.
//
// Unlike IsMember, it doesn't stop on the first hit but continues evaluating
// all groups.
//
// Unknown groups are considered empty. The order of groups in the result may
// be different from the order in 'groups'.
//
// May return errors if underlying datastore has issues.
func (db *SnapshotDB) CheckMembership(c context.Context, id identity.Identity, groups []string) (out []string, err error) {
	// TODO(vadimsh): Optimize multi-group case.
	for _, gr := range groups {
		switch found, err := db.isMemberImpl(c, id, gr); {
		case err != nil:
			return nil, err
		case found:
			out = append(out, gr)
		}
	}
	return
}

// isMemberImpl implements IsMember check for a single group only.
func (db *SnapshotDB) isMemberImpl(c context.Context, id identity.Identity, groupName string) (bool, error) {
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

// GetCertificates returns a bundle with certificates of a trusted signer.
func (db *SnapshotDB) GetCertificates(c context.Context, signerID identity.Identity) (*signing.PublicCertificates, error) {
	mapping, err := db.certs.Get(c, func(interface{}) (interface{}, time.Duration, error) {
		mapping, err := db.fetchTrustedCerts(c)
		return mapping, time.Hour, err
	})
	if err != nil {
		return nil, err
	}
	trustedCertsMap := mapping.(certMap)
	return trustedCertsMap[signerID], nil
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

// GetAuthServiceURL returns root URL ("https://<host>") of the auth service
// the snapshot was fetched from.
//
// This is needed to implement authdb.DB interface.
func (db *SnapshotDB) GetAuthServiceURL(c context.Context) (string, error) {
	return db.AuthServiceURL, nil
}

// GetTokenServiceURL returns root URL ("https://<host>") of the token server.
//
// This is needed to implement authdb.DB interface.
func (db *SnapshotDB) GetTokenServiceURL(c context.Context) (string, error) {
	return db.tokenServiceURL, nil
}

//// Implementation details.

// fetchTrustedCerts is called by GetCertificates to fetch certificates.
//
// We currently trust only the token server, as provided by the auth service.
func (db *SnapshotDB) fetchTrustedCerts(c context.Context) (certMap, error) {
	if db.tokenServiceURL == "" {
		logging.Warningf(
			c, "Delegation is not supported, the token server URL is not set by %s",
			db.AuthServiceURL)
		return certMap{}, nil
	}

	certs, err := signing.FetchCertificatesFromLUCIService(c, db.tokenServiceURL)
	if err != nil {
		return nil, err
	}
	if certs.ServiceAccountName == "" {
		return nil, fmt.Errorf("the token server %s didn't provide its service account name", db.tokenServiceURL)
	}

	id, err := identity.MakeIdentity("user:" + certs.ServiceAccountName)
	if err != nil {
		return nil, fmt.Errorf("invalid service_account_name %q in fetched certificates bundle - %s", certs.ServiceAccountName, err)
	}

	return certMap{id: certs}, nil
}
