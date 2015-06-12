// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

// GetReplicationState returns AuthReplicationState singleton entity.
func GetReplicationState(c context.Context) (*AuthReplicationState, error) {
	a := &AuthReplicationState{}
	err := datastore.Get(c, ReplicationStateKey(c), a)
	if err != nil {
		return nil, err
	}
	return a, nil
}
