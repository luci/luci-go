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
	"io"
	"io/ioutil"
	"net"
	"strings"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/server/auth/authdb/internal/certs"
	"go.chromium.org/luci/server/auth/authdb/internal/graph"
	"go.chromium.org/luci/server/auth/authdb/internal/ipaddr"
	"go.chromium.org/luci/server/auth/authdb/internal/oauthid"
	"go.chromium.org/luci/server/auth/authdb/internal/seccfg"
)

// SnapshotDB implements DB using AuthDB proto message.
//
// Use NewSnapshotDB to create new instances. Don't touch public fields
// of existing instances.
//
// Zero value represents an empty AuthDB.
type SnapshotDB struct {
	AuthServiceURL string // where it was fetched from
	Rev            int64  // its revision number

	groups         *graph.QueryableGraph  // queryable representation of groups
	clientIDs      oauthid.Whitelist      // set of allowed client IDs
	whitelistedIPs ipaddr.Whitelist       // set of named IP whitelists
	securityCfg    *seccfg.SecurityConfig // parsed SecurityConfig proto

	tokenServiceURL   string       // URL of the token server as provided by Auth service
	tokenServiceCerts certs.Bundle // cached public keys of the token server
}

var _ DB = &SnapshotDB{}

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

// SnapshotDBFromTextProto constructs SnapshotDB by loading it from a text proto
// with AuthDB message.
func SnapshotDBFromTextProto(r io.Reader) (*SnapshotDB, error) {
	blob, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read the file").Err()
	}
	msg := &protocol.AuthDB{}
	if err := proto.UnmarshalText(string(blob), msg); err != nil {
		return nil, errors.Annotate(err, "not a valid AuthDB text proto file").Err()
	}
	db, err := NewSnapshotDB(msg, "", 0, true)
	if err != nil {
		return nil, errors.Annotate(err, "failed to validate AuthDB").Err()
	}
	return db, nil
}

// NewSnapshotDB creates new instance of SnapshotDB.
//
// It does some preprocessing to speed up subsequent checks. Returns errors if
// it encounters inconsistencies.
//
// If 'validate' is false, skips some expensive validation steps, assuming they
// were performed before, when AuthDB was initially received.
func NewSnapshotDB(authDB *protocol.AuthDB, authServiceURL string, rev int64, validate bool) (*SnapshotDB, error) {
	if validate {
		if err := validateAuthDB(authDB); err != nil {
			return nil, err
		}
	}

	groups, err := graph.BuildQueryable(authDB.Groups)
	if err != nil {
		return nil, errors.Annotate(err, "failed to build groups graph").Err()
	}

	ipWL, err := ipaddr.NewWhitelist(authDB.IpWhitelists, authDB.IpWhitelistAssignments)
	if err != nil {
		return nil, errors.Annotate(err, "bad IP whitelists in AuthDB").Err()
	}

	securityCfg, err := seccfg.Parse(authDB.SecurityConfig)
	if err != nil {
		return nil, errors.Annotate(err, "bad SecurityConfig").Err()
	}

	return &SnapshotDB{
		AuthServiceURL:    authServiceURL,
		Rev:               rev,
		groups:            groups,
		clientIDs:         oauthid.NewWhitelist(authDB.OauthClientId, authDB.OauthAdditionalClientIds),
		whitelistedIPs:    ipWL,
		securityCfg:       securityCfg,
		tokenServiceURL:   authDB.TokenServerUrl,
		tokenServiceCerts: certs.Bundle{ServiceURL: authDB.TokenServerUrl},
	}, nil
}

// IsAllowedOAuthClientID returns true if the given OAuth2 client ID can be used
// to authorize access from the given email.
func (db *SnapshotDB) IsAllowedOAuthClientID(_ context.Context, email, clientID string) (bool, error) {
	return db.clientIDs.IsAllowedOAuthClientID(email, clientID), nil
}

