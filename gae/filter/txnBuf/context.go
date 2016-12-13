// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package txnBuf

import (
	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
)

var (
	dsTxnBufParent   = "holds a *txnBufState of the parent transaction"
	dsTxnBufHaveLock = "a boolean indicating that this context has the lock for this level of the transaction"
)

// FilterRDS installs a transaction buffer datastore filter in the context.
func FilterRDS(c context.Context) context.Context {
	// TODO(riannucci): allow the specification of the set of roots to limit this
	// transaction to, transitively.
	return ds.AddRawFilters(c, func(c context.Context, rds ds.RawInterface) ds.RawInterface {
		if par, _ := c.Value(&dsTxnBufParent).(*txnBufState); par != nil {
			haveLock, _ := c.Value(&dsTxnBufHaveLock).(bool)
			return &dsTxnBuf{c, par, haveLock, rds}
		}
		return &dsBuf{rds}
	})
}

// impossible is a marker function to indicate that the given error is an
// impossible state, due to conditions outside of the function.
func impossible(err error) {
	if err != nil {
		panic(err)
	}
}

// memoryCorruption is a marker function to indicate that given error is
// actually due to corrupted memory to make it easier to read the code.
func memoryCorruption(err error) {
	if err != nil {
		panic(err)
	}
}
