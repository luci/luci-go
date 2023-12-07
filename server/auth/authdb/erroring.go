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
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
)

// ErroringDB implements DB by forbidding all access and returning errors.
type ErroringDB struct {
	Error error // returned by all calls
}

// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
// to authenticate access for given email.
func (db ErroringDB) IsAllowedOAuthClientID(ctx context.Context, email, clientID string) (bool, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return false, db.Error
}

// IsInternalService returns true if the given hostname belongs to a service
// that is a part of the current LUCI deployment.
func (db ErroringDB) IsInternalService(ctx context.Context, hostname string) (bool, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return false, db.Error
}

// IsMember returns true if the given identity belongs to any of the groups.
func (db ErroringDB) IsMember(ctx context.Context, id identity.Identity, groups []string) (bool, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return false, db.Error
}

// CheckMembership returns groups from the given list the identity belongs to.
func (db ErroringDB) CheckMembership(ctx context.Context, id identity.Identity, groups []string) ([]string, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return nil, db.Error
}

// HasPermission returns true if the identity has the given permission in any
// of the realms.
func (db ErroringDB) HasPermission(ctx context.Context, id identity.Identity, perm realms.Permission, realm string, attrs realms.Attrs) (bool, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return false, db.Error
}

// QueryRealms returns a list of realms where the identity has the given
// permission.
func (db ErroringDB) QueryRealms(ctx context.Context, id identity.Identity, perm realms.Permission, project string, attrs realms.Attrs) ([]string, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return nil, db.Error
}

// FilterKnownGroups filters the list of groups keeping only ones that exist.
func (db ErroringDB) FilterKnownGroups(ctx context.Context, groups []string) ([]string, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return nil, db.Error
}

// GetCertificates returns a bundle with certificates of a trusted signer.
func (db ErroringDB) GetCertificates(ctx context.Context, id identity.Identity) (*signing.PublicCertificates, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return nil, db.Error
}

// GetAllowlistForIdentity returns name of the IP allowlist to use to check
// IP of requests from the given `ident`.
func (db ErroringDB) GetAllowlistForIdentity(ctx context.Context, ident identity.Identity) (string, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return "", db.Error
}

// IsAllowedIP returns true if IP address belongs to given named IP allowlist.
func (db ErroringDB) IsAllowedIP(ctx context.Context, ip net.IP, allowlist string) (bool, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return false, db.Error
}

// GetAuthServiceURL returns root URL ("https://<host>") of the auth service.
func (db ErroringDB) GetAuthServiceURL(ctx context.Context) (string, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return "", db.Error
}

// GetTokenServiceURL returns root URL ("https://<host>") of the token service.
func (db ErroringDB) GetTokenServiceURL(ctx context.Context) (string, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return "", db.Error
}

// GetRealmData returns data attached to a realm.
func (db ErroringDB) GetRealmData(ctx context.Context, realm string) (*protocol.RealmData, error) {
	logging.Errorf(ctx, "%s", db.Error)
	return nil, db.Error
}