// IsInternalService returns true if the given hostname belongs to a service
// that is a part of the current LUCI deployment.
//
// What hosts are internal is controlled by 'internal_service_regexp' setting
// in security.cfg in the Auth Service configs.
func (db *SnapshotDB) IsInternalService(c context.Context, hostname string) (bool, error) {
	if db.securityCfg != nil {
		return db.securityCfg.IsInternalService(hostname), nil
	}
	return false, nil
}

// IsMember returns true if the given identity belongs to any of the groups.
//
// Unknown groups are considered empty. May return errors if underlying
// datastore has issues.
func (db *SnapshotDB) IsMember(c context.Context, id identity.Identity, groups []string) (bool, error) {
	if db.groups == nil {
		return false, nil
	}

	_, span := trace.StartSpan(c, "go.chromium.org/luci/server/auth/authdb.IsMember")
	span.Attribute("cr.dev/groups", strings.Join(groups, ", "))
	defer span.End(nil)

	// TODO(vadimsh): Optimize multi-group case.
	for _, gr := range groups {
		if db.groups.IsMember(id, gr) {
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
	if db.groups == nil {
		return
	}

	_, span := trace.StartSpan(c, "go.chromium.org/luci/server/auth/authdb.CheckMembership")
	span.Attribute("cr.dev/groups", strings.Join(groups, ", "))
	defer span.End(nil)

	// TODO(vadimsh): Optimize multi-group case.
	for _, gr := range groups {
		if db.groups.IsMember(id, gr) {
			out = append(out, gr)
		}
	}
	return
}

// HasPermission returns true if the identity has the given permission in any
// of the realms.
func (db *SnapshotDB) HasPermission(c context.Context, id identity.Identity, perm realms.Permission, realms []string) (bool, error) {
	_, span := trace.StartSpan(c, "go.chromium.org/luci/server/auth/authdb.HasPermission")
	span.Attribute("cr.dev/permission", perm.Name())
	span.Attribute("cr.dev/realms", strings.Join(realms, ", "))
	defer span.End(nil)

	// TODO(vadimsh): Implement.

	return false, errors.Reason("HasPermission is not implemented yet").Err()
}

// GetCertificates returns a bundle with certificates of a trusted signer.
//
// Currently only the Token Server is a trusted signer.
func (db *SnapshotDB) GetCertificates(c context.Context, signerID identity.Identity) (*signing.PublicCertificates, error) {
	if db.tokenServiceURL == "" {
		logging.Warningf(
			c, "Delegation is not supported, the token server URL is not set by %s",
			db.AuthServiceURL)
		return nil, nil
	}
	switch tokenServerID, certs, err := db.tokenServiceCerts.GetCerts(c); {
	case err != nil:
		return nil, err
	case signerID != tokenServerID:
		return nil, nil // signerID is not trusted since it's not a token server
	default:
		return certs, nil
	}
}

// GetWhitelistForIdentity returns name of the IP whitelist to use to check
// IP of requests from given `ident`.
//
// It's used to restrict access for certain account to certain IP subnets.
//
// Returns ("", nil) if `ident` is not IP restricted.
func (db *SnapshotDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	return db.whitelistedIPs.GetWhitelistForIdentity(ident), nil
}

// IsInWhitelist returns true if IP address belongs to given named IP whitelist.
//
// IP whitelist is a set of IP subnets. Unknown IP whitelists are considered
// empty. May return errors if underlying datastore has issues.
func (db *SnapshotDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	return db.whitelistedIPs.IsInWhitelist(ip, whitelist), nil
}

// GetAuthServiceURL returns root URL ("https://<host>") of the auth service
// the snapshot was fetched from.
//
// This is needed to implement authdb.DB interface.
func (db *SnapshotDB) GetAuthServiceURL(c context.Context) (string, error) {
	if db.AuthServiceURL == "" {
		return "", errors.Reason("not using Auth Service").Err()
	}
	return db.AuthServiceURL, nil
}

// GetTokenServiceURL returns root URL ("https://<host>") of the token server.
//
// This is needed to implement authdb.DB interface.
func (db *SnapshotDB) GetTokenServiceURL(c context.Context) (string, error) {
	return db.tokenServiceURL, nil
}
