// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	rds "github.com/luci/gae/service/rawdatastore"
	"google.golang.org/appengine/datastore"
)

type dsKeyImpl struct {
	*datastore.Key
}

var _ rds.Key = dsKeyImpl{}

func (k dsKeyImpl) Parent() rds.Key { return dsR2F(k.Key.Parent()) }

// dsR2F (DS real-to-fake) converts an SDK Key to a rds.Key
func dsR2F(k *datastore.Key) rds.Key {
	return dsKeyImpl{k}
}

// dsR2FErr (DS real-to-fake with error) acts like dsR2F, but is for wrapping function
// invocations which return (*Key, error). e.g.
//
//   return dsR2FErr(datastore.Put(...))
//
// instead of:
//
//   k, err := datastore.Put(...)
//   if err != nil {
//     return nil, err
//   }
//   return dsR2F(k), nil
func dsR2FErr(k *datastore.Key, err error) (rds.Key, error) {
	if err != nil {
		return nil, err
	}
	return dsR2F(k), nil
}

// dsF2R (DS fake-to-real) converts a DSKey back to an SDK *Key.
func dsF2R(k rds.Key) *datastore.Key {
	if rkey, ok := k.(dsKeyImpl); ok {
		return rkey.Key
	}
	// we should always hit the fast case above, but just in case, safely round
	// trip through the proto encoding.
	rkey, err := datastore.DecodeKey(rds.KeyEncode(k))
	if err != nil {
		// should never happen in a good program, but it's not ignorable, and
		// passing an error back makes this function too cumbersome (and it causes
		// this `if err != nil { panic(err) }` logic to show up in a bunch of
		// places. Realistically, everything should hit the early exit clause above.
		panic(err)
	}
	return rkey
}

// dsMR2F (DS multi-real-to-fake) converts a slice of SDK keys to their wrapped
// types.
func dsMR2F(ks []*datastore.Key) []rds.Key {
	ret := make([]rds.Key, len(ks))
	for i, k := range ks {
		ret[i] = dsR2F(k)
	}
	return ret
}

// dsMF2R (DS multi-fake-to-fake) converts a slice of wrapped keys to SDK keys.
func dsMF2R(ks []rds.Key) []*datastore.Key {
	ret := make([]*datastore.Key, len(ks))
	for i, k := range ks {
		ret[i] = dsF2R(k)
	}
	return ret
}
