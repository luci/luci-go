// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package txnBuf

import (
	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
)

type dsBuf struct {
	ds.RawInterface
}

var _ ds.RawInterface = (*dsBuf)(nil)

func (d *dsBuf) RunInTransaction(f func(context.Context) error, opts *ds.TransactionOptions) error {
	return doRunInTransaction(d.RawInterface, f, opts)
}

func doRunInTransaction(base ds.RawInterface, f func(context.Context) error, opts *ds.TransactionOptions) error {
	return base.RunInTransaction(func(ctx context.Context) error {
		return withTxnBuf(ctx, f, opts)
	}, opts)
}
