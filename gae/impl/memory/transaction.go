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

package memory

import (
	"sync/atomic"

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"

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
