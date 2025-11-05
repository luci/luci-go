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
	"net"

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth/authdb/internal/certs"
	"go.chromium.org/luci/server/auth/authdb/internal/graph"
	"go.chromium.org/luci/server/auth/authdb/internal/ipaddr"
	"go.chromium.org/luci/server/auth/authdb/internal/oauthid"
	"go.chromium.org/luci/server/auth/authdb/internal/realmset"
	"go.chromium.org/luci/server/auth/authdb/internal/seccfg"
	"go.chromium.org/luci/server/auth/internal/tracing"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
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
	realms         *realmset.Realms       // queryable representation of realms
	clientIDs      oauthid.Allowlist      // set of allowed client IDs
	allowlistedIPs ipaddr.Allowlist       // set of named IP allowlist
	securityCfg    *seccfg.SecurityConfig // parsed SecurityConfig proto

	tokenServiceURL   string        // URL of the token server as provided by Auth service
	tokenServiceCerts *certs.Bundle // cached public keys of the token server
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
	blob, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Fmt("failed to read the file: %w", err)
	}
	msg := &protocol.AuthDB{}
	if err := prototext.Unmarshal(blob, msg); err != nil {
		return nil, errors.Fmt("not a valid AuthDB text proto file: %w", err)
	}
	db, err := NewSnapshotDB(msg, "", 0, true)
	if err != nil {
		return nil, errors.Fmt("failed to validate AuthDB: %w", err)
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
		if err := ValidateAuthDB(authDB); err != nil {
			return nil, err
		}
	}

	groups, err := graph.BuildQueryable(authDB.Groups)
	if err != nil {
		return nil, errors.Fmt("failed to build groups graph: %w", err)
	}

	var realmSet *realmset.Realms
	if authDB.Realms != nil {
		realmSet, err = realmset.Build(authDB.Realms, groups, realms.RegisteredPermissions())
		if err != nil {
			return nil, errors.Fmt("failed to prepare Realms DB: %w", err)
		}
	}

	allowlistedIPs, err := ipaddr.NewAllowlist(authDB.IpWhitelists, authDB.IpWhitelistAssignments)
	if err != nil {
		return nil, errors.Fmt("bad IP allowlist in AuthDB: %w", err)
	}

	securityCfg, err := seccfg.Parse(authDB.SecurityConfig)
	if err != nil {
		return nil, errors.Fmt("bad SecurityConfig: %w", err)
	}

	return &SnapshotDB{
		AuthServiceURL:    authServiceURL,
		Rev:               rev,
		groups:            groups,
		realms:            realmSet,
		clientIDs:         oauthid.NewAllowlist(authDB.OauthClientId, authDB.OauthAdditionalClientIds),
		allowlistedIPs:    allowlistedIPs,
		securityCfg:       securityCfg,
		tokenServiceURL:   authDB.TokenServerUrl,
		tokenServiceCerts: &certs.Bundle{ServiceURL: authDB.TokenServerUrl},
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
func (db *SnapshotDB) IsInternalService(ctx context.Context, hostname string) (bool, error) {
	if db.securityCfg != nil {
		return db.securityCfg.IsInternalService(hostname), nil
	}
	return false, nil
}

// IsMember returns true if the given identity belongs to any of the groups.
//
// Unknown groups are considered empty, but are logged as warnings.
// May return errors if underlying datastore has issues.
func (db *SnapshotDB) IsMember(ctx context.Context, id identity.Identity, groups []string) (ok bool, err error) {
	_, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth/authdb.IsMember",
		attribute.StringSlice("cr.dev.groups", groups),
	)
	defer func() { tracing.End(span, err, attribute.Bool("cr.dev.outcome", ok)) }()

	if db.groups == nil {
		return false, nil
	}

	// TODO(vadimsh): Optimize multi-group case.
	for _, gr := range groups {
		switch db.groups.IsMember(id, gr) {
		case graph.IdentIsMember:
			return true, nil
		case graph.GroupIsUnknown:
			logging.Warningf(ctx, "Group %q is unknown in auth db snapshot %d", gr, db.Rev)
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
func (db *SnapshotDB) CheckMembership(ctx context.Context, id identity.Identity, groups []string) (out []string, err error) {
	_, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth/authdb.CheckMembership",
		attribute.StringSlice("cr.dev.groups", groups),
	)
	defer func() { tracing.End(span, err, attribute.StringSlice("cr.dev.outcome", out)) }()

	if db.groups == nil {
		return
	}

	// TODO(vadimsh): Optimize multi-group case.
	for _, gr := range groups {
		switch db.groups.IsMember(id, gr) {
		case graph.IdentIsMember:
			out = append(out, gr)
		case graph.GroupIsUnknown:
			logging.Warningf(ctx, "Group %q is unknown in auth db snapshot %d", gr, db.Rev)
		}
	}
	return
}

// HasPermission returns true if the identity has the given permission in the
// realm.
func (db *SnapshotDB) HasPermission(ctx context.Context, id identity.Identity, perm realms.Permission, realm string, attrs realms.Attrs) (ok bool, err error) {
	otelAttrs := append(make([]attribute.KeyValue, 0, 2+len(attrs)),
		attribute.String("cr.dev.permission", perm.Name()),
		attribute.String("cr.dev.realm", realm),
	)
	for k, v := range attrs {
		otelAttrs = append(otelAttrs, attribute.String("cr.dev.attr."+k, v))
	}
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth/authdb.HasPermission",
		otelAttrs...,
	)
	defer func() { tracing.End(span, err, attribute.Bool("cr.dev.outcome", ok)) }()

	// This may happen if the AuthDB proto has no Realms yet.
	if db.realms == nil {
		return false, errors.New("Realms API is not available")
	}

	permIdx, ok := db.realms.PermissionIndex(perm)
	if !ok {
		logging.Warningf(ctx, "Checking permission %q not present in the AuthDB", perm)
		return false, nil
	}

	// Verify such realm is defined in the DB or fallback to its @root.
	if !db.realms.HasRealm(realm) {
		if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
			return false, errors.Fmt("when checking %q: %w", perm, err)
		}
		project, name := realms.Split(realm)
		root := realms.Join(project, realms.RootRealm)
		if realm == root {
			logging.Warningf(ctx, "Checking %q in a non-existing root realm %q: denying", perm, realm)
			return false, nil
		}
		if !db.realms.HasRealm(root) {
			logging.Warningf(ctx, "Checking %q in a non-existing realm %q that doesn't have a root realm (no such project?): denying", perm, realm)
			return false, nil
		}
		// Don't log @legacy => @root fallbacks, they are semi-expected.
		if name != realms.LegacyRealm {
			logging.Warningf(ctx, "Checking %q in a non-existing realm %q: falling back to the root realm %q", perm, realm, root)
		}
		realm = root
	}

	// Grab the list of bindings for this permission and check if any applies to
	// the `id` based on its group memberships.
	q := db.groups.MembershipsQueryCache(id)
	return db.realms.Bindings(realm, permIdx).Check(ctx, &q, attrs), nil
}

