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
	"go.chromium.org/luci/server/auth/signing"
)

// DB is interface to access a database of authorization related information.
//
// It is static read only object that represent snapshot of auth data at some
// moment in time.
type DB interface {
	// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
	// to authenticate access for given email.
	IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error)

	// IsInternalService returns true if the given hostname belongs to a service
	// that is a part of the current LUCI deployment.
	//
	// What hosts are internal is controlled by 'internal_service_regexp' setting
	// in security.cfg in the Auth Service configs.
	IsInternalService(c context.Context, hostname string) (bool, error)

	// IsMember returns true if the given identity belongs to any of the groups.
	//
	// Unknown groups are considered empty. May return errors if underlying
	// datastore has issues.
	IsMember(c context.Context, id identity.Identity, groups []string) (bool, error)

	// CheckMembership returns groups from the given list the identity belongs to.
	//
	// Unlike IsMember, it doesn't stop on the first hit but continues evaluating
	// all groups.
	//
	// Unknown groups are considered empty. The order of groups in the result may
	// be different from the order in 'groups'.
	//
	// May return errors if underlying datastore has issues.
	CheckMembership(c context.Context, id identity.Identity, groups []string) ([]string, error)

	// GetCertificates returns a bundle with certificates of a trusted signer.
	//
	// Returns (nil, nil) if the given signer is not trusted.
	//
	// Returns errors (usually transient) if the bundle can't be fetched.
	GetCertificates(c context.Context, id identity.Identity) (*signing.PublicCertificates, error)

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

	// GetAuthServiceURL returns root URL ("https://<host>") of the auth service.
	//
	// Returns an error if the DB implementation is not using an auth service.
	GetAuthServiceURL(c context.Context) (string, error)

	// GetTokenServiceURL returns root URL ("https://<host>") of the token server.
	//
	// Returns an error if the DB implementation doesn't know how to retrieve it.
	//
	// Returns ("", nil) if the token server URL is not configured.
	GetTokenServiceURL(c context.Context) (string, error)
}
