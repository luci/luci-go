// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package revocation

import (
	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/errors"
)

const tokenIDSequence = "delegationTokenID"

// GenerateTokenID produces an int64 that can be used as a token identifier.
//
// We reuse datastore ID generator to produce token ids. The tokens are not
// actually stored in the datastore. The generated ID sequence is associated
// with some entity kind (indicated via 'kind'). If we ever need to restart the
// ID sequence, this kind can be changed.
func GenerateTokenID(c context.Context, kind string) (int64, error) {
	// Note: AllocateIDs modifies passed slice in place, by replacing the keys
	// there.
	keys := []*datastore.Key{
		datastore.NewKey(c, kind, "", 0, nil),
	}
	if err := datastore.AllocateIDs(c, keys); err != nil {
		return 0, errors.WrapTransient(err)
	}
	return keys[0].IntID(), nil
}