// QueryRealms returns a list of realms where the identity has the given
// permission.
func (db *SnapshotDB) QueryRealms(ctx context.Context, id identity.Identity, perm realms.Permission, project string, attrs realms.Attrs) (out []string, err error) {
	otelAttrs := append(make([]attribute.KeyValue, 0, 2+len(attrs)),
		attribute.String("cr.dev.permission", perm.Name()),
		attribute.String("cr.dev.project", project),
	)
	for k, v := range attrs {
		otelAttrs = append(otelAttrs, attribute.String("cr.dev.attr."+k, v))
	}
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth/authdb.QueryRealms",
		otelAttrs...,
	)
	// `out` list can be huge. Just report the number of realms.
	defer func() { tracing.End(span, err, attribute.Int("cr.dev.outcome", len(out))) }()

	if project != "" {
		if err := realms.ValidateProjectName(project); err != nil {
			return nil, err
		}
	}

	// This may happen if the AuthDB proto has no Realms yet.
	if db.realms == nil {
		return nil, errors.New("Realms API is not available")
	}

	permIdx, ok := db.realms.PermissionIndex(perm)
	if !ok {
		logging.Warningf(ctx, "Querying realms with permission %q not present in the AuthDB", perm)
		return nil, nil
	}

	// Get the map project => all bindings for the given permission there. This
	// returns `ok == false` if the permission was not flagged with
	// UsedInQueryRealms.
	permBindings, ok := db.realms.QueryBindings(permIdx)
	if !ok {
		return nil, errors.Fmt("permission %s cannot be used in QueryRealms: it was not flagged with UsedInQueryRealms flag", perm)
	}

	// For each potentially matching list of bindings, check if it really matches.
	q := db.groups.MembershipsQueryCache(id)
	visit := func(bindings []realmset.RealmBindings) {
		for _, realmBindings := range bindings {
			if realmBindings.Bindings.Check(ctx, &q, attrs) {
				out = append(out, realmBindings.Realm)
			}
		}
	}
	if project != "" {
		visit(permBindings[project])
	} else {
		for _, bindings := range permBindings {
			visit(bindings)
		}
	}

	return out, nil
}

