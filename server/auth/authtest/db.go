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
	"net"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/signing"
)

// FakeDB implements user group checking part of db.DB (IsMember).
//
// It is a mapping "identity -> list of its groups". Intended to be used mostly
// for testing request handlers, thus all other DB methods are hardcoded to
// implement some default behavior sufficient for fake requests to pass
// authentication.
type FakeDB map[identity.Identity][]string

var _ authdb.DB = (FakeDB)(nil)

// Use installs the fake db into the context.
func (db FakeDB) Use(c context.Context) context.Context {
	return auth.ModifyConfig(c, func(cfg auth.Config) auth.Config {
		cfg.DBProvider = func(context.Context) (authdb.DB, error) {
			return db, nil
		}
		return cfg
	})
}

// IsMember is part of authdb.DB interface.
//
// It returns true if any of 'groups' is listed in db[id].
func (db FakeDB) IsMember(c context.Context, id identity.Identity, groups []string) (bool, error) {
	hits, err := db.CheckMembership(c, id, groups)
	if err != nil {
		return false, err
	}
	return len(hits) > 0, nil
}

// CheckMembership is part of authdb.DB interface.
//
// It returns a list of groups the identity belongs to.
func (db FakeDB) CheckMembership(c context.Context, id identity.Identity, groups []string) (out []string, err error) {
	belongsTo := stringset.NewFromSlice(db[id]...)
	for _, g := range groups {
		if belongsTo.Has(g) {
			out = append(out, g)
		}
	}
	return
}

// IsAllowedOAuthClientID is part of authdb.DB interface.
func (db FakeDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	return true, nil
}

// IsInternalService is part of authdb.DB interface.
func (db FakeDB) IsInternalService(c context.Context, hostname string) (bool, error) {
	return false, nil
}

// GetCertificates is part of authdb.DB interface.
func (db FakeDB) GetCertificates(c context.Context, id identity.Identity) (*signing.PublicCertificates, error) {
	return nil, fmt.Errorf("GetCertificates is not implemented by FakeDB")
}

// GetWhitelistForIdentity is part of authdb.DB interface.
func (db FakeDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	return "", nil
}

// IsInWhitelist is part of authdb.DB interface.
func (db FakeDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	return false, nil
}

// GetAuthServiceURL is part of authdb.DB interface.
func (db FakeDB) GetAuthServiceURL(c context.Context) (string, error) {
	return "", fmt.Errorf("GetAuthServiceURL is not implemented by FakeDB")
}

// GetTokenServiceURL is part of authdb.DB interface.
func (db FakeDB) GetTokenServiceURL(c context.Context) (string, error) {
	return "", fmt.Errorf("GetTokenServiceURL is not implemented by FakeDB")
}

// FakeErroringDB is authdb.DB with IsMember returning an error.
type FakeErroringDB struct {
	FakeDB

	// Error is returned by IsMember.
	Error error
}

// IsMember is part of authdb.DB interface.
//
// It returns db.Error if it is not nil.
func (db *FakeErroringDB) IsMember(c context.Context, id identity.Identity, groups []string) (bool, error) {
	if db.Error != nil {
		return false, db.Error
	}
	return db.FakeDB.IsMember(c, id, groups)
}

// CheckMembership is part of authdb.DB interface.
//
// It returns db.Error if it is not nil.
func (db *FakeErroringDB) CheckMembership(c context.Context, id identity.Identity, groups []string) ([]string, error) {
	if db.Error != nil {
		return nil, db.Error
	}
	return db.FakeDB.CheckMembership(c, id, groups)
}

// Use installs the fake db into the context.
func (db *FakeErroringDB) Use(c context.Context) context.Context {
	return auth.ModifyConfig(c, func(cfg auth.Config) auth.Config {
		cfg.DBProvider = func(context.Context) (authdb.DB, error) {
			return db, nil
		}
		return cfg
	})
}
