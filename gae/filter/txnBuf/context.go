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

package txnBuf

import (
	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
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