// FilterKnownGroups filters the list of groups keeping only ones that exist.
//
// May return errors if underlying datastore has issues. If all groups are
// unknown, returns an empty list and no error.
func (db *SnapshotDB) FilterKnownGroups(ctx context.Context, groups []string) (known []string, err error) {
	_, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth/authdb.FilterKnownGroups",
		attribute.Int("cr.dev.groups", len(groups)),
	)
	defer func() { tracing.End(span, err, attribute.Int("cr.dev.outcome", len(known))) }()

	if db.groups == nil {
		return nil, nil
	}

	known = make([]string, 0, len(groups))
	for _, gr := range groups {
		if _, ok := db.groups.GroupIndex(gr); ok {
			known = append(known, gr)
		}
	}
	return known, nil
}

// GetCertificates returns a bundle with certificates of a trusted signer.
//
// Currently only the Token Server is a trusted signer.
func (db *SnapshotDB) GetCertificates(ctx context.Context, signerID identity.Identity) (*signing.PublicCertificates, error) {
	if db.tokenServiceURL == "" {
		logging.Warningf(
			ctx, "Delegation is not supported, the token server URL is not set by %s",
			db.AuthServiceURL)
		return nil, nil
	}
	switch tokenServerID, certs, err := db.tokenServiceCerts.GetCerts(ctx); {
	case err != nil:
		return nil, err
	case signerID != tokenServerID:
		return nil, nil // signerID is not trusted since it's not a token server
	default:
		return certs, nil
	}
}

// GetAllowlistForIdentity returns name of the IP allowlist to use to check
// IP of requests from the given `ident`.
//
// It's used to restrict access for certain account to certain IP subnets.
//
// Returns ("", nil) if `ident` is not IP restricted.
func (db *SnapshotDB) GetAllowlistForIdentity(ctx context.Context, ident identity.Identity) (string, error) {
	return db.allowlistedIPs.GetAllowlistForIdentity(ident), nil
}

// IsAllowedIP returns true if IP address belongs to given named IP allowlist.
func (db *SnapshotDB) IsAllowedIP(ctx context.Context, ip net.IP, allowlist string) (bool, error) {
	return db.allowlistedIPs.IsAllowedIP(ip, allowlist), nil
}

// GetAuthServiceURL returns root URL ("https://<host>") of the auth service
// the snapshot was fetched from.
//
// This is needed to implement authdb.DB interface.
func (db *SnapshotDB) GetAuthServiceURL(ctx context.Context) (string, error) {
	if db.AuthServiceURL == "" {
		return "", errors.New("not using Auth Service")
	}
	return db.AuthServiceURL, nil
}

// GetTokenServiceURL returns root URL ("https://<host>") of the token server.
//
// This is needed to implement authdb.DB interface.
func (db *SnapshotDB) GetTokenServiceURL(ctx context.Context) (string, error) {
	return db.tokenServiceURL, nil
}

// GetRealmData returns data attached to a realm.
func (db *SnapshotDB) GetRealmData(ctx context.Context, realm string) (*protocol.RealmData, error) {
	// This may happen if the AuthDB proto has no Realms yet.
	if db.realms == nil {
		return nil, errors.New("Realms API is not available")
	}

	// Verify such realm is defined in the DB or fallback to its @root.
	if !db.realms.HasRealm(realm) {
		if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
			return nil, err
		}
		project, _ := realms.Split(realm)
		root := realms.Join(project, realms.RootRealm)
		if realm == root || !db.realms.HasRealm(root) {
			return nil, nil // no such project or it doesn't have realms.cfg
		}
		realm = root
	}

	data := db.realms.Data(realm)
	if data == nil {
		data = &protocol.RealmData{}
	}
	return data, nil
}
