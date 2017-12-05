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
	"net"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/signing"
)

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

// IsMember returns true if the given identity belongs to any of the groups.
func (db ErroringDB) IsMember(c context.Context, id identity.Identity, groups []string) (bool, error) {
	logging.Errorf(c, "%s", db.Error)
	return false, db.Error
}

// CheckMembership returns groups from the given list the identity belongs to.
func (db ErroringDB) CheckMembership(c context.Context, id identity.Identity, groups []string) ([]string, error) {
	logging.Errorf(c, "%s", db.Error)
	return nil, db.Error
}

// GetCertificates returns a bundle with certificates of a trusted signer.
func (db ErroringDB) GetCertificates(c context.Context, id identity.Identity) (*signing.PublicCertificates, error) {
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

// GetAuthServiceURL returns root URL ("https://<host>") of the auth service.
func (db ErroringDB) GetAuthServiceURL(c context.Context) (string, error) {
	return "", db.Error
}

// GetTokenServiceURL returns root URL ("https://<host>") of the token service.
func (db ErroringDB) GetTokenServiceURL(c context.Context) (string, error) {
	return "", db.Error
}
