// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"sync/atomic"

	ds "github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/common/errors"

	"golang.org/x/net/context"
)

type transactionImpl struct {
	// boolean 0 or 1, use atomic.*Int32 to access.
	closed int32
	isXG   bool
}

func (ti *transactionImpl) close() error {
	if !atomic.CompareAndSwapInt32(&ti.closed, 0, 1) {
		return errors.New("transaction is already closed")
	}
	return nil
}

func (ti *transactionImpl) valid() error {
	if atomic.LoadInt32(&ti.closed) == 1 {
		return errors.New("transaction has expired")
	}
	return nil
}

func assertTxnValid(c context.Context) error {
	t := ds.CurrentTransaction(c)
	if t == nil {
		return nil
	}

	return t.(*transactionImpl).valid()
}
