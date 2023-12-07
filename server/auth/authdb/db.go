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
	"net"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
)

// AuthServiceAccessGroup members are allowed to see all groups.
const AuthServiceAccessGroup = "auth-service-access"

// DB is interface to access a database of authorization related information.
//
// It is static read only object that represent snapshot of auth data at some
// moment in time.
type DB interface {
	// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
	// to authenticate access for given email.
	IsAllowedOAuthClientID(ctx context.Context, email, clientID string) (bool, error)

	// IsInternalService returns true if the given hostname belongs to a service
	// that is a part of the current LUCI deployment.
	//
	// What hosts are internal is controlled by 'internal_service_regexp' setting
	// in security.cfg in the Auth Service configs.
	IsInternalService(ctx context.Context, hostname string) (bool, error)

	// IsMember returns true if the given identity belongs to any of the groups.
	//
	// Unknown groups are considered empty (but logged as warnings).
	// May return errors if underlying datastore has issues.
	IsMember(ctx context.Context, id identity.Identity, groups []string) (bool, error)

	// CheckMembership returns groups from the given list the identity belongs to.
	//
	// Unlike IsMember, it doesn't stop on the first hit but continues evaluating
	// all groups.
	//
	// Unknown groups are considered empty. The order of groups in the result may
	// be different from the order in 'groups'.
	//
	// May return errors if underlying datastore has issues.
	CheckMembership(ctx context.Context, id identity.Identity, groups []string) ([]string, error)

	// HasPermission returns true if the identity has the given permission in the
	// realm.
	//
	// A non-existing realm is replaced with the corresponding root realm (e.g. if
	// "projectA:some/realm" doesn't exist, "projectA:@root" will be used in its
	// place). If the project doesn't exist or is not using realms yet, all its
	// realms (including the root realm) are considered empty. HasPermission
	// returns false in this case.
	//
	// Attributes are the context of this particular permission check and are used
	// as inputs to `conditions` predicates in conditional bindings. If a service
	// supports conditional bindings, it must document what attributes it passes
	// with each permission it checks.
	//
	// Returns an error only if the check itself failed due to a misconfiguration
	// or transient issues. This should usually result in an Internal error.
	HasPermission(ctx context.Context, id identity.Identity, perm realms.Permission, realm string, attrs realms.Attrs) (bool, error)

	// QueryRealms returns a list of realms where the identity has the given
	// permission.
	//
	// If `project` is not empty, restricts the check only to the realms in this
	// project, otherwise checks all realms across all projects. Either way, the
	// returned realm names have form `<some-project>:<some-realm>`. The list is
	// returned in some arbitrary order.
	//
	// Semantically it is equivalent to visiting all explicitly defined realms
	// (plus "<project>:@root" and "<project>:@legacy") in the requested project
	// or all projects, and calling HasPermission(id, perm, realm, attr) for each
	// of  them.
	//
	// The permission `perm` should be flagged in the process with
	// UsedInQueryRealms flag, which lets the runtime know it must prepare indexes
	// for the corresponding QueryRealms call.
	//
	// Returns an error only if the check itself failed due to a misconfiguration
	// or transient issues. This should usually result in an Internal error.
	QueryRealms(ctx context.Context, id identity.Identity, perm realms.Permission, project string, attrs realms.Attrs) ([]string, error)

	// FilterKnownGroups filters the list of groups keeping only ones that exist.
	//
	// May return errors if underlying datastore has issues. If all groups are
	// unknown, returns an empty list and no error.
	FilterKnownGroups(ctx context.Context, groups []string) ([]string, error)

	// GetCertificates returns a bundle with certificates of a trusted signer.
	//
	// Returns (nil, nil) if the given signer is not trusted.
	//
	// Returns errors (usually transient) if the bundle can't be fetched.
	GetCertificates(ctx context.Context, id identity.Identity) (*signing.PublicCertificates, error)

	// GetAllowlistForIdentity returns name of the IP allowlist to use to check
	// IP of requests from given `ident`.
	//
	// It's used to restrict access for certain account to certain IP subnets.
	//
	// Returns ("", nil) if `ident` is not IP restricted.
	GetAllowlistForIdentity(ctx context.Context, ident identity.Identity) (string, error)

	// IsAllowedIP returns true if IP address belongs to given named IP allowlist.
	//
	// An IP allowlist is a set of IP subnets. Unknown allowlists are considered
	// empty. May return errors if underlying datastore has issues.
	IsAllowedIP(ctx context.Context, ip net.IP, allowlist string) (bool, error)

	// GetAuthServiceURL returns root URL ("https://<host>") of the auth service.
	//
	// Returns an error if the DB implementation is not using an auth service.
	GetAuthServiceURL(ctx context.Context) (string, error)

	// GetTokenServiceURL returns root URL ("https://<host>") of the token server.
	//
	// Returns an error if the DB implementation doesn't know how to retrieve it.
	//
	// Returns ("", nil) if the token server URL is not configured.
	GetTokenServiceURL(ctx context.Context) (string, error)

	// GetRealmData returns data attached to a realm.
	//
	// Falls back to the "@root" realm if `realm` doesn't exist. Returns nil if
	// the root realm doesn't exist either, which means that either project
	// doesn't exist or it has no realms.cfg file.
	//
	// Returns an error only if the check itself failed due to a misconfiguration
	// or transient issues. This should usually result in an Internal error.
	GetRealmData(ctx context.Context, realm string) (*protocol.RealmData, error)
}
