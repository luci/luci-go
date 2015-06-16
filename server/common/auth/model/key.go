// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

// RootKey returns root key of AuthGlobalConfig.
func RootKey(c context.Context) *datastore.Key {
	return datastore.NewKey(c, "AuthGlobalConfig", "root", 0, nil)
}

// ReplicationStateKey returns self key of AuthReplicationState.
func ReplicationStateKey(c context.Context) *datastore.Key {
	return datastore.NewKey(c, "AuthReplicationState", "self", 0, RootKey(c))
}

// GroupKey returns an AuthGroup key for a specified group.
func GroupKey(c context.Context, group string) *datastore.Key {
	return datastore.NewKey(c, "AuthGroup", group, 0, RootKey(c))
}

// IPWhitelistKey returns an AuthIPWhitelist key for a specified name.
func IPWhitelistKey(c context.Context, name string) *datastore.Key {
	return datastore.NewKey(c, "AuthIPWhitelist", name, 0, RootKey(c))
}

// IPWhitelistAssignmentsKey returns the default AuthIPWhitelistAssignments key.
func IPWhitelistAssignmentsKey(c context.Context) *datastore.Key {
	return datastore.NewKey(c, "AuthIPWhitelistAssignments", "default", 0, RootKey(c))
}
