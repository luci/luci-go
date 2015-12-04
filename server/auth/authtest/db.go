// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package authtest

import (
	"net"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/secrets"
)

// FakeDB implements user group checking part of auth.DB (IsMember).
//
// It is a mapping "identity -> list of its groups". Intended to be used mostly
// for testing request handlers, thus all other DB methods (that used by auth
// system when authenticating the request) is not implement and panic when
// called: the wast majority of request handlers are not calling them.
type FakeDB map[identity.Identity][]string

var _ auth.DB = (FakeDB)(nil)

// IsMember is part of auth.DB interface.
//
// It returns true if 'group' is listed in db[id].
func (db FakeDB) IsMember(c context.Context, id identity.Identity, group string) (bool, error) {
	for _, gr := range db[id] {
		if gr == group {
			return true, nil
		}
	}
	return false, nil
}

// IsAllowedOAuthClientID is part of auth.DB interface. Panics.
func (db FakeDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	panic("FakeDB.IsAllowedOAuthClientID must not be called")
}

// SharedSecrets is part of auth.DB interface. Panics.
func (db FakeDB) SharedSecrets(c context.Context) (secrets.Store, error) {
	panic("FakeDB.SharedSecrets must not be called")
}

// GetWhitelistForIdentity is part of auth.DB interface. Panics.
func (db FakeDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	panic("FakeDB.GetWhitelistForIdentity must not be called")
}

// IsInWhitelist is part of auth.DB interface. Panics.
func (db FakeDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	panic("FakeDB.IsInWhitelist must not be called")
}

// FakeErroringDB is auth.DB with IsMember returning a error.
type FakeErroringDB struct {
	FakeDB

	// Error is returned by IsMember.
	Error error
}

// IsMember is part of auth.DB interface.
//
// It returns db.Error if it is not nil.
func (db *FakeErroringDB) IsMember(c context.Context, id identity.Identity, group string) (bool, error) {
	if db.Error != nil {
		return false, db.Error
	}
	return db.FakeDB.IsMember(c, id, group)
}
