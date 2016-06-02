// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package txnBuf

import (
	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
)

type key int

var (
	dsTxnBufParent   key
	dsTxnBufHaveLock key = 1
)

// FilterRDS installs a transaction buffer datastore filter in the context.
func FilterRDS(c context.Context) context.Context {
	// TODO(riannucci): allow the specification of the set of roots to limit this
	// transaction to, transitively.
	return ds.AddRawFilters(c, func(c context.Context, rds ds.RawInterface) ds.RawInterface {
		if par, _ := c.Value(dsTxnBufParent).(*txnBufState); par != nil {
			haveLock, _ := c.Value(dsTxnBufHaveLock).(bool)
			return &dsTxnBuf{c, par, haveLock}
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

// GetNoTxn allows you to to escape the buffered transaction (if any), and get
// a non-transactional handle to the datastore. This does not invalidate the
// currently-buffered transaction, but it also does not see any effects of it.
//
// TODO(iannucci): This is messy, but mostly because the way that transactions
// work is messy. Fixing luci/gae#issues/23 would help though, because it means
// we could make transactionality recorded by a single index in the context
// instead of relying on function nesting.
func GetNoTxn(c context.Context) ds.Interface {
	c = context.WithValue(c, dsTxnBufParent, nil)
	c = context.WithValue(c, dsTxnBufHaveLock, nil)
	return ds.GetNoTxn(c)
}
