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
	"net"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/signing"
)

// FakeDB implements user group checking part of db.DB (IsMember).
//
// It is a mapping "identity -> list of its groups". Intended to be used mostly
// for testing request handlers, thus all other DB methods (that used by auth
// system when authenticating the request) is not implement and panic when
// called: the wast majority of request handlers are not calling them.
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

// IsAllowedOAuthClientID is part of authdb.DB interface. Panics.
func (db FakeDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	panic("FakeDB.IsAllowedOAuthClientID must not be called")
}

// GetCertificates is part of authdb.DB interface. Panics.
func (db FakeDB) GetCertificates(c context.Context, id identity.Identity) (*signing.PublicCertificates, error) {
	panic("FakeDB.GetCertificates must not be called")
}

// GetWhitelistForIdentity is part of authdb.DB interface. Panics.
func (db FakeDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	panic("FakeDB.GetWhitelistForIdentity must not be called")
}

// IsInWhitelist is part of authdb.DB interface. Panics.
func (db FakeDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	panic("FakeDB.IsInWhitelist must not be called")
}

// GetAuthServiceURL is part of authdb.DB interface. Panics.
func (db FakeDB) GetAuthServiceURL(c context.Context) (string, error) {
	panic("FakeDB.GetAuthServiceURL must not be called")
}

// GetTokenServiceURL is part of authdb.DB interface. Panics.
func (db FakeDB) GetTokenServiceURL(c context.Context) (string, error) {
	panic("FakeDB.GetTokenServiceURL must not be called")
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
